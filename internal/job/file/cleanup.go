// Package file 负责文件对象的异步维护：回收过期仍未完成的 pending 记录及其对象。
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
	Scanned int
	Cleared int
	Failed  int
}

// Cleaner 回收过期未完成（pending）的文件记录。
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

// RunOnce 扫描一批过期 pending 记录并逐个回收；单条失败只记日志，不中断整批。
func (c *Cleaner) RunOnce(ctx context.Context) CleanupResult {
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

// clear 先把记录原子置为 deleted，再回收对象：顺序反了会与 complete 竞争，可能误删刚置 ready 的对象。
// 置 deleted 后对象回收失败只记日志，残留对象与分片由桶的生命周期规则兜底。
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
	if c.s3 == nil {
		return true, nil
	}
	if record.UploadID != "" {
		if err := c.s3.AbortMultipartUpload(ctx, record.ObjectKey, record.UploadID); err != nil && !s3.IsNotFound(err) {
			c.log.Warnf("回收过期分片上传失败: file_id=%d, key=%s, err=%v", record.ID, record.ObjectKey, err)
		}
		return true, nil
	}
	if err := c.s3.DeleteObject(ctx, record.ObjectKey); err != nil && !s3.IsNotFound(err) {
		c.log.Warnf("删除过期对象失败: file_id=%d, key=%s, err=%v", record.ID, record.ObjectKey, err)
	}
	return true, nil
}
