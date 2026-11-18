// Package file 定义文件对象的领域模型：用途、状态、上传校验与对象键规则。
package file

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 文件用途。
const (
	PurposeAvatar     = "avatar"
	PurposeAttachment = "attachment"
)

// PublicPrefix 是所有允许匿名读的对象的一级前缀；仅该命名空间可由部署侧开放公开访问。
const PublicPrefix = "public"

// 文件状态，与 file_objects.status 保持一致。
const (
	StatusPending int8 = 0
	StatusReady   int8 = 1
	StatusDeleted int8 = 2
)

// 上传限制。
const (
	// MaxAvatarBytes 是头像大小上限（5MB），与 client-config 的 limits.maxAvatarBytes 一致。
	MaxAvatarBytes int64 = 5 * 1024 * 1024
	// MaxAttachmentBytes 是单个附件大小上限（200MB）。
	MaxAttachmentBytes int64 = 200 * 1024 * 1024
	// MultipartPartSize 是分片大小（8MB），S3 要求除最后一片外每片不小于 5MB。
	MultipartPartSize int64 = 8 * 1024 * 1024
	// MaxMultipartParts 是分片数上限，避免一次签名返回过多 URL。
	MaxMultipartParts int32 = 100
)

// 领域错误。
var (
	ErrNotFound       = errors.New("文件不存在")
	ErrInvalidPurpose = errors.New("不支持的文件用途")
	ErrInvalidMIME    = errors.New("不支持的文件类型")
	ErrInvalidSize    = errors.New("文件大小超出限制")
	ErrInvalidSHA256  = errors.New("sha256 需为 64 位十六进制")
	ErrNotReady       = errors.New("文件尚未上传完成")
	ErrUseMultipart   = errors.New("文件超过单次上传上限，请使用分片上传")
	ErrInvalidParts   = errors.New("分片列表不合法")
)

// File 是文件对象元数据；对象本体存放在对象存储 bucket 中。
type File struct {
	ID        int64
	UserID    int64
	Purpose   string
	ObjectKey string
	FileName  string
	MIME      string
	Size      int64
	SHA256    string
	Status    int8
	// UploadID 是 S3 分片上传 ID；单文件 PUT 上传为空。
	UploadID  string
	ExpiresAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ValidateUpload 校验单文件上传申请：用途、MIME、大小与可选摘要。
func ValidateUpload(purpose, mime string, size int64, sha256 string) error {
	maxSize, err := MaxSizeFor(purpose)
	if err != nil {
		return err
	}
	if _, err := ExtForUpload(purpose, mime); err != nil {
		return err
	}
	if size <= 0 || size > maxSize {
		return ErrInvalidSize
	}
	if size > MaxSingleUploadBytes(purpose) {
		return ErrUseMultipart
	}
	return validateSHA256(sha256)
}

// ValidateMultipartUpload 校验分片上传申请；仅 attachment 支持分片。
func ValidateMultipartUpload(purpose, mime string, size int64, sha256 string) error {
	if purpose != PurposeAttachment {
		return ErrInvalidPurpose
	}
	if _, err := ExtForUpload(purpose, mime); err != nil {
		return err
	}
	if size <= MaxSingleUploadBytes(purpose) || size > MaxAttachmentBytes {
		return ErrInvalidSize
	}
	if MultipartPartCount(size) > MaxMultipartParts {
		return ErrInvalidSize
	}
	return validateSHA256(sha256)
}

// MaxSingleUploadBytes 返回单次 PUT 可接受的大小上限。
func MaxSingleUploadBytes(purpose string) int64 {
	switch purpose {
	case PurposeAvatar:
		return MaxAvatarBytes
	case PurposeAttachment:
		return MultipartPartSize
	default:
		return 0
	}
}

// MultipartPartCount 返回按 MultipartPartSize 切分的分片数。
func MultipartPartCount(size int64) int32 {
	if size <= 0 {
		return 0
	}
	return int32((size + MultipartPartSize - 1) / MultipartPartSize)
}

// IsPublicPurpose 表示该用途的对象允许匿名读；新增公开资源时只需在此登记。
func IsPublicPurpose(purpose string) bool {
	return purpose == PurposeAvatar
}

// IsPublicObjectKey 判断对象 key 是否位于公开命名空间。
func IsPublicObjectKey(objectKey string) bool {
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	return strings.HasPrefix(key, PublicPrefix+"/")
}

// BuildObjectKey 统一生成对象 key：公开资源落在 public/<purpose>/<ownerID>/，私有资源落在 <purpose>/<ownerID>/。
func BuildObjectKey(purpose string, ownerID int64, token, ext string) string {
	if IsPublicPurpose(purpose) {
		return fmt.Sprintf("%s/%s/%d/%s.%s", PublicPrefix, purpose, ownerID, token, ext)
	}
	return fmt.Sprintf("%s/%d/%s.%s", purpose, ownerID, token, ext)
}

// NewFile 构造一条待上传记录；调用方需先完成校验并生成 objectKey。
func NewFile(userID int64, purpose, fileName, objectKey, mime string, size int64, sha256 string, expiresAt, now time.Time) *File {
	return &File{
		UserID:    userID,
		Purpose:   purpose,
		ObjectKey: objectKey,
		FileName:  fileName,
		MIME:      NormalizeMIME(mime),
		Size:      size,
		SHA256:    strings.ToLower(sha256),
		Status:    StatusPending,
		ExpiresAt: &expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NormalizeMIME 统一小写并去掉参数。
func NormalizeMIME(mime string) string {
	if idx := strings.IndexByte(mime, ';'); idx >= 0 {
		mime = mime[:idx]
	}
	return strings.ToLower(strings.TrimSpace(mime))
}

// ExtForUpload 返回用途允许的扩展名；不同类型用途的白名单不同。
func ExtForUpload(purpose, mime string) (string, error) {
	m := NormalizeMIME(mime)
	if purpose == PurposeAvatar {
		switch m {
		case "image/jpeg":
			return "jpg", nil
		case "image/png":
			return "png", nil
		case "image/webp":
			return "webp", nil
		default:
			return "", ErrInvalidMIME
		}
	}
	return ExtForMIME(m)
}

// ExtForMIME 返回附件允许的扩展名；不支持的 MIME 返回 ErrInvalidMIME。
func ExtForMIME(mime string) (string, error) {
	switch NormalizeMIME(mime) {
	case "image/jpeg":
		return "jpg", nil
	case "image/png":
		return "png", nil
	case "image/webp":
		return "webp", nil
	case "image/gif":
		return "gif", nil
	case "application/pdf":
		return "pdf", nil
	case "application/zip":
		return "zip", nil
	case "text/plain":
		return "txt", nil
	case "video/mp4":
		return "mp4", nil
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx", nil
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx", nil
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "pptx", nil
	default:
		return "", ErrInvalidMIME
	}
}

// MaxSizeFor 返回指定用途的大小上限。
func MaxSizeFor(purpose string) (int64, error) {
	switch purpose {
	case PurposeAvatar:
		return MaxAvatarBytes, nil
	case PurposeAttachment:
		return MaxAttachmentBytes, nil
	default:
		return 0, ErrInvalidPurpose
	}
}

func validateSHA256(sha256 string) error {
	if sha256 == "" {
		return nil
	}
	decoded, err := hex.DecodeString(sha256)
	if err != nil || len(decoded) != 32 {
		return ErrInvalidSHA256
	}
	return nil
}
