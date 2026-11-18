package s3

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound 判断 S3 错误是否为对象或上传任务不存在（HeadObject / DeleteObject / AbortMultipartUpload 等）。
func IsNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket", "NoSuchUpload":
			return true
		}
	}
	return false
}

// IsInvalidMultipart 判断是否为分片数据问题（分片缺失、顺序错误或分片过小），需要客户端重新上传。
func IsInvalidMultipart(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "InvalidPart", "InvalidPartOrder", "EntityTooSmall":
			return true
		}
	}
	return false
}
