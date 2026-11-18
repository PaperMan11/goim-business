// Package s3 封装 S3 兼容对象存储（RustFS / MinIO / AWS S3）的访问，
// 只负责签名、对象管理与元数据查询，不代理文件字节。
package s3

import "context"

// HeaderKV 表示一个 HTTP header 项（一对多值），例如 Content-Type、x-amz-meta-*。
// 使用切片而非 map，因为 header 允许多个同名 key。
type HeaderKV struct {
	Key    string
	Values []string
}

// PresignedURLResult 描述一次预签名 URL 生成结果：
// 客户端直接 PUT/POST/GET url 即可完成操作，签名已内嵌在 query 中。
type PresignedURLResult struct {
	// URL 是带签名 query string 的完整 URL，可直接使用。
	URL string
	// Headers 是客户端必须额外携带的 HTTP header（例如 Content-Type）。
	Headers []HeaderKV
	// ExpireAt 是预签名 URL 的过期时间（Unix 毫秒）。
	ExpireAt int64
}

// PostFormPresignedResult 是 S3 POST form 上传的返回值。
// 客户端构造 multipart/form-data：action=URL，带上 FormData 的全部键值，最后附文件字段 file。
type PostFormPresignedResult struct {
	URL      string
	FormData map[string]string
	Headers  []HeaderKV
	ExpireAt int64
}

// MultipartCompletedPart 描述 CompleteMultipartUpload 的一个分片。
type MultipartCompletedPart struct {
	PartNumber int32
	ETag       string
}

// ObjectInfo 描述对象基础元数据。
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified int64 // Unix 毫秒
	ContentType  string
}

// S3Client 是对象存储适配层的统一接口，屏蔽 RustFS / MinIO / AWS S3 的差异。
type S3Client interface {
	// InitiateMultipartUpload 发起分片上传，返回 S3 uploadID。
	// key 是对象在 bucket 内的完整路径，例如 "avatar/123/abc.png"。
	InitiateMultipartUpload(ctx context.Context, key, contentType string) (uploadID string, err error)

	// GenerateUploadPartPresignedURL 为指定分片号生成预签名 PUT URL；expireSeconds 为 0 时使用配置值。
	GenerateUploadPartPresignedURL(ctx context.Context, key, uploadID string, partNumber int32, expireSeconds int64) (PresignedURLResult, error)

	// CompleteMultipartUpload 合并已上传分片；completedParts 由客户端在分片 PUT 成功后收集。
	CompleteMultipartUpload(ctx context.Context, key, uploadID string, completedParts []MultipartCompletedPart) (finalURL string, err error)

	// AbortMultipartUpload 放弃未完成的分片上传，释放已上传分片占用的存储。
	AbortMultipartUpload(ctx context.Context, key, uploadID string) error

	// GeneratePostFormPresignedData 生成 S3 POST form 直传所需的全部参数。
	GeneratePostFormPresignedData(ctx context.Context, key, contentType string) (PostFormPresignedResult, error)

	// GeneratePutObjectPresignedURL 生成单文件 PUT 直传的预签名 URL；expireSeconds 为 0 时使用配置值。
	GeneratePutObjectPresignedURL(ctx context.Context, key, contentType string, expireSeconds int64) (PresignedURLResult, error)

	// GenerateGetObjectPresignedURL 生成带过期时间的 GET 预签名 URL；expireSeconds 为 0 时使用配置值。
	GenerateGetObjectPresignedURL(ctx context.Context, key string, expireSeconds int64) (PresignedURLResult, error)

	// HeadObject 读取对象元数据。
	HeadObject(ctx context.Context, key string) (*ObjectInfo, error)

	// DeleteObject 删除指定对象。
	DeleteObject(ctx context.Context, key string) error

	// ListObjects 列出指定 prefix 下的所有对象。
	ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error)
}
