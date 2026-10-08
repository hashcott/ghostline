package core

import (
	"crypto/ed25519"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	builtinStrategies "github.com/hashcott/ghostline/assets/strategies"
	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/store"
)

// strategyBox holds the zapret2 strategy list in use: the built-in one, or
// the downloaded one when it is signed, newer and valid.
type strategyBox struct {
	paths store.Paths
	pub   ed25519.PublicKey
	log   *slog.Logger
	mu    sync.Mutex
	cur   strategies.List
}

func newStrategyBox(p store.Paths, pub ed25519.PublicKey, log *slog.Logger) *strategyBox {
	b := &strategyBox{paths: p, pub: pub, log: log}
	b.reload()
	return b
}

func (b *strategyBox) reload() {
	var remote, sig []byte
	if raw, err := os.ReadFile(b.paths.DPIStrategies); err == nil {
		remote = raw
		if sig, err = os.ReadFile(b.paths.DPIStrategiesSig); err != nil {
			b.log.Warn("strategy list: reading the signature failed", "file", b.paths.DPIStrategiesSig, "err", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		b.log.Warn("strategy list: reading the downloaded list failed", "file", b.paths.DPIStrategies, "err", err)
	}
	l, err := strategies.Select(builtinStrategies.BuiltinJSON, remote, sig, b.pub, zapret2.ValidateArgs)
	if err != nil {
		b.log.Info("strategy list: downloaded list not used; using the built-in one", "code", app.CodeStrategyListInvalid, "err", err)
	}
	b.mu.Lock()
	b.cur = l
	b.mu.Unlock()
}

func (b *strategyBox) get() strategies.List {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cur
}

// NewDPIManager wires the engines this OS has, extracted under
// paths.BinDir. list may be nil (the built-in strategies), e.g. for the
// headless --restore path that only stops the engine.
func NewDPIManager(paths store.Paths, p platform.Deps, list func() strategies.List) *dpi.Manager {
	if list == nil {
		l, err := strategies.Parse(builtinStrategies.BuiltinJSON, zapret2.ValidateArgs)
		if err != nil {
			slog.Error("strategy list: parsing the built-in list failed", "err", err)
		}
		list = func() strategies.List { return l }
	}
	return dpi.NewManager(filepath.Clean(paths.BinDir), p.DPIEngines(list), p.DPIRunner, p.DPIInterceptor, time.Sleep)
}
