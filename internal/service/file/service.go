// Package file 实现文件上传业务：申请直传、完成校验、访问签名与删除。
package file

import (
	"context"
	"time"

	domain "goim-business/internal/domain/file"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/idgen"
	"goim-business/internal/pkg/logger"
	"goim-business/internal/pkg/storage/s3"
	"goim-business/internal/pkg/timex"
	"goim-business/internal/types"
)

// Config 是文件服务的运行期参数。
type Config struct {
	// PresignExpire 是单文件上传/下载预签名 URL 的有效期。
	PresignExpire time.Duration
	// MultipartPresignExpire 是分片上传签名有效期，需覆盖大文件弱网上传时长。
	MultipartPresignExpire time.Duration
	// PendingTTL 是 pending 记录的最长保留时间，超时由清理任务回收。
	PendingTTL time.Duration
}

// Service 是文件服务。
type Service struct {
	repo domain.Repository
	s3   s3.S3Client
	cfg  Config
	log  logger.Logger
	now  func() time.Time
}

// NewService 构造文件服务；s3Client 为 nil 表示对象存储未启用。
func NewService(repo domain.Repository, s3Client s3.S3Client, cfg Config, log logger.Logger) *Service {
	if cfg.PresignExpire <= 0 {
		cfg.PresignExpire = 15 * time.Minute
	}
	if cfg.MultipartPresignExpire <= 0 {
		cfg.MultipartPresignExpire = 2 * time.Hour
	}
	if cfg.PendingTTL <= 0 {
		cfg.PendingTTL = 24 * time.Hour
	}
	return &Service{repo: repo, s3: s3Client, cfg: cfg, log: log, now: timex.Now}
}

// PresignUpload 校验申请并生成直传 URL；记录以 pending 状态落库，超时未完成由清理任务回收。
func (s *Service) PresignUpload(ctx context.Context, userID int64, in types.FilePresignRequest) (*types.FilePresignResp, error) {
	if s.s3 == nil {
		return nil, errcode.ErrDependency.WithMessage("对象存储未配置，暂不支持文件上传")
	}
	if err := domain.ValidateUpload(in.Purpose, in.MIME, in.Size, in.SHA256); err != nil {
		return nil, errcode.InvalidParam(err)
	}
	ext, err := domain.ExtForUpload(in.Purpose, in.MIME)
	if err != nil {
		return nil, errcode.InvalidParam(err)
	}
	uuid, err := idgen.NewWithPrefix(idgen.PrefixFile)
	if err != nil {
		s.log.Errorf("生成文件 ID 失败: %v", err)
		return nil, errcode.ErrInternal
	}

	now := s.now()
	objectKey := domain.BuildObjectKey(in.Purpose, userID, uuid, ext)
	file := domain.NewFile(userID, in.Purpose, in.FileName, objectKey, in.MIME, in.Size, in.SHA256, now.Add(s.cfg.PendingTTL), now)
	if err := s.repo.Create(ctx, file); err != nil {
		return nil, s.dbErr("创建文件记录", err)
	}

	presigned, err := s.s3.GeneratePutObjectPresignedURL(ctx, objectKey, file.MIME, int64(s.cfg.PresignExpire.Seconds()))
	if err != nil {
		s.log.Errorf("生成上传签名失败: file_id=%d, key=%s, err=%v", file.ID, objectKey, err)
		return nil, errcode.ErrDependency
	}

	headers := make(map[string]string, len(presigned.Headers))
	for _, h := range presigned.Headers {
		if len(h.Values) > 0 {
			headers[h.Key] = h.Values[0]
		}
	}
	return &types.FilePresignResp{
		FileID:    convert.ToString(file.ID),
		ObjectKey: file.ObjectKey,
		UploadURL: presigned.URL,
		Method:    "PUT",
		Headers:   headers,
		ExpiresAt: presigned.ExpireAt / 1000,
	}, nil
}

// Complete 校验对象是否已上传且符合申请时的类型/大小，通过后置为 ready。
func (s *Service) Complete(ctx context.Context, userID int64, rawFileID string) (*types.FileView, error) {
	if s.s3 == nil {
		return nil, errcode.ErrDependency.WithMessage("对象存储未配置，暂不支持文件上传")
	}
	fileID, err := parseFileID(rawFileID)
	if err != nil {
		return nil, err
	}
	file, err := s.loadOwned(ctx, fileID, userID)
	if err != nil {
		return nil, err
	}
	// 重复 complete 幂等返回，便于客户端重试。
	if file.Status == domain.StatusReady {
		return fileView(file), nil
	}
	return s.verifyAndMarkReady(ctx, file, userID)
}

// MultipartPresign 初始化分片上传并一次性返回全部分片的预签名 URL。
// 当前仅 attachment 用途支持；申请前需先调用本接口，不能对单文件 PUT 使用。
func (s *Service) MultipartPresign(ctx context.Context, userID int64, in types.MultipartPresignRequest) (*types.MultipartPresignResp, error) {
	if s.s3 == nil {
		return nil, errcode.ErrDependency.WithMessage("对象存储未配置，暂不支持文件上传")
	}
	if err := domain.ValidateMultipartUpload(in.Purpose, in.MIME, in.Size, in.SHA256); err != nil {
		return nil, errcode.InvalidParam(err)
	}
	ext, err := domain.ExtForUpload(in.Purpose, in.MIME)
	if err != nil {
		return nil, errcode.InvalidParam(err)
	}
	uuid, err := idgen.NewWithPrefix(idgen.PrefixFile)
	if err != nil {
		s.log.Errorf("生成文件 ID 失败: %v", err)
		return nil, errcode.ErrInternal
	}

	now := s.now()
	objectKey := domain.BuildObjectKey(in.Purpose, userID, uuid, ext)
	uploadID, err := s.s3.InitiateMultipartUpload(ctx, objectKey, domain.NormalizeMIME(in.MIME))
	if err != nil {
		s.log.Errorf("初始化分片上传失败: key=%s, err=%v", objectKey, err)
		return nil, errcode.ErrDependency
	}

	file := domain.NewFile(userID, in.Purpose, in.FileName, objectKey, in.MIME, in.Size, in.SHA256, now.Add(s.cfg.PendingTTL), now)
	file.UploadID = uploadID
	if err := s.repo.Create(ctx, file); err != nil {
		s.abortBestEffort(ctx, objectKey, uploadID)
		return nil, s.dbErr("创建文件记录", err)
	}

	partCount := domain.MultipartPartCount(in.Size)
	numbers := make([]int32, 0, partCount)
	for partNumber := int32(1); partNumber <= partCount; partNumber++ {
		numbers = append(numbers, partNumber)
	}
	parts, expiresAt, err := s.signParts(ctx, file, numbers)
	if err != nil {
		s.log.Errorf("生成分片签名失败: file_id=%d, key=%s, err=%v", file.ID, file.ObjectKey, err)
		s.abortBestEffort(ctx, objectKey, uploadID)
		s.markDeletedBestEffort(ctx, file.ID, userID)
		return nil, errcode.ErrDependency
	}
	return &types.MultipartPresignResp{
		FileID:    convert.ToString(file.ID),
		ObjectKey: file.ObjectKey,
		UploadID:  uploadID,
		PartSize:  domain.MultipartPartSize,
		PartCount: partCount,
		Parts:     parts,
		ExpiresAt: expiresAt / 1000,
	}, nil
}

// RefreshMultipartParts 为未完成的分片上传重新签名；不改变 uploadId 与分片大小，已传分片保留。
func (s *Service) RefreshMultipartParts(ctx context.Context, userID int64, rawFileID string, numbers []int32) (*types.MultipartPresignResp, error) {
	if s.s3 == nil {
		return nil, errcode.ErrDependency.WithMessage("对象存储未配置，暂不支持文件上传")
	}
	fileID, err := parseFileID(rawFileID)
	if err != nil {
		return nil, err
	}
	file, err := s.loadOwned(ctx, fileID, userID)
	if err != nil {
		return nil, err
	}
	if file.Status == domain.StatusReady {
		return nil, errcode.ErrConflict.WithMessage("文件已上传完成，无需刷新分片")
	}
	if file.UploadID == "" {
		return nil, errcode.InvalidParam(domain.ErrInvalidParts)
	}

	partCount := domain.MultipartPartCount(file.Size)
	if len(numbers) == 0 {
		numbers = make([]int32, 0, partCount)
		for partNumber := int32(1); partNumber <= partCount; partNumber++ {
			numbers = append(numbers, partNumber)
		}
	}
	prev := int32(0)
	for _, n := range numbers {
		if n <= prev || n > partCount {
			return nil, errcode.InvalidParam(domain.ErrInvalidParts)
		}
		prev = n
	}

	parts, expiresAt, err := s.signParts(ctx, file, numbers)
	if err != nil {
		s.log.Errorf("刷新分片签名失败: file_id=%d, key=%s, err=%v", file.ID, file.ObjectKey, err)
		return nil, errcode.ErrDependency
	}
	return &types.MultipartPresignResp{
		FileID:    convert.ToString(file.ID),
		ObjectKey: file.ObjectKey,
		UploadID:  file.UploadID,
		PartSize:  domain.MultipartPartSize,
		PartCount: partCount,
		Parts:     parts,
		ExpiresAt: expiresAt / 1000,
	}, nil
}

// signParts 为指定分片号生成预签名 URL，返回分片信息与最晚过期时间（毫秒）。
func (s *Service) signParts(ctx context.Context, file *domain.File, numbers []int32) ([]types.MultipartPartPresign, int64, error) {
	parts := make([]types.MultipartPartPresign, 0, len(numbers))
	var expiresAt int64
	for _, partNumber := range numbers {
		presigned, err := s.s3.GenerateUploadPartPresignedURL(ctx, file.ObjectKey, file.UploadID, partNumber, int64(s.cfg.MultipartPresignExpire.Seconds()))
		if err != nil {
			return nil, 0, err
		}
		parts = append(parts, types.MultipartPartPresign{
			PartNumber: partNumber,
			URL:        presigned.URL,
			ExpiresAt:  presigned.ExpireAt / 1000,
		})
		if presigned.ExpireAt > expiresAt {
			expiresAt = presigned.ExpireAt
		}
	}
	return parts, expiresAt, nil
}

// MultipartComplete 合并分片并复用单文件的元数据校验；重复完成已 ready 的文件幂等返回。
func (s *Service) MultipartComplete(ctx context.Context, userID int64, rawFileID string, in types.MultipartCompleteRequest) (*types.FileView, error) {
	if s.s3 == nil {
		return nil, errcode.ErrDependency.WithMessage("对象存储未配置，暂不支持文件上传")
	}
	fileID, err := parseFileID(rawFileID)
	if err != nil {
		return nil, err
	}
	file, err := s.loadOwned(ctx, fileID, userID)
	if err != nil {
		return nil, err
	}
	if file.Status == domain.StatusReady {
		return fileView(file), nil
	}
	if file.UploadID == "" {
		return nil, errcode.InvalidParam(domain.ErrInvalidParts)
	}

	expected := domain.MultipartPartCount(file.Size)
	if int32(len(in.Parts)) != expected {
		return nil, errcode.InvalidParam(domain.ErrInvalidParts)
	}
	parts := make([]s3.MultipartCompletedPart, 0, len(in.Parts))
	for idx, p := range in.Parts {
		if p.PartNumber != int32(idx+1) {
			return nil, errcode.InvalidParam(domain.ErrInvalidParts)
		}
		parts = append(parts, s3.MultipartCompletedPart{PartNumber: p.PartNumber, ETag: p.ETag})
	}

	if _, err := s.s3.CompleteMultipartUpload(ctx, file.ObjectKey, file.UploadID, parts); err != nil {
		if s3.IsNotFound(err) {
			return nil, errcode.ErrInvalidParam.WithMessage("上传任务不存在或已过期，请重新申请上传")
		}
		if s3.IsInvalidMultipart(err) {
			return nil, errcode.InvalidParam(domain.ErrInvalidParts)
		}
		s.log.Errorf("合并分片失败: file_id=%d, key=%s, err=%v", file.ID, file.ObjectKey, err)
		return nil, errcode.ErrDependency
	}
	return s.verifyAndMarkReady(ctx, file, userID)
}

// MultipartAbort 放弃未完成的分片上传；已 ready 的文件不允许 abort。
func (s *Service) MultipartAbort(ctx context.Context, userID int64, rawFileID string) error {
	fileID, err := parseFileID(rawFileID)
	if err != nil {
		return err
	}
	file, err := s.load(ctx, fileID, userID)
	if err != nil {
		return err
	}
	if file.Status == domain.StatusDeleted {
		return nil
	}
	if file.Status != domain.StatusPending || file.UploadID == "" {
		return errcode.ErrConflict.WithMessage("文件已上传完成，无法取消分片上传")
	}
	ok, err := s.repo.MarkDeleted(ctx, file.ID, userID, s.now())
	if err != nil {
		return s.dbErr("取消分片上传", err)
	}
	if !ok {
		return errcode.ErrConflict.WithMessage("文件状态已变化，请刷新后重试")
	}
	if s.abortBestEffort(ctx, file.ObjectKey, file.UploadID) {
		s.markObjectDeletedBestEffort(ctx, file.ID)
	}
	return nil
}

// verifyAndMarkReady 读取对象元数据、校验类型/大小并置为 ready。
func (s *Service) verifyAndMarkReady(ctx context.Context, file *domain.File, userID int64) (*types.FileView, error) {
	info, err := s.s3.HeadObject(ctx, file.ObjectKey)
	if err != nil {
		if s3.IsNotFound(err) {
			return nil, errcode.ErrInvalidParam.WithMessage("对象不存在或尚未上传完成")
		}
		s.log.Errorf("校验上传对象失败: file_id=%d, key=%s, err=%v", file.ID, file.ObjectKey, err)
		return nil, errcode.ErrDependency
	}

	maxSize, err := domain.MaxSizeFor(file.Purpose)
	if err != nil {
		s.log.Errorf("文件用途非法: file_id=%d, purpose=%s", file.ID, file.Purpose)
		return nil, errcode.ErrInternal
	}
	if info.Size <= 0 || info.Size > maxSize {
		s.log.Warnf("上传对象大小不合法: file_id=%d, size=%d, max=%d", file.ID, info.Size, maxSize)
		return nil, errcode.InvalidParam(domain.ErrInvalidSize)
	}
	if info.ContentType != "" && domain.NormalizeMIME(info.ContentType) != file.MIME {
		s.log.Warnf("上传对象类型不匹配: file_id=%d, expect=%s, actual=%s", file.ID, file.MIME, info.ContentType)
		return nil, errcode.InvalidParam(domain.ErrInvalidMIME)
	}

	ok, err := s.repo.MarkReady(ctx, file.ID, userID, info.Size, file.MIME, s.now())
	if err != nil {
		return nil, s.dbErr("标记文件 ready", err)
	}
	if !ok {
		return nil, errcode.ErrConflict.WithMessage("文件状态已变化，请刷新后重试")
	}
	file.Size = info.Size
	file.Status = domain.StatusReady
	// 分片上传已完成；清空本地副本的 upload_id，与仓储保持一致。
	file.UploadID = ""
	return fileView(file), nil
}

// URL 为本人已就绪的文件生成短时访问地址。
func (s *Service) URL(ctx context.Context, userID int64, rawFileID string) (*types.FileURLResp, error) {
	if s.s3 == nil {
		return nil, errcode.ErrDependency.WithMessage("对象存储未配置，暂不支持文件访问")
	}
	fileID, err := parseFileID(rawFileID)
	if err != nil {
		return nil, err
	}
	file, err := s.loadOwned(ctx, fileID, userID)
	if err != nil {
		return nil, err
	}
	if file.Status != domain.StatusReady {
		return nil, errcode.InvalidParam(domain.ErrNotReady)
	}

	presigned, err := s.s3.GenerateGetObjectPresignedURL(ctx, file.ObjectKey, int64(s.cfg.PresignExpire.Seconds()))
	if err != nil {
		s.log.Errorf("生成访问签名失败: file_id=%d, key=%s, err=%v", file.ID, file.ObjectKey, err)
		return nil, errcode.ErrDependency
	}
	return &types.FileURLResp{URL: presigned.URL, ExpiresAt: presigned.ExpireAt / 1000}, nil
}

// Delete 软删文件并尽力删除对象；重复删除幂等。
func (s *Service) Delete(ctx context.Context, userID int64, rawFileID string) error {
	fileID, err := parseFileID(rawFileID)
	if err != nil {
		return err
	}
	file, err := s.load(ctx, fileID, userID)
	if err != nil {
		return err
	}
	if file.Status == domain.StatusDeleted {
		return nil
	}
	wasPending := file.Status == domain.StatusPending

	ok, err := s.repo.MarkDeleted(ctx, file.ID, userID, s.now())
	if err != nil {
		return s.dbErr("删除文件记录", err)
	}
	if !ok {
		return errcode.ErrConflict.WithMessage("文件状态已变化，请刷新后重试")
	}

	if s.s3 == nil {
		// 未配置对象存储时不存在对象，直接记录回收完成，避免清理任务反复扫描。
		s.markObjectDeletedBestEffort(ctx, file.ID)
		return nil
	}
	if wasPending && file.UploadID != "" {
		if s.abortBestEffort(ctx, file.ObjectKey, file.UploadID) {
			s.markObjectDeletedBestEffort(ctx, file.ID)
		}
		return nil
	}
	if err := s.s3.DeleteObject(ctx, file.ObjectKey); err != nil && !s3.IsNotFound(err) {
		// 元数据已软删，对象残留由清理任务按 deleted 状态回收。
		s.log.Warnf("删除对象失败: file_id=%d, key=%s, err=%v", file.ID, file.ObjectKey, err)
		return nil
	}
	s.markObjectDeletedBestEffort(ctx, file.ID)
	return nil
}

// abortBestEffort 放弃分片上传；失败只记日志并返回 false，残留对象由清理任务重试。
func (s *Service) abortBestEffort(ctx context.Context, objectKey, uploadID string) bool {
	if s.s3 == nil {
		return true
	}
	if err := s.s3.AbortMultipartUpload(ctx, objectKey, uploadID); err != nil && !s3.IsNotFound(err) {
		s.log.Warnf("取消分片上传失败: key=%s, upload_id=%s, err=%v", objectKey, uploadID, err)
		return false
	}
	return true
}

// markObjectDeletedBestEffort 记录对象已确认回收；失败保留 NULL，由清理任务重试。
func (s *Service) markObjectDeletedBestEffort(ctx context.Context, fileID int64) {
	if _, err := s.repo.MarkObjectDeleted(ctx, fileID, s.now()); err != nil {
		s.log.Warnf("记录对象回收完成失败: file_id=%d, err=%v", fileID, err)
	}
}

// markDeletedBestEffort 在签名等后续步骤失败时回收已创建的 pending 记录。
func (s *Service) markDeletedBestEffort(ctx context.Context, fileID, userID int64) {
	if _, err := s.repo.MarkDeleted(ctx, fileID, userID, s.now()); err != nil {
		s.log.Warnf("回收 pending 文件失败: file_id=%d, err=%v", fileID, err)
	}
}

// loadOwned 读取文件并校验归属与可用状态；他人或已删除文件统一返回不存在。
func (s *Service) loadOwned(ctx context.Context, fileID, userID int64) (*domain.File, error) {
	file, err := s.load(ctx, fileID, userID)
	if err != nil {
		return nil, err
	}
	if file.Status == domain.StatusDeleted {
		return nil, errcode.ErrNotFound
	}
	return file, nil
}

// load 读取文件并校验归属；他人文件统一返回不存在，避免暴露资源存在性。
func (s *Service) load(ctx context.Context, fileID, userID int64) (*domain.File, error) {
	file, err := s.repo.GetByID(ctx, fileID)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, errcode.ErrNotFound
		}
		return nil, s.dbErr("读取文件记录", err)
	}
	if file.UserID != userID {
		return nil, errcode.ErrNotFound
	}
	return file, nil
}

func parseFileID(raw string) (int64, error) {
	id := convert.ToInt64(raw)
	if id <= 0 {
		return 0, errcode.ErrInvalidParam.WithMessage("fileId 格式非法")
	}
	return id, nil
}

func (s *Service) dbErr(op string, err error) error {
	s.log.Errorf("数据库操作失败: op=%s, err=%v", op, err)
	return errcode.ErrDependency
}

func fileView(file *domain.File) *types.FileView {
	return &types.FileView{
		FileID:    convert.ToString(file.ID),
		Purpose:   file.Purpose,
		ObjectKey: file.ObjectKey,
		MIME:      file.MIME,
		Size:      file.Size,
		SHA256:    file.SHA256,
		Status:    file.Status,
		CreatedAt: file.CreatedAt,
	}
}
