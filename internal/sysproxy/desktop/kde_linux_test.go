package desktop

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeKwrite applies kwriteconfig6 calls to an in-memory [Proxy Settings].
type fakeKwrite struct {
	keys    map[string]string
	log     []string
	reparse int
}

func (f *fakeKwrite) run(name string, args ...string) ([]byte, error) {
	f.log = append(f.log, name+" "+strings.Join(args, " "))
	key := args[slices.Index(args, "--key")+1]
	if args[len(args)-1] == "--delete" {
		delete(f.keys, key)
	} else {
		f.keys[key] = args[len(args)-1]
	}
	return nil, nil
}

func (f *fakeKwrite) read() ([]byte, error) {
	if f.keys == nil {
		return nil, os.ErrNotExist
	}
	var b strings.Builder
	b.WriteString("[General]\nfoo=bar\n\n[Proxy Settings]\n")
	for _, k := range kdeKeys {
		if v, ok := f.keys[k]; ok {
			b.WriteString(k + "=" + v + "\n")
		}
	}
	return []byte(b.String()), nil
}

func fakeKDE(f *fakeKwrite) kde {
	return kde{run: f.run, read: f.read, tool: "kwriteconfig6", reparse: func() error { f.reparse++; return nil }}
}

func TestKDE_ApplyWritesKeysAndSignals(t *testing.T) {
	f := &fakeKwrite{keys: map[string]string{}}
	k := fakeKDE(f)
	require.NoError(t, k.apply("127.0.0.1:8080"))
	require.Equal(t, "1", f.keys["ProxyType"])
	require.Equal(t, "http://127.0.0.1 8080", f.keys["httpProxy"])
	require.Equal(t, "http://127.0.0.1 8080", f.keys["httpsProxy"])
	require.Equal(t, "localhost,127.0.0.0/8,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16", f.keys["NoProxyFor"])
	require.Equal(t, "false", f.keys["ReversedException"])
	require.Equal(t, "kwriteconfig6 --file kioslaverc --group Proxy Settings --key ProxyType 1", f.log[len(f.log)-1], "the type flips last")
	require.Equal(t, 1, f.reparse)
	ours, err := k.isOurs("127.0.0.1:8080")
	require.NoError(t, err)
	require.True(t, ours)
}

func TestKDE_RoundTrip(t *testing.T) {
	f := &fakeKwrite{keys: map[string]string{"ProxyType": "2", "Proxy Config Script": "http://wpad/x.pac"}}
	k := fakeKDE(f)
	s, err := k.snapshot("127.0.0.1:8080")
	require.NoError(t, err)
	require.Equal(t, "kde", s.Backend)
	require.ElementsMatch(t, []string{"ProxyType", "Proxy Config Script"}, s.KDE.Present)
	require.NoError(t, k.apply("127.0.0.1:8080"))
	require.NoError(t, k.restore(s))
	require.Equal(t, map[string]string{"ProxyType": "2", "Proxy Config Script": "http://wpad/x.pac"}, f.keys)
	require.Equal(t, 2, f.reparse)
}

// Review Focus 5: a user who never opened KDE's proxy settings has no
// kioslaverc; restore deletes every key Ghostline wrote.
func TestKDE_RestoreAbsentDeletesKeys(t *testing.T) {
	f := &fakeKwrite{}
	k := fakeKDE(f)
	s, err := k.snapshot("127.0.0.1:8080")
	require.NoError(t, err)
	require.Empty(t, s.KDE.Present)
	f.keys = map[string]string{}
	require.NoError(t, k.apply("127.0.0.1:8080"))
	require.NoError(t, k.restore(s))
	require.Empty(t, f.keys)
}

func TestKDE_ReadsTheRealFile(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".config", "kioslaverc"), []byte("[Proxy Settings]\nProxyType=1\nhttpProxy=http://127.0.0.1 8080\n[Other]\nProxyType=9\n"), 0o644))
	vals, present, err := readKIO(fileReader(filepath.Join(home, ".config", "kioslaverc")))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"ProxyType": "1", "httpProxy": "http://127.0.0.1 8080"}, vals)
	require.ElementsMatch(t, []string{"ProxyType", "httpProxy"}, present)
}

func TestKDE_WatchSeesWrite(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hit := make(chan struct{}, 4)
	go func() { _ = watchKIO(ctx, dir, 50*time.Millisecond, func() { hit <- struct{}{} }) }()
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.rc"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kioslaverc"), []byte("[Proxy Settings]\n"), 0o644))
	select {
	case <-hit:
	case <-time.After(2 * time.Second):
		t.Fatal("kioslaverc change not seen")
	}
}
