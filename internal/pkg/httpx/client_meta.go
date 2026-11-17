package httpx

import (
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"goim-business/internal/pkg/errcode"
)

// DeviceIDFrom 读取 X-Device-Id；设备标识必填，最长 64 个字符。
func DeviceIDFrom(c *gin.Context) (string, error) {
	raw := strings.TrimSpace(c.GetHeader("X-Device-Id"))
	if raw == "" || utf8.RuneCountInString(raw) > 64 {
		return "", errcode.ErrInvalidParam.WithMessage("缺少或非法的 X-Device-Id")
	}
	return raw, nil
}

// ClientVersionFrom 读取 X-Client-Version；可选，最长 32 个字符。
func ClientVersionFrom(c *gin.Context) (string, error) {
	raw := strings.TrimSpace(c.GetHeader("X-Client-Version"))
	if utf8.RuneCountInString(raw) > 32 {
		return "", errcode.ErrInvalidParam.WithMessage("X-Client-Version 最长 32 个字符")
	}
	return raw, nil
}
