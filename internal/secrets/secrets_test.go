package secrets

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// xorProt stands in for DPAPI: reversible, and never returns its input.
type xorProt struct{}

func xor(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = c ^ 0x5a
	}
	return out
}

func (xorProt) Protect(b []byte) ([]byte, error)   { return xor(b), nil }
func (xorProt) Unprotect(b []byte) ([]byte, error) { return xor(b), nil }

// passEnc values written by v0.5 are base64 of the raw DPAPI blob; any other
// encoding would make saved upstream proxy passwords unreadable.
func TestEncodeString_IsBase64OfProtected(t *testing.T) {
	got, err := EncodeString(xorProt{}, "pässwörd")
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString(xor([]byte("pässwörd"))), got)
}

func TestDecodeString_RoundTrip(t *testing.T) {
	enc, err := EncodeString(xorProt{}, "pässwörd")
	require.NoError(t, err)
	got, err := DecodeString(xorProt{}, enc)
	require.NoError(t, err)
	require.Equal(t, "pässwörd", got)
}

func TestDecodeString_RejectsEmptyAndBadBase64(t *testing.T) {
	_, err := DecodeString(xorProt{}, "")
	require.Error(t, err)
	_, err = DecodeString(xorProt{}, "%%%")
	require.Error(t, err)
}

func TestUnsupported(t *testing.T) {
	_, err := Unsupported{}.Protect([]byte("x"))
	require.ErrorIs(t, err, errors.ErrUnsupported)
	_, err = Unsupported{}.Unprotect([]byte("x"))
	require.ErrorIs(t, err, errors.ErrUnsupported)
}
