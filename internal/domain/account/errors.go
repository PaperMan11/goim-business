package account

import "errors"

// 领域错误：只表达业务语义，由应用层翻译为对外错误码与中文提示。
var (
	ErrNotFound  = errors.New("记录不存在")
	ErrDuplicate = errors.New("唯一键冲突")

	ErrInvalidLoginName   = errors.New("登录名需为 3-64 位小写字母、数字或下划线")
	ErrPasswordLength     = errors.New("密码长度需为 8-64 位")
	ErrPasswordComplexity = errors.New("密码需同时包含字母和数字")
	ErrInvalidNickname    = errors.New("昵称长度不能超过 64 个字符")
	ErrInvalidDeviceID    = errors.New("deviceId 需为 1-64 个字符")

	ErrAccountFrozen   = errors.New("账号已被冻结")
	ErrAccountDisabled = errors.New("账号不可用")
)
