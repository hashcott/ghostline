package dpi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrHashMismatch  = errors.New("dpi: DPI engine files do not match the pinned hashes")
	ErrStartFailed   = errors.New("dpi: DPI engine failed to start")
	ErrBlockedByAV   = errors.New("dpi: DPI engine was blocked (antivirus?)")
	ErrUnknownEngine = errors.New("dpi: unknown engine")
	// ErrKernelUnsupported: the Linux kernel cannot queue packets to the
	// engine (nfnetlink_queue / nft_queue missing).
	ErrKernelUnsupported = errors.New("dpi: the kernel cannot queue packets (nfnetlink_queue/nft_queue)")
)

// List files are copied into the engine directory under these names: the
// engines read argv as ANSI (GoodbyeDPI) or through Cygwin (winws2), so a
// path with Vietnamese letters (C:\Users\Đức…) would be mangled.
// autoHostlistName is per OS (lists_<os>.go).
const blacklistName = "blacklist.txt"

func extractWith(src fs.FS, dir string, pins map[string]string) error {
	for name, want := range pins {
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if fileHash(dst) == want {
			continue
		}
		b, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		if hashOf(b) != want {
			return fmt.Errorf("%w: embedded %s", ErrHashMismatch, name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	return verifyWith(dir, pins)
}

func verifyWith(dir string, pins map[string]string) error {
	for name, want := range pins {
		if fileHash(filepath.Join(dir, filepath.FromSlash(name))) != want {
			return fmt.Errorf("%w: %s", ErrHashMismatch, name)
		}
	}
	return nil
}

func hashOf(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func fileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return hashOf(b)
}

// Process is a started engine process.
type Process interface {
	PID() int
	Exited() bool
	Kill() error
}

// Runner starts processes.
type Runner interface {
	Start(exe string, args []string, dir string) (Process, error)
}

// Installed pairs an engine with the embedded files it is extracted from.
type Installed struct {
	Engine Engine
	Assets fs.FS
}

// Manager runs at most one engine process. Each engine lives in its own
// directory under binDir.
type Manager struct {
	binDir  string
	engines map[string]Installed
	runner  Runner
	ic      Interceptor
	sleep   func(time.Duration)

	mu      sync.Mutex
	proc    Process
	running string // engine ID of proc
	plan    Plan   // plan proc was started with (absolute paths)
}

// NewManager manages the given engines, extracted under binDir.
func NewManager(binDir string, engines []Installed, r Runner, ic Interceptor, sleep func(time.Duration)) *Manager {
	if sleep == nil {
		sleep = time.Sleep
	}
	m := &Manager{binDir: binDir, engines: map[string]Installed{}, runner: r, ic: ic, sleep: sleep}
	for _, e := range engines {
		m.engines[e.Engine.ID()] = e
	}
	return m
}

// Get returns an engine by ID.
func (m *Manager) Get(engine string) (Engine, bool) {
	in, ok := m.engines[engine]
	return in.Engine, ok
}

func (m *Manager) dir(engine string) string { return filepath.Join(m.binDir, engine) }

// Start stops whatever runs (cleaning up its capture), verifies
// (re-extracting if needed), prepares the capture and launches the engine.
// It is running when, after 2s, the process is alive and the Interceptor
// says it captures. p's list paths are absolute.
func (m *Manager) Start(ctx context.Context, engine string, p Plan) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	in, ok := m.engines[engine]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownEngine, engine)
	}
	if err := m.stopLocked(); err != nil {
		return 0, fmt.Errorf("%w: cleanup: %v", ErrStartFailed, err)
	}
	dir, pins := m.dir(engine), in.Engine.Files()
	if err := verifyWith(dir, pins); err != nil {
		if err := extractWith(in.Assets, dir, pins); err != nil {
			// Real-time antivirus refuses the write itself.
			if errors.Is(err, os.ErrPermission) || isAppControlBlock(err) {
				return 0, fmt.Errorf("%w: %v", ErrBlockedByAV, err)
			}
			return 0, err
		}
	}
	rel, err := copyLists(dir, p)
	if err != nil {
		return 0, fmt.Errorf("%w: lists: %v", ErrStartFailed, err)
	}
	if err := m.ic.Prepare(dir); err != nil {
		return 0, err
	}
	args, err := in.Engine.Args(rel)
	if err != nil {
		return 0, err
	}
	proc, err := m.runner.Start(filepath.Join(dir, filepath.FromSlash(in.Engine.Exe())), args, dir)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, fs.ErrNotExist) || isAppControlBlock(err) {
			return 0, fmt.Errorf("%w: %v", ErrBlockedByAV, err)
		}
		return 0, fmt.Errorf("%w: %v", ErrStartFailed, err)
	}
	m.sleep(2 * time.Second)
	if proc.Exited() {
		if verifyWith(dir, pins) != nil {
			return 0, ErrBlockedByAV // files vanished or changed: quarantined
		}
		return 0, ErrStartFailed
	}
	if !m.ic.Ready(proc.PID()) {
		_ = proc.Kill()
		return 0, fmt.Errorf("%w: %s not capturing", ErrStartFailed, m.ic.Info().Mechanism)
	}
	m.proc, m.running, m.plan = proc, engine, p
	return proc.PID(), nil
}

// copyLists copies p's list files into dir and returns p with relative names.
func copyLists(dir string, p Plan) (Plan, error) {
	if p.Scope != ScopeBlacklist {
		p.Blacklist = ""
	}
	if p.Blacklist != "" {
		b, err := os.ReadFile(p.Blacklist)
		if err != nil {
			return p, err
		}
		if err := os.WriteFile(filepath.Join(dir, blacklistName), b, 0o644); err != nil {
			return p, err
		}
		p.Blacklist = blacklistName
	}
	if p.AutoHostlist != "" {
		b, err := os.ReadFile(p.AutoHostlist)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return p, err
		}
		dst := filepath.Join(dir, filepath.FromSlash(autoHostlistName))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return p, err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return p, err
		}
		p.AutoHostlist = autoHostlistName
	}
	return p, nil
}

// RefreshLists copies the blacklist into the running engine's directory, for
// engines that re-read it by themselves.
func (m *Manager) RefreshLists(p Plan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running == "" || p.Blacklist == "" {
		return nil
	}
	b, err := os.ReadFile(p.Blacklist)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir(m.running), blacklistName), b, 0o644)
}

func isAppControlBlock(err error) bool {
	return bytes.Contains([]byte(err.Error()), []byte("Application Control")) ||
		bytes.Contains([]byte(err.Error()), []byte("virus"))
}

// Stop kills the engine and cleans up its capture.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *Manager) stopLocked() error {
	var errs []error
	if m.proc != nil {
		if m.plan.AutoHostlist != "" {
			// Keep what the engine learned; it only lives in its directory.
			if b, err := os.ReadFile(filepath.Join(m.dir(m.running), filepath.FromSlash(autoHostlistName))); err == nil {
				errs = append(errs, os.WriteFile(m.plan.AutoHostlist, b, 0o644))
			}
		}
		if !m.proc.Exited() {
			errs = append(errs, m.proc.Kill())
		}
		m.proc, m.running, m.plan = nil, "", Plan{}
	}
	errs = append(errs, m.ic.Cleanup())
	return errors.Join(errs...)
}

// Running reports whether the managed process is alive.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proc != nil && !m.proc.Exited()
}

// Engine returns the ID of the running engine, "" when none runs.
func (m *Manager) Engine() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc == nil || m.proc.Exited() {
		return ""
	}
	return m.running
}

// Info describes the packet capture for the UI.
func (m *Manager) Info() InterceptorInfo { return m.ic.Info() }
