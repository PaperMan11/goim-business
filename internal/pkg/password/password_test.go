package password

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("Passw0rd!")
	require.NoError(t, err)
	require.Contains(t, hash, "$argon2id$v=19$m=65536,t=3,p=4$")

	ok, err := Verify(hash, "Passw0rd!")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = Verify(hash, "wrong-password")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestHashUsesRandomSalt(t *testing.T) {
	first, err := Hash("Passw0rd!")
	require.NoError(t, err)
	second, err := Hash("Passw0rd!")
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestVerifyInvalidHash(t *testing.T) {
	_, err := Verify("not-a-phc-string", "any")
	require.ErrorIs(t, err, ErrInvalidHash)
}
