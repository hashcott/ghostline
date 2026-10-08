package goodbyedpi_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"testing"

	goodbyedpi "github.com/hashcott/ghostline/assets/goodbyedpi"
	engine "github.com/hashcott/ghostline/internal/dpi/goodbyedpi"
	"github.com/stretchr/testify/require"
)

// This test binary embeds GoodbyeDPI and WinDivert; Smart App Control may
// refuse to run it. CI runs it on GitHub's Windows runners.
func TestEmbeddedFilesMatchPinnedHashes(t *testing.T) {
	for name, want := range engine.Pinned {
		b, err := fs.ReadFile(goodbyedpi.FS, name)
		require.NoError(t, err, name)
		h := sha256.Sum256(b)
		require.Equal(t, want, hex.EncodeToString(h[:]), name)
	}
}
