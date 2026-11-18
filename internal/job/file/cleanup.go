// Package file 负责文件对象的异步维护：回收过期未完成的 pending 记录与已删除记录的对象。
package file

import (
	"context"
	"time"

	domain "goim-business/internal/domain/file"
	"goim-business/internal/pkg/logger"
	"goim-business/internal/pkg/storage/s3"
)

// defaultBatchSize 是未配置批量大小时的兜底值。
const defaultBatchSize = 100

// CleanupResult 是一轮清理的统计结果。
type CleanupResult struct {
	// Scanned 是本轮扫描到的记录数（两阶段合计）。
	Scanned int
	// Cleared 是本轮由过期 pending 转为 deleted 的记录数。
	Cleared int
	// Reclaimed 是本轮确认对象已回收的记录数。
	Reclaimed int
	// Failed 是对象操作失败的记录数，下轮重试。
	Failed int
}

// Cleaner 两阶段回收文件：过期 pending 先置 deleted 再删对象；已删除记录补删残留对象。
type Cleaner struct {
	repo      domain.Repository
	s3        s3.S3Client
	batchSize int
	now       func() time.Time
	log       logger.Logger
}

// NewCleaner 构造清理器；storage 为 nil 表示只回收元数据（对象由桶生命周期规则兜底）。
func NewCleaner(repo domain.Repository, storage s3.S3Client, batchSize int, now func() time.Time, log logger.Logger) *Cleaner {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if now == nil {
		now = time.Now
	}
	return &Cleaner{repo: repo, s3: storage, batchSize: batchSize, now: now, log: log}
}

// RunOnce 依次执行两个阶段：先回收过期 pending，再补收已删除但对象残留的记录。
// 单条失败只记日志，不中断整批。
func (c *Cleaner) RunOnce(ctx context.Context) CleanupResult {
	result := CleanupResult{}
	result.merge(c.clearExpiredPending(ctx))
	result.merge(c.reclaimDeleted(ctx))
	return result
}

// clearExpiredPending 扫描一批过期 pending 记录并逐个置 deleted 后删除对象。
func (c *Cleaner) clearExpiredPending(ctx context.Context) CleanupResult {
	result := CleanupResult{}
	records, err := c.repo.ListExpiredPending(ctx, c.now(), c.batchSize)
	if err != nil {
		c.log.Errorf("扫描过期 pending 文件失败: %v", err)
		return result
	}
	result.Scanned = len(records)
	for _, record := range records {
		if ctx.Err() != nil {
			break
		}
		updated, err := c.clear(ctx, record)
		if err != nil {
			result.Failed++
			c.log.Warnf("回收过期文件失败: file_id=%d, key=%s, err=%v", record.ID, record.ObjectKey, err)
			continue
		}
		if updated {
			result.Cleared++
		}
	}
	if result.Scanned > 0 {
		c.log.Infof("过期 pending 文件清理完成: scanned=%d, cleared=%d, failed=%d",
			result.Scanned, result.Cleared, result.Failed)
	}
	return result
}

// reclaimDeleted 扫描已删除但对象未确认回收的记录，补删残留对象并写入回收时间。
func (c *Cleaner) reclaimDeleted(ctx context.Context) CleanupResult {
	result := CleanupResult{}
	records, err := c.repo.ListDeletedObjects(ctx, c.batchSize)
	if err != nil {
		c.log.Errorf("扫描待回收删除对象失败: %v", err)
		return result
	}
	result.Scanned = len(records)
	for _, record := range records {
		if ctx.Err() != nil {
			break
		}
		reclaimed, err := c.reclaim(ctx, record)
		if err != nil {
			result.Failed++
			c.log.Warnf("回收删除对象失败: file_id=%d, key=%s, err=%v", record.ID, record.ObjectKey, err)
			continue
		}
		if reclaimed {
			result.Reclaimed++
		}
	}
	if result.Scanned > 0 {
		c.log.Infof("删除态对象回收完成: scanned=%d, reclaimed=%d, failed=%d",
			result.Scanned, result.Reclaimed, result.Failed)
	}
	return result
}

// clear 先把记录原子置为 deleted，再回收对象：顺序反了会与 complete 竞争，可能误删刚置 ready 的对象。
// 对象回收失败只记日志，记录保持 object_deleted_at 为空，由第二阶段或下一轮重试。
// 返回是否真正由本轮把记录置为 deleted；false 表示记录已被 complete 或用户删除处理。
func (c *Cleaner) clear(ctx context.Context, record *domain.File) (bool, error) {
	ok, err := c.repo.MarkDeletedIfPending(ctx, record.ID, record.UserID, c.now())
	if err != nil {
		return false, err
	}
	if !ok {
		// 记录已被 complete 或用户删除处理，不再触碰对象。
		return false, nil
	}
	_ = c.removeObject(ctx, record)
	return true, nil
}

// reclaim 删除已删除记录的残留对象（分片上传先 abort），成功后写入 object_deleted_at。
func (c *Cleaner) reclaim(ctx context.Context, record *domain.File) (bool, error) {
	if err := c.removeObject(ctx, record); err != nil {
		return false, err
	}
	return true, nil
}

// removeObject 删除对象并在成功后记录 object_deleted_at；失败返回错误，下轮重试。
func (c *Cleaner) removeObject(ctx context.Context, record *domain.File) error {
	if c.s3 == nil {
		c.markObjectDeleted(ctx, record)
		return nil
	}
	// upload_id 在分片合并成功时会被清空，因此这里只会命中原先未完成的分片上传：
	// 必须先 abort 丢弃残留分片，再删除对象。
	if record.UploadID != "" {
		if err := c.s3.AbortMultipartUpload(ctx, record.ObjectKey, record.UploadID); err != nil && !s3.IsNotFound(err) {
			c.log.Warnf("回收分片上传失败: file_id=%d, key=%s, err=%v", record.ID, record.ObjectKey, err)
			return err
		}
	}
	if err := c.s3.DeleteObject(ctx, record.ObjectKey); err != nil && !s3.IsNotFound(err) {
		c.log.Warnf("删除对象失败: file_id=%d, key=%s, err=%v", record.ID, record.ObjectKey, err)
		return err
	}
	c.markObjectDeleted(ctx, record)
	return nil
}

// markObjectDeleted 记录对象回收完成；失败保留 NULL，下一轮重试。
func (c *Cleaner) markObjectDeleted(ctx context.Context, record *domain.File) {
	if _, err := c.repo.MarkObjectDeleted(ctx, record.ID, c.now()); err != nil {
		c.log.Warnf("记录对象回收完成失败: file_id=%d, err=%v", record.ID, err)
	}
}

func (r *CleanupResult) merge(other CleanupResult) {
	r.Scanned += other.Scanned
	r.Cleared += other.Cleared
	r.Reclaimed += other.Reclaimed
	r.Failed += other.Failed
}
