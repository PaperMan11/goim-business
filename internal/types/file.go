package types

import "time"

// FilePresignRequest 是申请文件直传的请求。
type FilePresignRequest struct {
	Purpose  string `json:"purpose" binding:"required,max=32"`
	FileName string `json:"fileName" binding:"max=255"`
	MIME     string `json:"mime" binding:"required,max=128"`
	Size     int64  `json:"size" binding:"required,min=1"`
	SHA256   string `json:"sha256" binding:"omitempty,len=64"`
}

// FilePresignResp 是申请上传的响应；客户端按 method + uploadUrl + headers 直传对象存储。
type FilePresignResp struct {
	FileID    string            `json:"fileId"`
	ObjectKey string            `json:"objectKey"`
	UploadURL string            `json:"uploadUrl"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt int64             `json:"expiresAt"`
}

// FileView 是对外暴露的文件元数据。
type FileView struct {
	FileID    string    `json:"fileId"`
	Purpose   string    `json:"purpose"`
	ObjectKey string    `json:"objectKey"`
	MIME      string    `json:"mime"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	Status    int8      `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

// FileURLResp 是文件访问地址响应。
type FileURLResp struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expiresAt"`
}

// MultipartPresignRequest 是申请分片上传的请求；当前仅 attachment 用途支持。
type MultipartPresignRequest struct {
	Purpose  string `json:"purpose" binding:"required,max=32"`
	FileName string `json:"fileName" binding:"max=255"`
	MIME     string `json:"mime" binding:"required,max=128"`
	Size     int64  `json:"size" binding:"required,min=1"`
	SHA256   string `json:"sha256" binding:"omitempty,len=64"`
}

// MultipartPartPresign 是单个分片的上传信息。
type MultipartPartPresign struct {
	PartNumber int32  `json:"partNumber"`
	URL        string `json:"url"`
	ExpiresAt  int64  `json:"expiresAt"`
}

// MultipartPartsRequest 是刷新分片签名的请求；partNumbers 为空表示刷新全部分片。
type MultipartPartsRequest struct {
	PartNumbers []int32 `json:"partNumbers" binding:"omitempty,min=1,max=100,dive,min=1,max=100"`
}

// MultipartPresignResp 是分片上传初始化响应；客户端逐片 PUT 到 parts[].url。
type MultipartPresignResp struct {
	FileID    string                 `json:"fileId"`
	ObjectKey string                 `json:"objectKey"`
	UploadID  string                 `json:"uploadId"`
	PartSize  int64                  `json:"partSize"`
	PartCount int32                  `json:"partCount"`
	Parts     []MultipartPartPresign `json:"parts"`
	ExpiresAt int64                  `json:"expiresAt"`
}

// MultipartCompletedPart 是客户端上报的已完成分片。
type MultipartCompletedPart struct {
	PartNumber int32  `json:"partNumber" binding:"required,min=1,max=100"`
	ETag       string `json:"etag" binding:"required,max=128"`
}

// MultipartCompleteRequest 是完成分片上传的请求。
type MultipartCompleteRequest struct {
	Parts []MultipartCompletedPart `json:"parts" binding:"required,min=1,max=100,dive"`
}
