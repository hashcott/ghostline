package secrets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDPAPI_RoundTrip(t *testing.T) {
	p := NewUserDPAPI()
	enc, err := EncodeString(p, "s3cret ✓")
	require.NoError(t, err)
	require.NotContains(t, enc, "s3cret")
	got, err := DecodeString(p, enc)
	require.NoError(t, err)
	require.Equal(t, "s3cret ✓", got)

	_, err = DecodeString(p, "not base64 !!")
	require.Error(t, err)
	_, err = DecodeString(p, "aGVsbG8=") // valid base64, not a DPAPI blob
	require.Error(t, err)
}

func TestProtectMachine_RoundTrip(t *testing.T) {
	p := NewMachineDPAPI()
	enc, err := p.Protect([]byte("key bytes"))
	require.NoError(t, err)
	require.NotContains(t, string(enc), "key bytes")
	got, err := p.Unprotect(enc)
	require.NoError(t, err)
	require.Equal(t, []byte("key bytes"), got)
	_, err = p.Unprotect([]byte("junk"))
	require.Error(t, err)
}
