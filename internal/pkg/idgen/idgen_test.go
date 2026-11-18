package idgen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWithPrefix(t *testing.T) {
	id, err := NewWithPrefix(PrefixFile)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(id, PrefixFile+"_"))
	// prefix_ + UUIDv7 的 36 位
	require.Len(t, id, len(PrefixFile)+1+36)
}

func TestNewWithPrefixValidation(t *testing.T) {
	for _, prefix := range []string{"File", "file_", "a-b", "", strings.Repeat("a", 13)} {
		if prefix == "" {
			id, err := NewWithPrefix("")
			require.NoError(t, err)
			require.Len(t, id, 36)
			continue
		}
		_, err := NewWithPrefix(prefix)
		require.ErrorIs(t, err, ErrInvalidPrefix, prefix)
	}
}

func TestNewWithoutPrefix(t *testing.T) {
	id, err := New()
	require.NoError(t, err)
	require.Len(t, id, 36)
	require.NotContains(t, id, "_")
}
