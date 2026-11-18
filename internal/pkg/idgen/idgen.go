// Package idgen 生成带业务前缀的 UUIDv7 字符串（按时间有序，便于区分来源）。
package idgen

import (
	"errors"
	"regexp"

	"github.com/google/uuid"
)

// 业务前缀：不同调用方使用不同前缀，生成的 ID 可直接看出归属。
const (
	PrefixRequest   = "req"
	PrefixOperation = "op"
	PrefixFile      = "file"
	PrefixAccount   = "acc"
)

var (
	prefixPattern = regexp.MustCompile(`^[a-z0-9]{1,12}$`)
	// ErrInvalidPrefix 表示前缀不符合约定。
	ErrInvalidPrefix = errors.New("prefix 需为 1-12 位小写字母或数字")
)

// NewWithPrefix 生成 prefix_uuidv7；prefix 为空时退化为纯 UUID。
// prefix 仅允许小写字母与数字，长度 1-12，避免与分隔符、日志检索冲突。
func NewWithPrefix(prefix string) (string, error) {
	if prefix != "" && !prefixPattern.MatchString(prefix) {
		return "", ErrInvalidPrefix
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	if prefix == "" {
		return id.String(), nil
	}
	return prefix + "_" + id.String(), nil
}

// New 生成纯 UUIDv7，保留给有固定长度约束的场景（例如 CHAR(36) 列）。
func New() (string, error) {
	return NewWithPrefix("")
}

// MustNew 生成纯 UUIDv7，失败时 panic（仅用于不会失败的场景，例如请求 ID）。
func MustNew() string {
	return MustNewWithPrefix("")
}

// MustNewWithPrefix 生成前缀 ID，失败时 panic；仅用于前缀合法且不会失败的场景。
func MustNewWithPrefix(prefix string) string {
	id, err := NewWithPrefix(prefix)
	if err != nil {
		panic(err)
	}
	return id
}
