package file

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	domain "goim-business/internal/domain/file"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/storage/s3"
	"goim-business/internal/types"
)

type fakeRepo struct {
	nextID int64
	files  map[int64]*domain.File
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{nextID: 1, files: make(map[int64]*domain.File)}
}

func (r *fakeRepo) Create(_ context.Context, f *domain.File) error {
	f.ID = r.nextID
	r.nextID++
	copied := *f
	r.files[f.ID] = &copied
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id int64) (*domain.File, error) {
	f, ok := r.files[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copied := *f
	return &copied, nil
}

func (r *fakeRepo) MarkReady(_ context.Context, id, userID, size int64, mime string, at time.Time) (bool, error) {
	f, ok := r.files[id]
	if !ok || f.UserID != userID || f.Status != domain.StatusPending {
		return false, nil
	}
	f.Size = size
	f.MIME = mime
	f.UploadID = ""
	f.Status = domain.StatusReady
	f.UpdatedAt = at
	return true, nil
}

func (r *fakeRepo) MarkDeleted(_ context.Context, id, userID int64, at time.Time) (bool, error) {
	f, ok := r.files[id]
	if !ok || f.UserID != userID || f.Status == domain.StatusDeleted {
		return false, nil
	}
	f.Status = domain.StatusDeleted
	f.UpdatedAt = at
	return true, nil
}

func (r *fakeRepo) ListExpiredPending(_ context.Context, now time.Time, limit int) ([]*domain.File, error) {
	expired := make([]*domain.File, 0)
	for _, f := range r.files {
		if f.Status != domain.StatusPending || f.ExpiresAt == nil || f.ExpiresAt.After(now) {
			continue
		}
		copied := *f
		expired = append(expired, &copied)
		if limit > 0 && len(expired) >= limit {
			break
		}
	}
	return expired, nil
}

func (r *fakeRepo) ListDeletedObjects(_ context.Context, limit int) ([]*domain.File, error) {
	out := make([]*domain.File, 0)
	for _, f := range r.files {
		if f.Status != domain.StatusDeleted || f.ObjectDeletedAt != nil {
			continue
		}
		copied := *f
		out = append(out, &copied)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakeRepo) MarkObjectDeleted(_ context.Context, id int64, at time.Time) (bool, error) {
	f, ok := r.files[id]
	if !ok || f.Status != domain.StatusDeleted || f.ObjectDeletedAt != nil {
		return false, nil
	}
	value := at
	f.ObjectDeletedAt = &value
	f.UpdatedAt = at
	return true, nil
}

func (r *fakeRepo) MarkDeletedByUser(_ context.Context, userID int64, at time.Time) (int64, error) {
	var affected int64
	for _, f := range r.files {
		if f.UserID != userID || f.Status == domain.StatusDeleted {
			continue
		}
		f.Status = domain.StatusDeleted
		f.UpdatedAt = at
		affected++
	}
	return affected, nil
}

func (r *fakeRepo) MarkDeletedIfPending(_ context.Context, id, userID int64, at time.Time) (bool, error) {
	f, ok := r.files[id]
	if !ok || f.UserID != userID || f.Status != domain.StatusPending {
		return false, nil
	}
	f.Status = domain.StatusDeleted
	f.UpdatedAt = at
	return true, nil
}

type fakeStorage struct {
	putURL      string
	getURL      string
	head        *s3.ObjectInfo
	headErr     error
	deleted     []string
	deleteErr   error
	uploadID    string
	completeErr error
	aborted     []string
	abortErr    error
}

func (s *fakeStorage) InitiateMultipartUpload(context.Context, string, string) (string, error) {
	if s.uploadID != "" {
		return s.uploadID, nil
	}
	return "upload-1", nil
}

func (s *fakeStorage) GenerateUploadPartPresignedURL(_ context.Context, _ string, _ string, partNumber int32, _ int64) (s3.PresignedURLResult, error) {
	return s3.PresignedURLResult{URL: fmt.Sprintf("https://rustfs.example/part/%d", partNumber), ExpireAt: time.Now().Add(15 * time.Minute).UnixMilli()}, nil
}

func (s *fakeStorage) CompleteMultipartUpload(context.Context, string, string, []s3.MultipartCompletedPart) (string, error) {
	return "", s.completeErr
}

func (s *fakeStorage) AbortMultipartUpload(_ context.Context, key, _ string) error {
	s.aborted = append(s.aborted, key)
	return s.abortErr
}

func (s *fakeStorage) GeneratePostFormPresignedData(context.Context, string, string) (s3.PostFormPresignedResult, error) {
	return s3.PostFormPresignedResult{}, errors.New("not implemented")
}

func (s *fakeStorage) GeneratePutObjectPresignedURL(context.Context, string, string, int64) (s3.PresignedURLResult, error) {
	return s3.PresignedURLResult{URL: s.putURL, ExpireAt: time.Now().Add(15 * time.Minute).UnixMilli()}, nil
}

func (s *fakeStorage) GenerateGetObjectPresignedURL(context.Context, string, int64) (s3.PresignedURLResult, error) {
	return s3.PresignedURLResult{URL: s.getURL, ExpireAt: time.Now().Add(15 * time.Minute).UnixMilli()}, nil
}

func (s *fakeStorage) HeadObject(context.Context, string) (*s3.ObjectInfo, error) {
	return s.head, s.headErr
}

func (s *fakeStorage) DeleteObject(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return s.deleteErr
}

func (s *fakeStorage) ListObjects(context.Context, string) ([]s3.ObjectInfo, error) {
	return nil, errors.New("not implemented")
}

type fakeAPIError struct{ code string }

func (e fakeAPIError) Error() string                 { return e.code }
func (e fakeAPIError) ErrorCode() string             { return e.code }
func (e fakeAPIError) ErrorMessage() string          { return e.code }
func (e fakeAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

// fakeGroupMembers 是群成员查询端口的可控实现。
type fakeGroupMembers struct {
	members map[string][]string
	err     error
	calls   int
}

func (f *fakeGroupMembers) MemberUserIDs(_ context.Context, groupID string) ([]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.members[groupID], nil
}

func newTestService(repo *fakeRepo, storage s3.S3Client, now time.Time) *Service {
	return newTestServiceWithGroups(repo, storage, now, &fakeGroupMembers{})
}

func newTestServiceWithGroups(repo *fakeRepo, storage s3.S3Client, now time.Time, groups GroupMembers) *Service {
	svc := NewService(repo, storage, Config{
		PresignExpire: 15 * time.Minute,
		PendingTTL:    24 * time.Hour,
		PublicObjectURL: func(objectKey string) string {
			if !domain.IsPublicObjectKey(objectKey) {
				return ""
			}
			return "https://files.example.com/goim-business/" + objectKey
		},
		GroupMembers: groups,
	}, zap.NewNop().Sugar())
	svc.now = func() time.Time { return now }
	return svc
}

func errorCode(t *testing.T, err error) int {
	t.Helper()
	var bizErr *errcode.Error
	require.ErrorAs(t, err, &bizErr)
	return bizErr.Code
}

func TestPresignUpload(t *testing.T) {
	repo := newFakeRepo()
	storage := &fakeStorage{putURL: "https://rustfs.example/put"}
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	svc := newTestService(repo, storage, now)

	res, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose:  domain.PurposeAvatar,
		FileName: "me.png",
		MIME:     "image/png",
		Size:     120000,
	})
	require.NoError(t, err)
	require.Equal(t, "1", res.FileID)
	require.Equal(t, "PUT", res.Method)
	require.Equal(t, "https://rustfs.example/put", res.UploadURL)
	require.Contains(t, res.ObjectKey, "public/avatar/42/")
	require.True(t, strings.HasSuffix(fileSuffix(res.ObjectKey), ".png"))
	require.NotZero(t, res.ExpiresAt)

	file, ok := repo.files[1]
	require.True(t, ok)
	require.Equal(t, domain.StatusPending, file.Status)
	require.Equal(t, int64(42), file.UserID)
	require.Equal(t, "public/avatar/42/", file.ObjectKey[:len("public/avatar/42/")])
	require.True(t, file.ExpiresAt.Equal(now.Add(24*time.Hour)))
}

func TestPresignUploadAttachmentWithinSingleLimit(t *testing.T) {
	repo := newFakeRepo()
	storage := &fakeStorage{putURL: "https://rustfs.example/put"}
	svc := newTestService(repo, storage, time.Now())

	res, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose:  domain.PurposeAttachment,
		FileName: "report.pdf",
		MIME:     "application/pdf",
		Size:     domain.MaxSingleUploadBytes(domain.PurposeAttachment),
	})
	require.NoError(t, err)
	require.Contains(t, res.ObjectKey, "attachment/42/")
	require.Equal(t, domain.PurposeAttachment, repo.files[1].Purpose)
	require.Equal(t, domain.StatusPending, repo.files[1].Status)

	_, err = svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeAttachment,
		MIME:    "application/pdf",
		Size:    domain.MaxSingleUploadBytes(domain.PurposeAttachment) + 1,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestPresignUploadValidation(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeStorage{}, time.Now())

	_, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeAvatar,
		MIME:    "image/gif",
		Size:    100,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))

	_, err = svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeAvatar,
		MIME:    "image/png",
		Size:    domain.MaxAvatarBytes + 1,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestCompleteSuccess(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAvatar, ObjectKey: "public/avatar/42/a.png",
		MIME: "image/png", Status: domain.StatusPending,
	}
	storage := &fakeStorage{head: &s3.ObjectInfo{Key: "public/avatar/42/a.png", Size: 1000, ContentType: "image/png"}}
	svc := newTestService(repo, storage, time.Now())

	view, err := svc.Complete(context.Background(), 42, "7")
	require.NoError(t, err)
	require.Equal(t, "7", view.FileID)
	require.Equal(t, int64(1000), view.Size)
	require.Equal(t, domain.StatusReady, view.Status)
	require.Equal(t, domain.StatusReady, repo.files[7].Status)
}

func TestCompleteRejectsForeignAndOversize(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAvatar, ObjectKey: "public/avatar/42/a.png",
		MIME: "image/png", Status: domain.StatusPending,
	}
	storage := &fakeStorage{head: &s3.ObjectInfo{Size: domain.MaxAvatarBytes + 1, ContentType: "image/png"}}
	svc := newTestService(repo, storage, time.Now())

	_, err := svc.Complete(context.Background(), 99, "7")
	require.Equal(t, errcode.CodeNotFound, errorCode(t, err))

	_, err = svc.Complete(context.Background(), 42, "7")
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestCompleteMissingObject(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAvatar, ObjectKey: "public/avatar/42/a.png",
		MIME: "image/png", Status: domain.StatusPending,
	}
	storage := &fakeStorage{headErr: fakeAPIError{code: "NotFound"}}
	svc := newTestService(repo, storage, time.Now())

	_, err := svc.Complete(context.Background(), 42, "7")
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestURLRequiresReady(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAvatar, ObjectKey: "public/avatar/42/a.png",
		MIME: "image/png", Status: domain.StatusPending,
	}
	storage := &fakeStorage{getURL: "https://rustfs.example/get"}
	svc := newTestService(repo, storage, time.Now())

	_, err := svc.URL(context.Background(), 42, "7")
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))

	repo.files[7].Status = domain.StatusReady
	res, err := svc.URL(context.Background(), 42, "7")
	require.NoError(t, err)
	require.Equal(t, "https://rustfs.example/get", res.URL)
}

func TestPresignUploadChatImage(t *testing.T) {
	repo := newFakeRepo()
	storage := &fakeStorage{putURL: "https://rustfs.example/put"}
	svc := newTestService(repo, storage, time.Now())

	res, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose:        domain.PurposeChatImage,
		ConversationID: "si_99_42",
		FileName:       "pic.gif",
		MIME:           "image/gif",
		Size:           domain.MaxChatImageBytes,
	})
	require.NoError(t, err)
	require.Contains(t, res.ObjectKey, "chat_image/42/")
	require.NotContains(t, res.ObjectKey, "public/")
	require.True(t, strings.HasSuffix(res.ObjectKey, ".gif"))
	require.Equal(t, domain.PurposeChatImage, repo.files[1].Purpose)
	require.Equal(t, "si_99_42", repo.files[1].ConversationID, "conversationID 由 goim 权威生成，biz 原样保留")

	// 超过 8MB 直接拒绝，聊天图片不支持分片上传。
	_, err = svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose:        domain.PurposeChatImage,
		ConversationID: "si_42_99",
		MIME:           "image/png",
		Size:           domain.MaxChatImageBytes + 1,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))

	// 非图片类型拒绝。
	_, err = svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose:        domain.PurposeChatImage,
		ConversationID: "si_42_99",
		MIME:           "application/pdf",
		Size:           100,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestChatUploadRequiresConversation(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeStorage{putURL: "https://rustfs.example/put"}, time.Now())

	_, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeChatImage, MIME: "image/png", Size: 100,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))

	_, err = svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeChatFile, MIME: "audio/mpeg", Size: 100,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestNonChatUploadRejectsConversation(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeStorage{putURL: "https://rustfs.example/put"}, time.Now())

	_, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeAvatar, ConversationID: "si_42_99", MIME: "image/png", Size: 100,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))

	_, err = svc.MultipartPresign(context.Background(), 42, types.MultipartPresignRequest{
		Purpose: domain.PurposeAttachment, ConversationID: "g_1", MIME: "video/mp4", Size: 9 * 1024 * 1024,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestSingleChatUploadRequiresOwnerInConversation(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeStorage{putURL: "https://rustfs.example/put"}, time.Now())

	_, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeChatImage, ConversationID: "si_1_2", MIME: "image/png", Size: 100,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestGroupChatUploadRequiresMembership(t *testing.T) {
	groups := &fakeGroupMembers{members: map[string][]string{"1001": {"42", "99"}}}
	svc := newTestServiceWithGroups(newFakeRepo(), &fakeStorage{putURL: "https://rustfs.example/put"}, time.Now(), groups)

	res, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeChatImage, ConversationID: "g_1001", MIME: "image/png", Size: 100,
	})
	require.NoError(t, err)
	require.Contains(t, res.ObjectKey, "chat_image/42/")
	require.Equal(t, 1, groups.calls)

	_, err = svc.PresignUpload(context.Background(), 77, types.FilePresignRequest{
		Purpose: domain.PurposeChatImage, ConversationID: "g_1001", MIME: "image/png", Size: 100,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err), "非群成员不允许上传")
}

func TestGroupChatUploadFailsClosedWhenIMUnavailable(t *testing.T) {
	groups := &fakeGroupMembers{err: errors.New("im down")}
	svc := newTestServiceWithGroups(newFakeRepo(), &fakeStorage{putURL: "https://rustfs.example/put"}, time.Now(), groups)

	_, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeChatImage, ConversationID: "g_1001", MIME: "image/png", Size: 100,
	})
	require.Equal(t, errcode.CodeDependency, errorCode(t, err))
}

func TestChatFileSingleAndMultipart(t *testing.T) {
	repo := newFakeRepo()
	storage := &fakeStorage{putURL: "https://rustfs.example/put", uploadID: "upload-cf"}
	svc := newTestService(repo, storage, time.Now())

	// 单文件 ≤8MB：私有命名空间，支持音频等资源。
	res, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeChatFile, ConversationID: "si_42_99", MIME: "audio/mpeg", Size: 1000,
	})
	require.NoError(t, err)
	require.Contains(t, res.ObjectKey, "chat_file/42/")
	require.NotContains(t, res.ObjectKey, "public/")
	require.True(t, strings.HasSuffix(res.ObjectKey, ".mp3"))

	// >8MB 必须走分片。
	const size = 9 * 1024 * 1024
	_, err = svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeChatFile, ConversationID: "si_42_99", MIME: "video/mp4", Size: size,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))

	mp, err := svc.MultipartPresign(context.Background(), 42, types.MultipartPresignRequest{
		Purpose: domain.PurposeChatFile, ConversationID: "si_42_99", MIME: "video/mp4", Size: size,
	})
	require.NoError(t, err)
	require.Contains(t, mp.ObjectKey, "chat_file/42/")
	require.NotContains(t, mp.ObjectKey, "public/")
	require.Equal(t, int32(2), mp.PartCount)
	require.Equal(t, "upload-cf", mp.UploadID)

	storage.head = &s3.ObjectInfo{Size: size, ContentType: "video/mp4"}
	view, err := svc.MultipartComplete(context.Background(), 42, mp.FileID, types.MultipartCompleteRequest{
		Parts: []types.MultipartCompletedPart{
			{PartNumber: 1, ETag: `"etag-1"`},
			{PartNumber: 2, ETag: `"etag-2"`},
		},
	})
	require.NoError(t, err)
	require.Equal(t, domain.StatusReady, view.Status)
	require.Empty(t, view.URL, "聊天文件不应返回公开 URL")

	// 超过 200MB 拒绝。
	_, err = svc.MultipartPresign(context.Background(), 42, types.MultipartPresignRequest{
		Purpose: domain.PurposeChatFile, ConversationID: "si_42_99", MIME: "video/mp4", Size: domain.MaxChatFileBytes + 1,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestCompleteChatImageReturnsNoURL(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeChatImage, ConversationID: "si_42_99",
		ObjectKey: "chat_image/42/a.jpg", MIME: "image/jpeg", Status: domain.StatusPending,
	}
	storage := &fakeStorage{head: &s3.ObjectInfo{Size: 1000, ContentType: "image/jpeg"}}
	svc := newTestService(repo, storage, time.Now())

	view, err := svc.Complete(context.Background(), 42, "7")
	require.NoError(t, err)
	require.Equal(t, domain.StatusReady, view.Status)
	require.Empty(t, view.URL, "聊天图片应在访问时按会话成员现签")

	// 重复 complete 幂等返回。
	again, err := svc.Complete(context.Background(), 42, "7")
	require.NoError(t, err)
	require.Empty(t, again.URL)
}

func TestCompleteAttachmentReturnsNoPublicURL(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAttachment, ObjectKey: "attachment/42/a.pdf",
		MIME: "application/pdf", Status: domain.StatusPending,
	}
	storage := &fakeStorage{head: &s3.ObjectInfo{Size: 1000, ContentType: "application/pdf"}}
	svc := newTestService(repo, storage, time.Now())

	view, err := svc.Complete(context.Background(), 42, "7")
	require.NoError(t, err)
	require.Empty(t, view.URL, "私有用途不应返回匿名地址")
}

func TestURLAvatarAllowsNonOwner(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAvatar, ObjectKey: "public/avatar/42/a.png",
		MIME: "image/png", Status: domain.StatusReady,
	}
	storage := &fakeStorage{getURL: "https://rustfs.example/get"}
	svc := newTestService(repo, storage, time.Now())

	res, err := svc.URL(context.Background(), 99, "7")
	require.NoError(t, err, "avatar 允许任意登录用户换取访问地址")
	require.Equal(t, "https://rustfs.example/get", res.URL)
}

func TestURLAttachmentOwnerOnly(t *testing.T) {
	repo := newFakeRepo()
	repo.files[8] = &domain.File{
		ID: 8, UserID: 42, Purpose: domain.PurposeAttachment, ObjectKey: "attachment/42/a.pdf",
		MIME: "application/pdf", Status: domain.StatusReady,
	}
	storage := &fakeStorage{getURL: "https://rustfs.example/get"}
	svc := newTestService(repo, storage, time.Now())

	_, err := svc.URL(context.Background(), 99, "8")
	require.Equal(t, errcode.CodeNotFound, errorCode(t, err))

	_, err = svc.URL(context.Background(), 42, "8")
	require.NoError(t, err)
}

func TestURLSingleChatMembersOnly(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeChatImage, ConversationID: "si_42_99",
		ObjectKey: "chat_image/42/a.jpg", MIME: "image/jpeg", Status: domain.StatusReady,
	}
	storage := &fakeStorage{getURL: "https://rustfs.example/get"}
	svc := newTestService(repo, storage, time.Now())

	_, err := svc.URL(context.Background(), 42, "7")
	require.NoError(t, err)

	_, err = svc.URL(context.Background(), 99, "7")
	require.NoError(t, err, "单聊对端可换取访问地址")

	_, err = svc.URL(context.Background(), 100, "7")
	require.Equal(t, errcode.CodeNotFound, errorCode(t, err), "单聊第三方不可访问")
}

func TestURLGroupChatMembersOnly(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeChatFile, ConversationID: "g_1001",
		ObjectKey: "chat_file/42/a.mp3", MIME: "audio/mpeg", Status: domain.StatusReady,
	}
	groups := &fakeGroupMembers{members: map[string][]string{"1001": {"42", "99"}}}
	storage := &fakeStorage{getURL: "https://rustfs.example/get"}
	svc := newTestServiceWithGroups(repo, storage, time.Now(), groups)

	_, err := svc.URL(context.Background(), 42, "7")
	require.NoError(t, err)

	_, err = svc.URL(context.Background(), 99, "7")
	require.NoError(t, err, "群成员可换取访问地址")

	_, err = svc.URL(context.Background(), 100, "7")
	require.Equal(t, errcode.CodeNotFound, errorCode(t, err), "非群成员不可访问")
	require.Equal(t, 2, groups.calls, "owner 命中后无需远程查询")
}

func TestURLGroupChatFailsClosedWhenIMUnavailable(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeChatImage, ConversationID: "g_1001",
		ObjectKey: "chat_image/42/a.jpg", MIME: "image/jpeg", Status: domain.StatusReady,
	}
	groups := &fakeGroupMembers{err: errors.New("im down")}
	storage := &fakeStorage{getURL: "https://rustfs.example/get"}
	svc := newTestServiceWithGroups(repo, storage, time.Now(), groups)

	_, err := svc.URL(context.Background(), 99, "7")
	require.Equal(t, errcode.CodeDependency, errorCode(t, err))

	_, err = svc.URL(context.Background(), 42, "7")
	require.NoError(t, err, "owner 不依赖 IM")
}

func TestURLHistoricalChatWithoutConversationOnlyOwner(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeChatImage,
		ObjectKey: "public/chat_image/42/a.jpg", MIME: "image/jpeg", Status: domain.StatusReady,
	}
	storage := &fakeStorage{getURL: "https://rustfs.example/get"}
	svc := newTestService(repo, storage, time.Now())

	_, err := svc.URL(context.Background(), 99, "7")
	require.Equal(t, errcode.CodeNotFound, errorCode(t, err), "历史空会话记录按私有处理")

	_, err = svc.URL(context.Background(), 42, "7")
	require.NoError(t, err)
}

func TestDeleteSoftDeletesAndIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAvatar, ObjectKey: "public/avatar/42/a.png",
		MIME: "image/png", Status: domain.StatusReady,
	}
	storage := &fakeStorage{}
	svc := newTestService(repo, storage, time.Now())

	require.NoError(t, svc.Delete(context.Background(), 42, "7"))
	require.Equal(t, domain.StatusDeleted, repo.files[7].Status)
	require.Equal(t, []string{"public/avatar/42/a.png"}, storage.deleted)

	require.NoError(t, svc.Delete(context.Background(), 42, "7"))
	require.Len(t, storage.deleted, 1)
}

func TestStorageNotConfigured(t *testing.T) {
	svc := newTestService(newFakeRepo(), nil, time.Now())

	_, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeAvatar,
		MIME:    "image/png",
		Size:    100,
	})
	require.Equal(t, errcode.CodeDependency, errorCode(t, err))
}

func TestMultipartPresignAndComplete(t *testing.T) {
	repo := newFakeRepo()
	storage := &fakeStorage{uploadID: "upload-9"}
	svc := newTestService(repo, storage, time.Now())

	const size = 10 * 1024 * 1024
	res, err := svc.MultipartPresign(context.Background(), 42, types.MultipartPresignRequest{
		Purpose: domain.PurposeAttachment,
		MIME:    "video/mp4",
		Size:    size,
	})
	require.NoError(t, err)
	require.Equal(t, "1", res.FileID)
	require.Equal(t, "upload-9", res.UploadID)
	require.Equal(t, domain.MultipartPartSize, res.PartSize)
	require.Equal(t, int32(2), res.PartCount)
	require.Len(t, res.Parts, 2)
	require.Equal(t, "https://rustfs.example/part/1", res.Parts[0].URL)

	file := repo.files[1]
	require.Equal(t, "upload-9", file.UploadID)
	require.Equal(t, domain.StatusPending, file.Status)

	storage.head = &s3.ObjectInfo{Size: size, ContentType: "video/mp4"}
	view, err := svc.MultipartComplete(context.Background(), 42, "1", types.MultipartCompleteRequest{
		Parts: []types.MultipartCompletedPart{
			{PartNumber: 1, ETag: "\"etag-1\""},
			{PartNumber: 2, ETag: "\"etag-2\""},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(size), view.Size)
	require.Equal(t, domain.StatusReady, view.Status)
	require.Equal(t, domain.StatusReady, repo.files[1].Status)
	require.Empty(t, repo.files[1].UploadID, "分片完成后应清空 upload_id，回收时无需 abort")
}

func TestMultipartRejectsInvalidParts(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAttachment, ObjectKey: "attachment/42/a.mp4",
		MIME: "video/mp4", Size: 10 * 1024 * 1024, UploadID: "upload-1", Status: domain.StatusPending,
	}
	svc := newTestService(repo, &fakeStorage{}, time.Now())

	_, err := svc.MultipartComplete(context.Background(), 42, "7", types.MultipartCompleteRequest{
		Parts: []types.MultipartCompletedPart{{PartNumber: 1, ETag: "\"etag\""}},
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))

	_, err = svc.MultipartComplete(context.Background(), 42, "7", types.MultipartCompleteRequest{
		Parts: []types.MultipartCompletedPart{
			{PartNumber: 2, ETag: "\"etag\""},
			{PartNumber: 1, ETag: "\"etag\""},
		},
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

func TestMultipartAbort(t *testing.T) {
	repo := newFakeRepo()
	storage := &fakeStorage{uploadID: "upload-1"}
	svc := newTestService(repo, storage, time.Now())

	_, err := svc.MultipartPresign(context.Background(), 42, types.MultipartPresignRequest{
		Purpose: domain.PurposeAttachment,
		MIME:    "application/pdf",
		Size:    9 * 1024 * 1024,
	})
	require.NoError(t, err)

	require.NoError(t, svc.MultipartAbort(context.Background(), 42, "1"))
	require.Equal(t, domain.StatusDeleted, repo.files[1].Status)
	require.Len(t, storage.aborted, 1)
	require.Len(t, storage.deleted, 1, "abort 后仍需删除可能已合并的对象")
}

func TestRefreshMultipartParts(t *testing.T) {
	repo := newFakeRepo()
	repo.files[7] = &domain.File{
		ID: 7, UserID: 42, Purpose: domain.PurposeAttachment, ObjectKey: "attachment/42/a.mp4",
		MIME: "video/mp4", Size: 10 * 1024 * 1024, UploadID: "upload-7", Status: domain.StatusPending,
	}
	svc := newTestService(repo, &fakeStorage{}, time.Now())

	// 指定分片：只返回请求的分片。
	res, err := svc.RefreshMultipartParts(context.Background(), 42, "7", []int32{2})
	require.NoError(t, err)
	require.Equal(t, "7", res.FileID)
	require.Equal(t, "upload-7", res.UploadID)
	require.Equal(t, domain.MultipartPartSize, res.PartSize)
	require.Equal(t, int32(2), res.PartCount)
	require.Len(t, res.Parts, 1)
	require.Equal(t, int32(2), res.Parts[0].PartNumber)

	// 未指定分片：返回全部分片。
	res, err = svc.RefreshMultipartParts(context.Background(), 42, "7", nil)
	require.NoError(t, err)
	require.Len(t, res.Parts, 2)
	require.Equal(t, int32(1), res.Parts[0].PartNumber)
	require.Equal(t, int32(2), res.Parts[1].PartNumber)

	// 越界或非严格递增的分片号拒绝。
	_, err = svc.RefreshMultipartParts(context.Background(), 42, "7", []int32{3})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
	_, err = svc.RefreshMultipartParts(context.Background(), 42, "7", []int32{2, 1})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))

	// 他人不可刷新，已 ready 的文件返回冲突。
	_, err = svc.RefreshMultipartParts(context.Background(), 99, "7", nil)
	require.Equal(t, errcode.CodeNotFound, errorCode(t, err))
	repo.files[7].Status = domain.StatusReady
	_, err = svc.RefreshMultipartParts(context.Background(), 42, "7", nil)
	require.Equal(t, errcode.CodeConflict, errorCode(t, err))
}

func TestSingleUploadRejectsLargeAttachment(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeStorage{}, time.Now())

	_, err := svc.PresignUpload(context.Background(), 42, types.FilePresignRequest{
		Purpose: domain.PurposeAttachment,
		MIME:    "application/pdf",
		Size:    10 * 1024 * 1024,
	})
	require.Equal(t, errcode.CodeInvalidParam, errorCode(t, err))
}

// fileSuffix 返回对象键中最后一个 / 之后的部分，便于断言 UUID.ext 格式。
func fileSuffix(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			return key[i+1:]
		}
	}
	return key
}
