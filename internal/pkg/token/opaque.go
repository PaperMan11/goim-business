package token

import (
	"encoding/base64"
	"fmt"

	"goim-business/internal/pkg/hashx"
	"goim-business/internal/pkg/randx"
)

const opaqueTokenBytes = 32

// NewOpaque 生成不透明令牌明文及其 SHA-256 哈希；入库只保存哈希。
func NewOpaque() (plain, digest string, err error) {
	raw, err := randx.SecureBytes(opaqueTokenBytes)
	if err != nil {
		return "", "", fmt.Errorf("生成不透明令牌失败: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(raw)
	return plain, HashOpaque(plain), nil
}

// HashOpaque 返回不透明令牌的 SHA-256 哈希，用于持久化与比对。
func HashOpaque(plain string) string {
	return hashx.Sha256String(plain)
}
