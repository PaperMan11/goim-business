// Package file 定义文件对象的领域模型：用途、状态、上传校验与对象键规则。
package file

import (
	"encoding/hex"
	"errors"
	"fmt"
	"goim-business/internal/pkg/convert"
	"strings"
	"time"
)

// 文件用途，按可访问范围分为三类。
const (
	// 公开用途：对象固定落在 public/ 命名空间，可匿名读，完成后返回稳定 URL。
	PurposeAvatar = "avatar"

	// 会话用途：对象落在私有命名空间，仅会话成员可换取短时预签名 URL。
	// chat_image 为聊天图片；chat_file 为聊天文件（语音、视频、文档等）。
	PurposeChatImage = "chat_image"
	PurposeChatFile  = "chat_file"

	// 私有用途：对象仅允许本人通过预签名 URL 访问。
	PurposeAttachment = "attachment"
)

// PublicPrefix 是所有允许匿名读的对象的一级前缀；仅该命名空间可由部署侧开放公开访问。
const PublicPrefix = "public"

// 会话标识前缀，与 goim 的 conversationID 保持一致。
const (
	// SingleConversationPrefix 是单聊会话前缀，格式 si_<id1>_<id2>。
	SingleConversationPrefix = "si_"
	// GroupConversationPrefix 是普通群会话前缀，格式 g_<groupID>。
	GroupConversationPrefix = "g_"
	// SuperGroupConversationPrefix 是超级群会话前缀，格式 sg_<groupID>。
	SuperGroupConversationPrefix = "sg_"
)

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
	// MaxChatImageBytes 是聊天图片大小上限（8MB），只支持单文件 PUT。
	MaxChatImageBytes int64 = 8 * 1024 * 1024
	// MaxChatFileBytes 是聊天文件大小上限（200MB），超过 8MB 时走分片上传。
	MaxChatFileBytes int64 = 200 * 1024 * 1024
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
	// ErrConversationRequired 表示聊天资源未绑定会话。
	ErrConversationRequired = errors.New("聊天资源必须绑定会话")
	// ErrInvalidConversation 表示会话标识不合法或与用途不匹配。
	ErrInvalidConversation = errors.New("会话标识不合法")
)

// File 是文件对象元数据；对象本体存放在对象存储 bucket 中。
type File struct {
	ID      int64
	UserID  int64
	Purpose string
	// ConversationID 是聊天资源绑定的会话标识；avatar/attachment 为空。
	ConversationID string
	ObjectKey      string
	FileName       string
	MIME           string
	Size           int64
	SHA256         string
	Status         int8
	// UploadID 是 S3 分片上传 ID；单文件 PUT 上传为空，分片合并成功后清空。
	UploadID  string
	ExpiresAt *time.Time
	// ObjectDeletedAt 是对象回收完成时间；nil 表示 status=2 但对象尚未确认删除。
	ObjectDeletedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
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

// ValidateMultipartUpload 校验分片上传申请；attachment 与 chat_file 支持分片。
func ValidateMultipartUpload(purpose, mime string, size int64, sha256 string) error {
	if purpose != PurposeAttachment && purpose != PurposeChatFile {
		return ErrInvalidPurpose
	}
	if _, err := ExtForUpload(purpose, mime); err != nil {
		return err
	}
	maxSize, err := MaxSizeFor(purpose)
	if err != nil {
		return err
	}
	if size <= MaxSingleUploadBytes(purpose) || size > maxSize {
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
	case PurposeChatImage:
		return MaxChatImageBytes
	case PurposeAttachment, PurposeChatFile:
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

// IsChatPurpose 表示该用途必须绑定会话，且仅会话成员可访问。
func IsChatPurpose(purpose string) bool {
	switch purpose {
	case PurposeChatImage, PurposeChatFile:
		return true
	default:
		return false
	}
}

// BindConversation 校验用途与会话标识的绑定关系，并返回校验后的会话标识。
// 聊天用途必须绑定合法会话，单聊要求 owner 是会话两端之一；其他用途不允许传会话。
func BindConversation(purpose, raw string, ownerID int64) (string, error) {
	raw = strings.TrimSpace(raw)
	if !IsChatPurpose(purpose) {
		if raw != "" {
			return "", ErrInvalidConversation
		}
		return "", nil
	}
	if raw == "" {
		return "", ErrConversationRequired
	}
	conversationID, err := validateConversationID(raw)
	if err != nil {
		return "", err
	}
	if a, b, ok := ConversationUsers(conversationID); ok && ownerID != a && ownerID != b {
		return "", ErrInvalidConversation
	}
	return conversationID, nil
}

// validateConversationID 校验客户端回传的会话标识格式（非空、两端为正整数且不相等、groupID 字符集）。
// 会话标识由 goim 权威生成，这里不重排、不改写；但入参不可信，非法值必须拒绝，避免落库后无法解析。
func validateConversationID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(id, SingleConversationPrefix):
		parts := strings.Split(strings.TrimPrefix(id, SingleConversationPrefix), "_")
		if len(parts) != 2 {
			return "", ErrInvalidConversation
		}
		a, errA := convert.ToInt64E(parts[0])
		b, errB := convert.ToInt64E(parts[1])
		if errA != nil || errB != nil || a <= 0 || b <= 0 || a == b {
			return "", ErrInvalidConversation
		}
		return id, nil
	case strings.HasPrefix(id, SuperGroupConversationPrefix):
		return validateGroupConversation(id, SuperGroupConversationPrefix)
	case strings.HasPrefix(id, GroupConversationPrefix):
		return validateGroupConversation(id, GroupConversationPrefix)
	default:
		return "", ErrInvalidConversation
	}
}

// validateGroupConversation 校验群会话的 groupID 长度与字符集；合法时原样返回会话标识。
func validateGroupConversation(id, prefix string) (string, error) {
	groupID := strings.TrimPrefix(id, prefix)
	if len(groupID) < 1 || len(groupID) > 64 {
		return "", ErrInvalidConversation
	}
	for _, r := range groupID {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return "", ErrInvalidConversation
		}
	}
	return id, nil
}

// ConversationUsers 解析单聊会话，返回两端用户 ID；非单聊返回 ok=false。
func ConversationUsers(conversationID string) (a, b int64, ok bool) {
	if !strings.HasPrefix(conversationID, SingleConversationPrefix) {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(conversationID, SingleConversationPrefix), "_")
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, errA := convert.ToInt64E(parts[0])
	b, errB := convert.ToInt64E(parts[1])
	if errA != nil || errB != nil {
		return 0, 0, false
	}
	return a, b, true
}

// ConversationGroupID 解析群聊会话，返回 groupID；非群聊返回 ok=false。
func ConversationGroupID(conversationID string) (string, bool) {
	switch {
	case strings.HasPrefix(conversationID, SuperGroupConversationPrefix):
		return strings.TrimPrefix(conversationID, SuperGroupConversationPrefix), true
	case strings.HasPrefix(conversationID, GroupConversationPrefix):
		return strings.TrimPrefix(conversationID, GroupConversationPrefix), true
	default:
		return "", false
	}
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
func NewFile(userID int64, purpose, conversationID, fileName, objectKey, mime string, size int64, sha256 string, expiresAt, now time.Time) *File {
	return &File{
		UserID:         userID,
		Purpose:        purpose,
		ConversationID: conversationID,
		ObjectKey:      objectKey,
		FileName:       fileName,
		MIME:           NormalizeMIME(mime),
		Size:           size,
		SHA256:         strings.ToLower(sha256),
		Status:         StatusPending,
		ExpiresAt:      &expiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
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
	switch purpose {
	case PurposeAvatar:
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
	case PurposeChatImage:
		// 聊天图片只允许图片类型，不支持 PDF/视频等附件类型。
		switch m {
		case "image/jpeg":
			return "jpg", nil
		case "image/png":
			return "png", nil
		case "image/webp":
			return "webp", nil
		case "image/gif":
			return "gif", nil
		default:
			return "", ErrInvalidMIME
		}
	}
	return ExtForMIME(m)
}

// ExtForMIME 返回通用资源允许的扩展名；不支持的 MIME 返回 ErrInvalidMIME。
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
	case "audio/mpeg":
		return "mp3", nil
	case "audio/wav", "audio/x-wav":
		return "wav", nil
	case "audio/amr":
		return "amr", nil
	case "audio/mp4", "audio/x-m4a":
		return "m4a", nil
	case "audio/aac":
		return "aac", nil
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
	case PurposeChatImage:
		return MaxChatImageBytes, nil
	case PurposeChatFile:
		return MaxChatFileBytes, nil
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
