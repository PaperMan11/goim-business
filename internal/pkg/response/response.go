// Package response 统一 HTTP 响应格式：{code,msg,data}。
package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"goim-business/internal/pkg/errcode"
)

// CodeKey 是写入 gin.Context 的业务错误码键，供访问日志读取。
const CodeKey = "response.code"

// Body 是所有接口的统一响应体。
type Body struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

// OK 输出成功响应。
func OK(c *gin.Context, data any) {
	c.Set(CodeKey, 0)
	c.JSON(http.StatusOK, Body{Code: errcode.CodeOK, Msg: "success", Data: data})
}

// Error 输出业务错误；HTTP 状态恒为 200，前端统一按 body 中的 code 判断。
func Error(c *gin.Context, err error) {
	e := toError(err)
	c.Set(CodeKey, e.Code)
	c.JSON(http.StatusOK, Body{Code: e.Code, Msg: e.Message, Data: e.Details})
}

// Abort 输出错误并中止后续 handler。
func Abort(c *gin.Context, err error) {
	Error(c, err)
	c.Abort()
}

func toError(err error) *errcode.Error {
	if err == nil {
		return errcode.ErrInternal
	}
	var bizErr *errcode.Error
	if errors.As(err, &bizErr) {
		return bizErr
	}
	return errcode.ErrInternal
}
