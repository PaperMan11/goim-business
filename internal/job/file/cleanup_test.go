package file

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	domain "goim-business/internal/domain/file"
	"goim-business/internal/pkg/storage/s3"
)

type fakeRepo struct {
	records []*domain.File
	// pending 为 false 时模拟记录已被 complete 处理，MarkDeletedIfPending 返回 false。
	pending bool
}

func (r *fakeRepo) Create(context.Context, *domain.File) error { return nil }

func (r *fakeRepo) GetByID(context.Context, int64) (*domain.File, error) {
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) MarkReady(context.Context, int64, int64, int64, string, time.Time) (bool, error) {
	return false, nil
}

func (r *fakeRepo) MarkDeleted(context.Context, int64, int64, time.Time) (bool, error) {
	return false, nil
}

func (r *fakeRepo) ListExpiredPending(context.Context, time.Time, int) ([]*domain.File, error) {
	return r.records, nil
}

func (r *fakeRepo) MarkDeletedIfPending(_ context.Context, id, userID int64, at time.Time) (bool, error) {
	if !r.pending {
		return false, nil
	}
	for _, record := range r.records {
		if record.ID == id && record.UserID == userID {
			record.Status = domain.StatusDeleted
			record.UpdatedAt = at
			return true, nil
		}
	}
	return false, nil
}

type fakeStorage struct {
	aborted   []string
	deleted   []string
	abortErr  error
	deleteErr error
}

func (f *fakeStorage) InitiateMultipartUpload(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *fakeStorage) GenerateUploadPartPresignedURL(context.Context, string, string, int32, int64) (s3.PresignedURLResult, error) {
	return s3.PresignedURLResult{}, nil
}

func (f *fakeStorage) CompleteMultipartUpload(context.Context, string, string, []s3.MultipartCompletedPart) (string, error) {
	return "", nil
}

func (f *fakeStorage) AbortMultipartUpload(_ context.Context, key, _ string) error {
	f.aborted = append(f.aborted, key)
	return f.abortErr
}

func (f *fakeStorage) GeneratePostFormPresignedData(context.Context, string, string) (s3.PostFormPresignedResult, error) {
	return s3.PostFormPresignedResult{}, nil
}

func (f *fakeStorage) GeneratePutObjectPresignedURL(context.Context, string, string, int64) (s3.PresignedURLResult, error) {
	return s3.PresignedURLResult{}, nil
}

func (f *fakeStorage) GenerateGetObjectPresignedURL(context.Context, string, int64) (s3.PresignedURLResult, error) {
	return s3.PresignedURLResult{}, nil
}

func (f *fakeStorage) HeadObject(context.Context, string) (*s3.ObjectInfo, error) { return nil, nil }

func (f *fakeStorage) DeleteObject(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return f.deleteErr
}

func (f *fakeStorage) ListObjects(context.Context, string) ([]s3.ObjectInfo, error) { return nil, nil }

func TestCleanerReclaimsExpiredPendingRecords(t *testing.T) {
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	expired := []*domain.File{
		{
			ID: 1, UserID: 7, Purpose: domain.PurposeAttachment,
			ObjectKey: "attachment/7/file_a.pdf", UploadID: "upload-1",
			Status: domain.StatusPending,
		},
		{
			ID: 2, UserID: 8, Purpose: domain.PurposeAvatar,
			ObjectKey: "public/avatar/8/file_b.png",
			Status:    domain.StatusPending,
		},
	}
	repo := &fakeRepo{records: expired, pending: true}
	storage := &fakeStorage{}
	cleaner := NewCleaner(repo, storage, 10, func() time.Time { return now }, zap.NewNop().Sugar())

	result := cleaner.RunOnce(context.Background())
	require.Equal(t, CleanupResult{Scanned: 2, Cleared: 2}, result)
	require.Equal(t, []string{"attachment/7/file_a.pdf"}, storage.aborted, "分片上传应 abort 而不是删对象")
	require.Equal(t, []string{"public/avatar/8/file_b.png"}, storage.deleted, "单文件 PUT 应删除可能残留的对象")
	require.Equal(t, domain.StatusDeleted, expired[0].Status)
	require.Equal(t, domain.StatusDeleted, expired[1].Status)
}

func TestCleanerSkipsRecordCompletedConcurrently(t *testing.T) {
	repo := &fakeRepo{
		records: []*domain.File{{
			ID: 1, UserID: 7, Purpose: domain.PurposeAvatar,
			ObjectKey: "public/avatar/7/file_a.png",
			Status:    domain.StatusPending,
		}},
		pending: false,
	}
	storage := &fakeStorage{}
	cleaner := NewCleaner(repo, storage, 10, time.Now, zap.NewNop().Sugar())

	result := cleaner.RunOnce(context.Background())
	require.Equal(t, CleanupResult{Scanned: 1}, result)
	require.Empty(t, storage.aborted)
	require.Empty(t, storage.deleted, "记录已被 complete/删除处理时不得触碰对象")
}
