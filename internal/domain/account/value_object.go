package account

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var loginNamePattern = regexp.MustCompile(`^[a-z0-9_]{3,64}$`)

// NewLoginName 归一化并校验登录名：去空格、转小写，仅允许 3-64 位字母数字下划线。
func NewLoginName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if !loginNamePattern.MatchString(name) {
		return "", ErrInvalidLoginName
	}
	return name, nil
}

// NewPassword 校验明文密码：8-64 字节且同时包含字母与数字。
func NewPassword(raw string) (string, error) {
	if len(raw) < 8 || len(raw) > 64 {
		return "", ErrPasswordLength
	}
	var hasLetter, hasDigit bool
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return "", ErrPasswordComplexity
	}
	return raw, nil
}

// NewNickname 归一化昵称：去空格，为空则回退登录名，最长 64 个字符。
func NewNickname(raw, loginName string) (string, error) {
	nickname := strings.TrimSpace(raw)
	if nickname == "" {
		nickname = loginName
	}
	if utf8.RuneCountInString(nickname) > 64 {
		return "", ErrInvalidNickname
	}
	return nickname, nil
}

// NewDeviceID 校验设备标识：非空且最长 64 个字符。
func NewDeviceID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" || utf8.RuneCountInString(id) > 64 {
		return "", ErrInvalidDeviceID
	}
	return id, nil
}
