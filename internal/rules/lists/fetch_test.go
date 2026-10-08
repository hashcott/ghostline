package lists_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/stretchr/testify/require"
)

func TestNormalizeURL(t *testing.T) {
	cases := []struct{ in, primary, fallback string }{
		{"github.com/u/r/blob/main/a/b.txt", "https://raw.githubusercontent.com/u/r/main/a/b.txt", "https://cdn.jsdelivr.net/gh/u/r@main/a/b.txt"},
		{"https://github.com/u/r/raw/main/x", "https://raw.githubusercontent.com/u/r/main/x", "https://cdn.jsdelivr.net/gh/u/r@main/x"},
		{"https://raw.githubusercontent.com/u/r/master/hosts", "https://raw.githubusercontent.com/u/r/master/hosts", "https://cdn.jsdelivr.net/gh/u/r@master/hosts"},
		{"https://raw.githubusercontent.com/u/r/refs/heads/main/x.txt", "https://raw.githubusercontent.com/u/r/main/x.txt", "https://cdn.jsdelivr.net/gh/u/r@main/x.txt"},
		{"https://cdn.jsdelivr.net/gh/u/r@v1/x", "https://raw.githubusercontent.com/u/r/v1/x", "https://cdn.jsdelivr.net/gh/u/r@v1/x"},
		{"https://gist.github.com/u/abc", "https://gist.github.com/u/abc/raw", ""},
		{"https://gist.github.com/u/abc/raw/f.txt", "https://gist.github.com/u/abc/raw/f.txt", ""},
		{"https://example.com/x.txt", "https://example.com/x.txt", ""},
	}
	for _, c := range cases {
		p, f, err := lists.NormalizeURL(c.in)
		require.NoError(t, err, c.in)
		require.Equal(t, c.primary, p, c.in)
		require.Equal(t, c.fallback, f, c.in)
	}
	for _, bad := range []string{"http://example.com/x", "ftp://x/y", "", "https://"} {
		_, _, err := lists.NormalizeURL(bad)
		require.Error(t, err, bad)
	}
}

// routed returns a client whose every host:443 dial goes to srv.
func routed(srv *httptest.Server, fail map[string]bool) *http.Client {
	addr := srv.Listener.Addr().String()
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, a string) (net.Conn, error) {
			h, _, _ := net.SplitHostPort(a)
			if fail[h] {
				return nil, errors.New("dial refused")
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
}

func atomicWrite(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func newFetcher(t *testing.T, c *http.Client) *lists.Fetcher {
	return &lists.Fetcher{Client: c, Dir: t.TempDir(), Now: time.Now, WriteFile: atomicWrite}
}

func TestFetch_ETag304(t *testing.T) {
	var gotINM []string
	var mu sync.Mutex
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotINM = append(gotINM, r.Header.Get("If-None-Match"))
		mu.Unlock()
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("ads.example\ntracker.example\n"))
	}))
	defer srv.Close()
	f := newFetcher(t, routed(srv, nil))
	l := lists.List{ID: "l1", Source: "url", URL: "https://example.com/list.txt", Format: "auto", Action: "block", Enabled: true}
	r, err := f.Fetch(context.Background(), &l)
	require.NoError(t, err)
	require.Len(t, r.Entries, 2)
	require.Equal(t, `"v1"`, l.ETag)
	require.Equal(t, "domains", l.Detected)
	require.Equal(t, 2, l.Counts["domain"])
	first := l.LastUpdated

	time.Sleep(5 * time.Millisecond)
	r, err = f.Fetch(context.Background(), &l)
	require.NoError(t, err)
	require.Len(t, r.Entries, 2)
	require.True(t, l.LastUpdated.After(first))
	require.Equal(t, []string{"", `"v1"`}, gotINM)
}

func gz(t *testing.T, b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(b)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestFetch_GzipAndZipBomb(t *testing.T) {
	small := gz(t, []byte("ads.example\n"))
	bomb := gz(t, bytes.Repeat([]byte("a"), lists.MaxFileBytes+10))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "bomb") {
			_, _ = w.Write(bomb)
			return
		}
		_, _ = w.Write(small)
	}))
	defer srv.Close()
	f := newFetcher(t, routed(srv, nil))
	l := lists.List{ID: "g", Source: "url", URL: "https://example.com/list.txt.gz", Format: "auto", Action: "block"}
	r, err := f.Fetch(context.Background(), &l)
	require.NoError(t, err)
	require.Len(t, r.Entries, 1)

	l.URL = "https://example.com/bomb.txt.gz"
	_, err = f.Fetch(context.Background(), &l)
	require.ErrorIs(t, err, lists.ErrTooLarge)
	require.NotEmpty(t, l.LastError)
	r, err = f.LoadCached(l)
	require.NoError(t, err)
	require.Len(t, r.Entries, 1)
}

func TestFetch_FallbackJsDelivr(t *testing.T) {
	var hosts []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts = append(hosts, r.Host)
		_, _ = w.Write([]byte("0.0.0.0 ads.example\n"))
	}))
	defer srv.Close()
	f := newFetcher(t, routed(srv, map[string]bool{"raw.githubusercontent.com": true}))
	l := lists.List{ID: "gh", Source: "url", URL: "https://github.com/u/r/blob/main/hosts", Format: "auto", Action: "block"}
	r, err := f.Fetch(context.Background(), &l)
	require.NoError(t, err)
	require.Len(t, r.Entries, 1)
	require.Equal(t, []string{"cdn.jsdelivr.net"}, hosts)
	require.Equal(t, "hosts", l.Detected)
}

func TestFetch_ErrorKeepsCache(t *testing.T) {
	fail := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ads.example\n"))
	}))
	defer srv.Close()
	f := newFetcher(t, routed(srv, nil))
	l := lists.List{ID: "e", Source: "url", URL: "https://example.com/l.txt", Format: "auto", Action: "block"}
	_, err := f.Fetch(context.Background(), &l)
	require.NoError(t, err)
	fail = true
	_, err = f.Fetch(context.Background(), &l)
	require.Error(t, err)
	require.Contains(t, l.LastError, "500")
	r, err := f.LoadCached(l)
	require.NoError(t, err)
	require.Len(t, r.Entries, 1)
}

func TestFetch_UnsupportedFormat(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("\x0a\x00\x01binary"))
	}))
	defer srv.Close()
	f := newFetcher(t, routed(srv, nil))
	l := lists.List{ID: "b", Source: "url", URL: "https://example.com/geosite.dat", Format: "auto", Action: "block"}
	_, err := f.Fetch(context.Background(), &l)
	require.ErrorIs(t, err, lists.ErrUnsupported)
}

func TestFetch_V2flyInclude(t *testing.T) {
	files := map[string]string{
		"/u/r/main/data/a": "include:b\na.example\n",
		"/u/r/main/data/b": "include:a\ninclude:c\nb.example\n",
		"/u/r/main/data/c": "include:d\nc.example\n",
		"/u/r/main/data/d": "include:e\nd.example\n",
		"/u/r/main/data/e": "include:f\ne.example\n",
		"/u/r/main/data/f": "include:g\nf.example\n",
		"/u/r/main/data/g": "g.example\n",
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	f := newFetcher(t, routed(srv, nil))
	l := lists.List{ID: "v", Source: "url", URL: "https://raw.githubusercontent.com/u/r/main/data/a", Format: "v2fly", Action: "block"}
	r, err := f.Fetch(context.Background(), &l)
	require.NoError(t, err)
	var got []string
	for _, e := range r.Entries {
		got = append(got, e.Pattern.Value)
	}
	// a (depth 0) → b (1) → c (2) → d (3) → e (4) → f (5); g would be depth 6.
	require.ElementsMatch(t, []string{"a.example", "b.example", "c.example", "d.example", "e.example", "f.example"}, got)
	// Cached includes resolve without the network.
	srv.Close()
	r, err = f.LoadCached(l)
	require.NoError(t, err)
	require.Len(t, r.Entries, 6)
}

func TestFetch_IncludeRejectsOtherDirs(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("include:../secret\nok.example\n"))
	}))
	defer srv.Close()
	f := newFetcher(t, routed(srv, nil))
	l := lists.List{ID: "x", Source: "url", URL: "https://raw.githubusercontent.com/u/r/main/data/a", Format: "v2fly", Action: "block"}
	r, err := f.Fetch(context.Background(), &l)
	require.NoError(t, err)
	require.Len(t, r.Entries, 1)
	require.Positive(t, r.Skipped)
}

func TestFetch_LocalFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "my.txt")
	require.NoError(t, os.WriteFile(p, []byte("0.0.0.0 ads.example\n1.2.3.4 site.example\n"), 0o644))
	f := newFetcher(t, nil)
	l := lists.List{ID: "f", Source: "file", Path: p, Format: "auto", Action: "fromFile"}
	r, err := f.Fetch(context.Background(), &l)
	require.NoError(t, err)
	ls, err := lists.ToListSet(l, r)
	require.NoError(t, err)
	require.True(t, ls.FromFile)
	c, err := rules.Compile(nil, []rules.ListSet{ls})
	require.NoError(t, err)
	require.True(t, c.Explain("ads.example").Block)
	require.Len(t, c.Explain("site.example").IPs, 1)
}

func TestToListSet_Actions(t *testing.T) {
	for action, check := range map[string]func(rules.ListSet) bool{
		"block":         func(s rules.ListSet) bool { return s.Action.Block },
		"allow":         func(s rules.ListSet) bool { return s.Action.Allow },
		"fragment=on":   func(s rules.ListSet) bool { return s.Action.Fragment == rules.FragOn },
		"upstream=corp": func(s rules.ListSet) bool { return s.Action.Upstream == "corp" },
	} {
		s, err := lists.ToListSet(lists.List{ID: "a", Action: action}, lists.Result{})
		require.NoError(t, err)
		require.True(t, check(s), action)
	}
	_, err := lists.ToListSet(lists.List{ID: "a", Action: "explode"}, lists.Result{})
	require.Error(t, err)
}

func TestScheduler_DueAndJitter(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	ls := []lists.List{
		{ID: "old", Enabled: true, UpdateHours: 24, LastUpdated: now.Add(-25 * time.Hour)},
		{ID: "fresh", Enabled: true, UpdateHours: 24, LastUpdated: now.Add(-time.Hour)},
		{ID: "manual", Enabled: true, UpdateHours: 0, LastUpdated: now.Add(-1000 * time.Hour)},
		{ID: "off", Enabled: false, UpdateHours: 24, LastUpdated: now.Add(-1000 * time.Hour)},
		{ID: "old2", Enabled: true, UpdateHours: 1, LastUpdated: now.Add(-2 * time.Hour)},
	}
	ctx, cancel := context.WithCancel(context.Background())
	var waits []time.Duration
	var ran []string
	s := &lists.Scheduler{
		Lists: func() []lists.List { return ls },
		Due:   lists.Due,
		Run: func(ctx context.Context, id string) error {
			ran = append(ran, id)
			return nil
		},
		Now:    func() time.Time { return now },
		Jitter: func() time.Duration { return 7 * time.Minute },
		After: func(d time.Duration) <-chan time.Time {
			waits = append(waits, d)
			if len(waits) > 3 {
				cancel()
			}
			c := make(chan time.Time, 1)
			c <- now
			return c
		},
	}
	s.Loop(ctx)
	require.Equal(t, []time.Duration{30 * time.Second, 7 * time.Minute, 7 * time.Minute, 15 * time.Minute}, waits[:4])
	require.Equal(t, []string{"old", "old2"}, ran)
}

func TestCatalog_Valid(t *testing.T) {
	items := lists.Catalog()
	require.Len(t, items, 31)
	seen := map[string]bool{}
	perCat := map[string]int{}
	for _, it := range items {
		require.False(t, seen[it.ID], "duplicate id %s", it.ID)
		seen[it.ID] = true
		require.Contains(t, lists.Categories, it.Category, it.ID)
		perCat[it.Category]++
		switch it.Category {
		case "bypass":
			require.Equal(t, "fragment=on", it.Action, it.ID)
		case "fakesni":
			require.Equal(t, "perLine", it.Action, it.ID)
		default:
			require.Equal(t, "block", it.Action, it.ID)
		}
		require.NotEmpty(t, it.ID)
		require.NotEmpty(t, it.License, it.ID)
		require.NotEmpty(t, it.Repo, it.ID)
		require.True(t, strings.HasPrefix(it.URL, "https://"), it.ID)
		_, _, err := lists.NormalizeURL(it.URL)
		require.NoError(t, err, it.ID)
		res := lists.Result{}
		if it.Format == "ghostline" {
			res.Format = "ghostline"
		}
		_, err = lists.ToListSet(lists.List{ID: it.ID, Action: it.Action}, res)
		require.NoError(t, err, it.ID)
	}
	for _, c := range lists.Categories {
		require.Positive(t, perCat[c], "category %s is empty", c)
	}
}

func TestFetch_RejectsUnsafeID(t *testing.T) {
	f := newFetcher(t, http.DefaultClient)
	for _, id := range []string{`..\..\evil`, "../x", "a/b", "", "a b"} {
		l := lists.List{ID: id, Source: "file", Path: filepath.Join(t.TempDir(), "x.txt"), Format: "hosts", Action: "block"}
		_, err := f.Fetch(context.Background(), &l)
		require.Error(t, err, id)
		_, err = f.LoadCached(l)
		require.Error(t, err, id)
	}
	require.True(t, lists.ValidID("hagezi-pro-plus"))
	require.True(t, lists.ValidID("my_list-01ab9f"))
}

// C2: the Linux daemon runs as root for other users; it must never read a
// path a client named (e.g. /etc/shadow as a "hosts" list).
func TestFetch_NoFilesRefusesFileLists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(p, []byte("root:x:0:0\n"), 0o600))
	f := newFetcher(t, nil)
	f.NoFiles = true
	l := lists.List{ID: "f", Source: "file", Path: p, Format: "hosts", Action: "block"}
	_, err := f.Fetch(context.Background(), &l)
	require.ErrorIs(t, err, lists.ErrFileListsOff)
}
