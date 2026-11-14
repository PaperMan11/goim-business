// Package httpx 提供 HTTP 传输层通用的请求解析与客户端上下文辅助。
package httpx

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"goim-business/internal/pkg/errcode"
)

// BindJSON 解析 JSON 请求体；失败时返回参数错误并附带字段级明细。
func BindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return errcode.ErrInvalidParam.WithDetails(BindDetails(err))
	}
	return nil
}

// BindDetails 把校验错误转成字段级明细，避免把框架细节直接暴露给客户端。
func BindDetails(err error) any {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return nil
	}
	details := make(map[string]string, len(validationErrs))
	for _, fieldErr := range validationErrs {
		details[fieldErr.Field()] = fieldErr.Tag()
	}
	return details
}
