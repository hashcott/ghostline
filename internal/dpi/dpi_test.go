package dpi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func TestExtractVerify(t *testing.T) {
	dir := t.TempDir()
	pins := map[string]string{"a.bin": sha("hello")}
	src := fstest.MapFS{"a.bin": {Data: []byte("hello")}}
	require.NoError(t, extractWith(src, dir, pins))
	require.NoError(t, verifyWith(dir, pins))
	fi1, _ := os.Stat(filepath.Join(dir, "a.bin"))

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, extractWith(src, dir, pins))
	fi2, _ := os.Stat(filepath.Join(dir, "a.bin"))
	require.Equal(t, fi1.ModTime(), fi2.ModTime(), "correct files are not rewritten")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.bin"), []byte("evil"), 0o644))
	require.ErrorIs(t, verifyWith(dir, pins), ErrHashMismatch)
	require.NoError(t, extractWith(src, dir, pins))
	require.NoError(t, verifyWith(dir, pins))

	require.ErrorIs(t, extractWith(fstest.MapFS{"a.bin": {Data: []byte("tampered")}}, t.TempDir(), pins), ErrHashMismatch)
}

// fakeEngine records the plans Args received and returns a fixed argv.
type fakeEngine struct {
	id    string
	files map[string]string
	got   []Plan
}

func (e *fakeEngine) ID() string                  { return e.id }
func (e *fakeEngine) Files() map[string]string    { return e.files }
func (e *fakeEngine) Exe() string                 { return e.id + ".exe" }
func (e *fakeEngine) Strategies() []Strategy      { return nil }
func (e *fakeEngine) ValidateCustom(string) error { return nil }
func (e *fakeEngine) HotReloadsLists() bool       { return e.id == "zapret2" }
func (e *fakeEngine) Args(p Plan) ([]string, error) {
	e.got = append(e.got, p)
	out := []string{"-" + e.id}
	if p.Blacklist != "" {
		out = append(out, "--list", p.Blacklist)
	}
	return out, nil
}

type fakeProc struct {
	pid    int
	exited bool
	killed bool
}

func (p *fakeProc) PID() int     { return p.pid }
func (p *fakeProc) Exited() bool { return p.exited }
func (p *fakeProc) Kill() error  { p.killed = true; p.exited = true; return nil }

// calls is shared by the fake runner and services to check ordering.
type calls struct{ log []string }

type fakeRunner struct {
	c     *calls
	procs []*fakeProc
	exe   string
	dir   string
	args  []string
	err   error
	onRun func(dir string)
}

func (r *fakeRunner) Start(exe string, args []string, dir string) (Process, error) {
	r.exe, r.args, r.dir = exe, args, dir
	r.c.log = append(r.c.log, "run:"+filepath.Base(exe))
	if r.err != nil {
		return nil, r.err
	}
	if r.onRun != nil {
		r.onRun(dir)
	}
	p := &fakeProc{pid: 100 + len(r.procs)}
	r.procs = append(r.procs, p)
	return p, nil
}

type fakeSvc struct {
	c       *calls
	running bool
	active  bool     // a WinDivert service held by another program
	names   []string // installed WinDivert services
}

func (s *fakeSvc) Find(prefix string) ([]string, error) { return s.names, nil }
func (s *fakeSvc) Running(name string) (bool, error)    { return s.running && name == "WinDivert", nil }
func (s *fakeSvc) Active(name string) (bool, error)     { return s.active, nil }
func (s *fakeSvc) Stop(name string) error               { s.c.log = append(s.c.log, "svc.stop:"+name); return nil }
func (s *fakeSvc) Delete(name string) error {
	s.c.log = append(s.c.log, "svc.delete:"+name)
	return nil
}

type rig struct {
	m   *Manager
	r   *fakeRunner
	s   *fakeSvc
	c   *calls
	z2  *fakeEngine
	bin string
}

func newRig(t *testing.T) *rig {
	c := &calls{}
	g := &fakeEngine{id: "goodbyedpi", files: map[string]string{"goodbyedpi.exe": sha("g")}}
	z := &fakeEngine{id: "zapret2", files: map[string]string{"zapret2.exe": sha("z"), "lua/a.lua": sha("lua")}}
	rg := &rig{r: &fakeRunner{c: c}, s: &fakeSvc{c: c, running: true}, c: c, z2: z, bin: t.TempDir()}
	rg.m = NewManager(rg.bin, []Installed{
		{Engine: g, Assets: fstest.MapFS{"goodbyedpi.exe": {Data: []byte("g")}}},
		{Engine: z, Assets: fstest.MapFS{"zapret2.exe": {Data: []byte("z")}, "lua/a.lua": {Data: []byte("lua")}}},
	}, rg.r, NewWinDivert(rg.s), func(time.Duration) {})
	return rg
}

func TestManager_StartSuccess(t *testing.T) {
	rg := newRig(t)
	pid, err := rg.m.Start(context.Background(), "goodbyedpi", Plan{Strategy: "light"})
	require.NoError(t, err)
	require.Equal(t, 100, pid)
	require.Equal(t, []string{"-goodbyedpi"}, rg.r.args)
	require.Equal(t, filepath.Join(rg.bin, "goodbyedpi"), rg.r.dir)
	require.Equal(t, filepath.Join(rg.bin, "goodbyedpi", "goodbyedpi.exe"), rg.r.exe)
	require.True(t, rg.m.Running())
	require.Equal(t, "goodbyedpi", rg.m.Engine())
}

func TestManager_UnknownEngine(t *testing.T) {
	_, err := newRig(t).m.Start(context.Background(), "x", Plan{})
	require.ErrorIs(t, err, ErrUnknownEngine)
}

func TestManager_NestedFilesExtracted(t *testing.T) {
	rg := newRig(t)
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{})
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(rg.bin, "zapret2", "lua", "a.lua"))
	require.NoError(t, err)
	require.Equal(t, "lua", string(b))
}

func TestManager_StartRemovesStaleDriverServices(t *testing.T) { // Review Focus #2
	rg := newRig(t)
	rg.s.names = []string{"WinDivert1.4", "WinDivert"}
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{})
	require.NoError(t, err)
	require.Equal(t, []string{"svc.stop:WinDivert1.4", "svc.delete:WinDivert1.4", "svc.stop:WinDivert", "svc.delete:WinDivert", "run:zapret2.exe"}, rg.c.log)
}

func TestManager_StartRejectsForeignDriverInUse(t *testing.T) {
	rg := newRig(t)
	// A standalone GoodbyeDPI/zapret holds WinDivert: it survives cleanup.
	rg.s.names = []string{"WinDivert"}
	rg.s.active = true
	_, err := rg.m.Start(context.Background(), "goodbyedpi", Plan{})
	require.ErrorIs(t, err, ErrDriverInUse)
	require.False(t, rg.m.Running())
	require.NotContains(t, rg.c.log, "run:goodbyedpi.exe") // engine never launched
}

func TestManager_ListsCopiedRelative(t *testing.T) { // Review Focus #1
	rg := newRig(t)
	src := filepath.Join(t.TempDir(), "Đức Hạnh", "dpi-blacklist.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte("youtube.com\n"), 0o644))
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{Scope: ScopeBlacklist, Blacklist: src})
	require.NoError(t, err)
	require.Equal(t, "blacklist.txt", rg.z2.got[0].Blacklist)
	require.Equal(t, []string{"-zapret2", "--list", "blacklist.txt"}, rg.r.args)
	b, err := os.ReadFile(filepath.Join(rg.bin, "zapret2", "blacklist.txt"))
	require.NoError(t, err)
	require.Equal(t, "youtube.com\n", string(b))
}

func TestManager_AutoHostlistRoundTrip(t *testing.T) {
	rg := newRig(t)
	src := filepath.Join(t.TempDir(), "dpi-autohostlist.txt")
	require.NoError(t, os.WriteFile(src, []byte("a.com\n"), 0o644))
	rg.r.onRun = func(dir string) {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(autoHostlistName)))
		require.NoError(t, err)
		require.Equal(t, "a.com\n", string(b))
		require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(autoHostlistName)), []byte("a.com\nb.com\n"), 0o644))
	}
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{AutoHostlist: src})
	require.NoError(t, err)
	require.Equal(t, autoHostlistName, rg.z2.got[0].AutoHostlist)
	require.NoError(t, rg.m.Stop())
	b, err := os.ReadFile(src)
	require.NoError(t, err)
	require.Equal(t, "a.com\nb.com\n", string(b))
}

func TestManager_AutoHostlistMissingSourceStartsEmpty(t *testing.T) {
	rg := newRig(t)
	src := filepath.Join(t.TempDir(), "dpi-autohostlist.txt")
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{AutoHostlist: src})
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(rg.bin, "zapret2", filepath.FromSlash(autoHostlistName)))
	require.NoError(t, err)
	require.Empty(t, b)
}

func TestManager_SwitchEngine(t *testing.T) {
	rg := newRig(t)
	_, err := rg.m.Start(context.Background(), "goodbyedpi", Plan{})
	require.NoError(t, err)
	_, err = rg.m.Start(context.Background(), "zapret2", Plan{})
	require.NoError(t, err)
	require.True(t, rg.r.procs[0].killed)
	require.Equal(t, "zapret2", rg.m.Engine())
}

func TestManager_RefreshLists(t *testing.T) {
	rg := newRig(t)
	src := filepath.Join(t.TempDir(), "bl.txt")
	require.NoError(t, os.WriteFile(src, []byte("a.com\n"), 0o644))
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{Scope: ScopeBlacklist, Blacklist: src})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(src, []byte("b.com\n"), 0o644))
	require.NoError(t, rg.m.RefreshLists(Plan{Scope: ScopeBlacklist, Blacklist: src}))
	b, _ := os.ReadFile(filepath.Join(rg.bin, "zapret2", "blacklist.txt"))
	require.Equal(t, "b.com\n", string(b))
	require.Len(t, rg.r.procs, 1, "no restart")
}

func TestManager_ExitedImmediatelyIsStartFailed(t *testing.T) {
	rg := newRig(t)
	rg.m.sleep = func(time.Duration) { rg.r.procs[len(rg.r.procs)-1].exited = true }
	_, err := rg.m.Start(context.Background(), "goodbyedpi", Plan{})
	require.ErrorIs(t, err, ErrStartFailed)
	require.False(t, rg.m.Running())
	require.Equal(t, "", rg.m.Engine())
}

func TestManager_ExitedAndFilesGoneIsBlockedByAV(t *testing.T) {
	rg := newRig(t)
	rg.m.sleep = func(time.Duration) {
		rg.r.procs[len(rg.r.procs)-1].exited = true
		_ = os.Remove(filepath.Join(rg.bin, "goodbyedpi", "goodbyedpi.exe"))
	}
	_, err := rg.m.Start(context.Background(), "goodbyedpi", Plan{})
	require.ErrorIs(t, err, ErrBlockedByAV)
}

func TestManager_AccessDeniedIsBlockedByAV(t *testing.T) {
	rg := newRig(t)
	rg.r.err = os.ErrPermission
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{})
	require.ErrorIs(t, err, ErrBlockedByAV)
}

func TestManager_TamperedAssetIsHashMismatch(t *testing.T) {
	c := &calls{}
	e := &fakeEngine{id: "zapret2", files: map[string]string{"zapret2.exe": sha("z")}}
	m := NewManager(t.TempDir(), []Installed{{Engine: e, Assets: fstest.MapFS{"zapret2.exe": {Data: []byte("evil")}}}},
		&fakeRunner{c: c}, NewWinDivert(&fakeSvc{c: c, running: true}), func(time.Duration) {})
	_, err := m.Start(context.Background(), "zapret2", Plan{})
	require.ErrorIs(t, err, ErrHashMismatch)
}

func TestManager_DriverNotRunningIsStartFailed(t *testing.T) {
	rg := newRig(t)
	rg.s.running = false
	_, err := rg.m.Start(context.Background(), "goodbyedpi", Plan{})
	require.ErrorIs(t, err, ErrStartFailed)
	require.True(t, rg.r.procs[0].killed)
}

func TestManager_StopKillsAndRemovesWinDivert(t *testing.T) {
	rg := newRig(t)
	_, err := rg.m.Start(context.Background(), "goodbyedpi", Plan{})
	require.NoError(t, err)
	rg.s.names = []string{"WinDivert"}
	rg.c.log = nil
	require.NoError(t, rg.m.Stop())
	require.True(t, rg.r.procs[0].killed)
	require.Equal(t, []string{"svc.stop:WinDivert", "svc.delete:WinDivert"}, rg.c.log)
	require.False(t, rg.m.Running())
	require.Equal(t, "", rg.m.Engine())
}

func TestManager_StopWhenNotRunningStillCleansDriver(t *testing.T) {
	rg := newRig(t)
	rg.s.names = []string{"WinDivert1.4", "WinDivert"}
	require.NoError(t, rg.m.Stop())
	require.Equal(t, []string{"svc.stop:WinDivert1.4", "svc.delete:WinDivert1.4", "svc.stop:WinDivert", "svc.delete:WinDivert"}, rg.c.log)
	require.False(t, errors.Is(nil, ErrStartFailed))
}

func TestManager_Get(t *testing.T) {
	rg := newRig(t)
	e, ok := rg.m.Get("zapret2")
	require.True(t, ok)
	require.Equal(t, "zapret2", e.ID())
	_, ok = rg.m.Get("x")
	require.False(t, ok)
}

// Defender refuses the write itself (ERROR_VIRUS_INFECTED or access
// denied) when it scans the file being extracted.
func TestManager_ExtractRefusedIsBlockedByAV(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes a read-only file: the refusal cannot be staged (CI runs this package as root)")
	}
	rg := newRig(t)
	dst := filepath.Join(rg.bin, "zapret2", "zapret2.exe")
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
	require.NoError(t, os.WriteFile(dst, []byte("stale"), 0o444))
	t.Cleanup(func() { _ = os.Chmod(dst, 0o644) })
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{})
	require.ErrorIs(t, err, ErrBlockedByAV)
	require.False(t, errors.Is(err, ErrHashMismatch))
}

func TestIsAppControlBlock(t *testing.T) {
	require.True(t, isAppControlBlock(errors.New("open x: Operation did not complete successfully because the file contains a virus or potentially unwanted software.")))
	require.False(t, isAppControlBlock(errors.New("disk full")))
}

func TestManager_AllScopeIgnoresMissingBlacklist(t *testing.T) {
	rg := newRig(t)
	missing := filepath.Join(t.TempDir(), "dpi-blacklist.txt")
	_, err := rg.m.Start(context.Background(), "zapret2", Plan{Scope: ScopeAll, Blacklist: missing})
	require.NoError(t, err)
	require.Equal(t, "", rg.z2.got[0].Blacklist)
}
