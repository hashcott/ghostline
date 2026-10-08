package zapret2_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"testing"

	zapret2 "github.com/hashcott/ghostline/assets/zapret2"
	pins "github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedFilesMatchPinnedHashes(t *testing.T) {
	for name, want := range pins.Pinned {
		b, err := fs.ReadFile(zapret2.FS, name)
		require.NoError(t, err, name)
		h := sha256.Sum256(b)
		require.Equal(t, want, hex.EncodeToString(h[:]), name)
	}
}
