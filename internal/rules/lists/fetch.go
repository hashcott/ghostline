package lists

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"github.com/hashcott/ghostline/internal/rules/formats"
)

// MaxFileBytes caps one list after decompression.
const MaxFileBytes = 50 << 20

// MaxIncludeDepth caps v2fly include: nesting.
const MaxIncludeDepth = 5

// ErrTooLarge means a list exceeds MaxFileBytes (or the total entry cap).
var ErrTooLarge = errors.New("rules: list too large")

var errNotModified = errors.New("not modified")

// ErrFileListsOff means lists from local files are disabled: Ghostline
// runs as root for other users (the Linux daemon) and never reads a path a
// client named.
var ErrFileListsOff = errors.New("lists: lists from a local file are not available here")

// Fetcher downloads lists and keeps their cache in Dir.
type Fetcher struct {
	Client    *http.Client
	Dir       string // <data>\lists
	Now       func() time.Time
	WriteFile func(path string, data []byte) error // atomic write, injected
	// SigKey verifies lists marked Signed (the servers.json key).
	SigKey ed25519.PublicKey
	// Fallback returns a built-in copy (and its signature) of a signed
	// list by URL, used before the first successful download.
	Fallback func(url string) (data, sig []byte, ok bool)
	// NoFiles refuses lists whose Source is "file" (ErrFileListsOff).
	NoFiles bool
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidID reports whether id is safe as a cache file name: letters, digits,
// '-' and '_' only, so it can never leave the cache directory.
func ValidID(id string) bool { return safeID.MatchString(id) }

var errBadID = errors.New("lists: invalid list id")

func (f *Fetcher) cachePath(id, include string) string {
	if include == "" {
		return filepath.Join(f.Dir, id+".txt")
	}
	return filepath.Join(f.Dir, id+"@"+include+".txt")
}

// Fetch downloads (or reads) l, parses it, refreshes the cache and l's
// metadata. On error the cache is left as it was and l.LastError is set.
func (f *Fetcher) Fetch(ctx context.Context, l *List) (Result, error) {
	if !ValidID(l.ID) {
		return Result{}, errBadID
	}
	r, meta, err := f.fetch(ctx, l)
	if err != nil {
		l.LastError = err.Error()
		return Result{}, err
	}
	l.LastUpdated = f.Now()
	if meta.etag != "" || meta.lastModified != "" {
		l.ETag, l.LastModified = meta.etag, meta.lastModified
	}
	l.Detected = string(r.Format)
	l.Counts, l.Skipped, l.SkippedSamples, l.LastError = r.Counts, r.Skipped, r.Samples, ""
	l.SignatureOK = l.Signed
	return r, nil
}

type httpMeta struct{ etag, lastModified string }

func (f *Fetcher) fetch(ctx context.Context, l *List) (Result, httpMeta, error) {
	if l.Source == "file" {
		if f.NoFiles {
			return Result{}, httpMeta{}, ErrFileListsOff
		}
		data, err := readLimited(l.Path)
		if err != nil {
			return Result{}, httpMeta{}, err
		}
		r, err := f.parse(l, l.Path, data, func(name string) ([]byte, error) {
			return readLimited(filepath.Join(filepath.Dir(l.Path), name))
		}, true)
		if err != nil {
			return Result{}, httpMeta{}, err
		}
		return r, httpMeta{}, f.WriteFile(f.cachePath(l.ID, ""), data)
	}
	primary, fallback, err := NormalizeURL(l.URL)
	if err != nil {
		return Result{}, httpMeta{}, err
	}
	conditional := false
	if _, err := os.Stat(f.cachePath(l.ID, "")); err == nil {
		conditional = true
	}
	data, meta, used, err := f.getWithFallback(ctx, primary, fallback, l, conditional)
	if errors.Is(err, errNotModified) {
		r, err := f.LoadCached(*l)
		return r, httpMeta{}, err
	}
	if err != nil {
		return Result{}, httpMeta{}, err
	}
	var sig []byte
	if l.Signed {
		// Verify before parsing or caching: an unsigned or tampered copy
		// never replaces the last verified one.
		if sig, err = f.fetchSig(ctx, used); err != nil {
			return Result{}, httpMeta{}, err
		}
		if err := f.verify(data, sig); err != nil {
			return Result{}, httpMeta{}, err
		}
	}
	dir := used[:len(used)-len(path.Base(used))]
	r, err := f.parse(l, used, data, func(name string) ([]byte, error) {
		b, _, err := f.get(ctx, dir+name, "", "")
		return b, err
	}, true)
	if err != nil {
		return Result{}, httpMeta{}, err
	}
	if err := f.WriteFile(f.cachePath(l.ID, ""), data); err != nil {
		return Result{}, httpMeta{}, err
	}
	if l.Signed {
		return r, meta, f.WriteFile(f.sigPath(l.ID), sig)
	}
	return r, meta, nil
}

func (f *Fetcher) getWithFallback(ctx context.Context, primary, fallback string, l *List, conditional bool) ([]byte, httpMeta, string, error) {
	etag, lm := "", ""
	if conditional {
		etag, lm = l.ETag, l.LastModified
	}
	data, meta, err := f.get(ctx, primary, etag, lm)
	var he *httpError
	if err == nil || errors.Is(err, errNotModified) || errors.As(err, &he) || errors.Is(err, ErrTooLarge) || fallback == "" {
		return data, meta, primary, err
	}
	data, meta, err = f.get(ctx, fallback, "", "")
	return data, meta, fallback, err
}

type httpError struct{ code int }

func (e *httpError) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

func (f *Fetcher) get(ctx context.Context, u, etag, lastModified string) ([]byte, httpMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, httpMeta{}, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	c := f.Client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, httpMeta{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified:
		return nil, httpMeta{}, errNotModified
	case resp.StatusCode != http.StatusOK:
		return nil, httpMeta{}, &httpError{resp.StatusCode}
	}
	data, err := readCapped(resp.Body)
	if err != nil {
		return nil, httpMeta{}, err
	}
	return data, httpMeta{etag: resp.Header.Get("ETag"), lastModified: resp.Header.Get("Last-Modified")}, nil
}

func readLimited(p string) ([]byte, error) {
	fh, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return readCapped(fh)
}

// readCapped reads at most MaxFileBytes, transparently un-gzipping.
func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileBytes {
		return nil, ErrTooLarge
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		data, err = io.ReadAll(io.LimitReader(zr, MaxFileBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > MaxFileBytes {
			return nil, ErrTooLarge
		}
	}
	return data, nil
}

// parse detects (unless l.Format is fixed) and parses data, then resolves
// v2fly includes with load. When cache is true, includes are cached.
func (f *Fetcher) parse(l *List, name string, data []byte, load func(string) ([]byte, error), cache bool) (Result, error) {
	fm := formats.Format(l.Format)
	if l.Format == "" || l.Format == "auto" {
		var err error
		if fm, err = formats.Detect(name, data); err != nil {
			return Result{}, err
		}
	}
	r, err := formats.Parse(fm, data)
	if err != nil {
		return Result{}, err
	}
	if fm == formats.V2fly {
		seen := map[string]bool{path.Base(name): true}
		f.includes(l, &r, r.Includes, 1, seen, load, cache)
	}
	return r, nil
}

func (f *Fetcher) includes(l *List, r *Result, names []string, depth int, seen map[string]bool, load func(string) ([]byte, error), cache bool) {
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		if !safeName.MatchString(name) || name == "." || name == ".." || depth > MaxIncludeDepth {
			r.Skipped++
			continue
		}
		data, err := load(name)
		if err != nil {
			r.Skipped++
			continue
		}
		if cache {
			_ = f.WriteFile(f.cachePath(l.ID, name), data)
		}
		sub, err := formats.Parse(formats.V2fly, data)
		if err != nil {
			r.Skipped++
			continue
		}
		r.Entries = append(r.Entries, sub.Entries...)
		for k, v := range sub.Counts {
			r.Counts[k] += v
		}
		r.Skipped += sub.Skipped
		f.includes(l, r, sub.Includes, depth+1, seen, load, cache)
	}
}

// LoadCached parses the cached copy of l (and its cached includes).
func (f *Fetcher) LoadCached(l List) (Result, error) {
	if !ValidID(l.ID) {
		return Result{}, errBadID
	}
	data, err := os.ReadFile(f.cachePath(l.ID, ""))
	switch {
	case err == nil && l.Signed:
		if err := f.verifyCached(l.ID, data); err != nil {
			return Result{}, err
		}
	case err != nil && l.Signed && f.Fallback != nil:
		// Never downloaded yet: the built-in copy, verified like a download.
		fb, sig, ok := f.Fallback(l.URL)
		if !ok {
			return Result{}, err
		}
		if verr := f.verify(fb, sig); verr != nil {
			return Result{}, verr
		}
		data = fb
	case err != nil:
		return Result{}, err
	}
	if l.Format == "" || l.Format == "auto" {
		l.Format = l.Detected
	}
	name := l.URL
	if l.Source == "file" {
		name = l.Path
	}
	return f.parse(&l, name, data, func(inc string) ([]byte, error) {
		return os.ReadFile(f.cachePath(l.ID, inc))
	}, false)
}
