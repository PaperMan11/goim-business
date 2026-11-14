// Package idgen 生成 UUIDv7 字符串（按时间有序，适合做主键）。
package idgen

import "github.com/google/uuid"

// New 生成一个 UUIDv7 字符串。
func New() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// MustNew 生成 UUIDv7，失败时 panic（仅用于不会失败的场景，例如请求 ID）。
func MustNew() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id.String()
}
