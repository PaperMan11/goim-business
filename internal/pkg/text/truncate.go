// Package text 提供与业务无关的文本处理工具。
package text

import "unicode/utf8"

// TruncateRunes 按字符（rune）截断字符串，避免按字节截断破坏多字节字符。
func TruncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
