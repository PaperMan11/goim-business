// Package errcode 定义稳定的业务错误码。
package errcode

// 通用段 1001-1099
const (
	CodeOK              = 0
	CodeInvalidParam    = 1001
	CodeUnauthenticated = 1002
	CodeForbidden       = 1003
	CodeNotFound        = 1004
	CodeConflict        = 1005
	CodeRateLimited     = 1029
)

// 账号段 2001-2099
const (
	CodeAccountExists     = 2001
	CodeInvalidCredential = 2002
	CodeAccountFrozen     = 2003
	CodeAccessExpired     = 2004
	CodeRefreshReplay     = 2005
	CodeSessionLimit      = 2006
)

// 内部段 9001-9099
const (
	CodeInternal   = 9001
	CodeDependency = 9002
)

// Error 是业务错误，handler 层统一通过 response.Error 输出。
type Error struct {
	Code    int
	Message string
	Details any
}

func (e *Error) Error() string { return e.Message }

// New 创建一个业务错误。
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WithMessage 返回带新提示的副本，便于在保留错误码的前提下补充上下文。
func (e *Error) WithMessage(message string) *Error {
	return &Error{Code: e.Code, Message: message, Details: e.Details}
}

// WithDetails 返回带明细（例如字段级校验错误）的副本。
func (e *Error) WithDetails(details any) *Error {
	return &Error{Code: e.Code, Message: e.Message, Details: details}
}

// InvalidParam 返回带原始提示的参数错误副本。
func InvalidParam(err error) *Error {
	return ErrInvalidParam.WithMessage(err.Error())
}

var (
	ErrInvalidParam    = New(CodeInvalidParam, "参数错误")
	ErrUnauthenticated = New(CodeUnauthenticated, "未认证或登录态已失效")
	ErrForbidden       = New(CodeForbidden, "无权限")
	ErrNotFound        = New(CodeNotFound, "资源不存在")
	ErrConflict        = New(CodeConflict, "资源冲突")
	ErrRateLimited     = New(CodeRateLimited, "请求过于频繁，请稍后重试")

	ErrAccountExists     = New(CodeAccountExists, "账号已存在")
	ErrInvalidCredential = New(CodeInvalidCredential, "账号或密码错误")
	ErrAccountFrozen     = New(CodeAccountFrozen, "账号已被冻结")
	ErrAccessExpired     = New(CodeAccessExpired, "access token 已过期")
	ErrRefreshReplay     = New(CodeRefreshReplay, "刷新令牌已失效，请重新登录")
	ErrSessionLimit      = New(CodeSessionLimit, "会话数量超出上限")

	ErrInternal   = New(CodeInternal, "服务内部错误")
	ErrDependency = New(CodeDependency, "依赖服务不可用，请稍后重试")
)
