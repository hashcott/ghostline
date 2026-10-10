// Package updater checks for new Ghostline releases and fetches signed
// server lists.
package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/servers"
	"golang.org/x/mod/semver"
)

// Release is a published version. Assets maps file names to download URLs
// (only when read from the API, not remembered in meta).
type Release struct {
	Tag        string            `json:"tag"`
	URL        string            `json:"url"`
	Prerelease bool              `json:"prerelease,omitempty"`
	Assets     map[string]string `json:"assets,omitempty"`
}

// ErrNoRelease means the list has no release for the chosen channel.
var ErrNoRelease = errors.New("updater: no release")

type apiRelease struct {
	Tag        string `json:"tag_name"`
	URL        string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (a apiRelease) release() Release {
	r := Release{Tag: a.Tag, URL: a.URL, Prerelease: a.Prerelease}
	for _, as := range a.Assets {
		if r.Assets == nil {
			r.Assets = map[string]string{}
		}
		r.Assets[as.Name] = as.URL
	}
	return r
}

// Latest reads the GitHub releases list (apiURL ends in /releases) and
// returns the highest version: stable releases only, or pre-releases too
// when beta is set. Drafts and tags that are not semver are skipped.
func Latest(ctx context.Context, c *http.Client, apiURL string, beta bool) (Release, error) {
	b, err := get(ctx, c, apiURL, 4<<20)
	if err != nil {
		return Release{}, err
	}
	var list []apiRelease
	if err := json.Unmarshal(b, &list); err != nil {
		return Release{}, err
	}
	var best *apiRelease
	for i := range list {
		a := &list[i]
		if a.Draft || !semver.IsValid(canon(a.Tag)) || (!beta && (a.Prerelease || IsPrerelease(a.Tag))) {
			continue
		}
		if best == nil || semver.Compare(canon(a.Tag), canon(best.Tag)) > 0 {
			best = a
		}
	}
	if best == nil {
		return Release{}, ErrNoRelease
	}
	return best.release(), nil
}

// ByTag reads one release from apiURL/tags/<tag>.
func ByTag(ctx context.Context, c *http.Client, apiURL, tag string) (Release, error) {
	b, err := get(ctx, c, strings.TrimSuffix(apiURL, "/")+"/tags/"+url.PathEscape(tag), 4<<20)
	if err != nil {
		return Release{}, err
	}
	var a apiRelease
	if err := json.Unmarshal(b, &a); err != nil {
		return Release{}, err
	}
	return a.release(), nil
}

// IsPrerelease reports whether tag is a semver pre-release (v1.2.0-beta.1).
func IsPrerelease(tag string) bool {
	t := canon(tag)
	return semver.IsValid(t) && semver.Prerelease(t) != ""
}

func canon(v string) string {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// Newer reports whether tag is a newer semver than current. Dev builds and
// unparsable tags never report an update.
func Newer(current, tag string) bool {
	c, t := canon(current), canon(tag)
	if !semver.IsValid(c) || !semver.IsValid(t) {
		return false
	}
	return semver.Compare(t, c) > 0
}

// Due reports whether a daily job last run at last should run again.
func Due(last, now time.Time) bool { return now.Sub(last) >= 24*time.Hour }

func get(ctx context.Context, c *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Ghostline")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updater: GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// FetchSigned downloads a file and its ed25519 signature (servers.Sign
// format) and verifies it.
func FetchSigned(ctx context.Context, c *http.Client, url, sigURL string, pub ed25519.PublicKey) (raw, sig []byte, err error) {
	if raw, err = get(ctx, c, url, 8<<20); err != nil {
		return nil, nil, err
	}
	if sig, err = get(ctx, c, sigURL, 4096); err != nil {
		return nil, nil, err
	}
	if err := servers.VerifySigned(raw, sig, pub); err != nil {
		return nil, nil, err
	}
	return raw, sig, nil
}

// FetchServerList downloads a list and its ed25519 signature and verifies it.
func FetchServerList(ctx context.Context, c *http.Client, url, sigURL string, pub ed25519.PublicKey) (servers.List, []byte, []byte, error) {
	raw, sig, err := FetchSigned(ctx, c, url, sigURL, pub)
	if err != nil {
		return servers.List{}, nil, nil, err
	}
	l, err := servers.ParseList(raw)
	if err != nil {
		return servers.List{}, nil, nil, err
	}
	return l, raw, sig, nil
}

// FetchDNSCrypt tries each URL until one serves a list whose .minisig
// verifies with minisignKey.
func FetchDNSCrypt(ctx context.Context, c *http.Client, urls []string, minisignKey string) (md, sig []byte, err error) {
	var errs []error
	for _, u := range urls {
		md, err := get(ctx, c, u, 16<<20)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		sig, err := get(ctx, c, u+".minisig", 4096)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := servers.VerifyMinisign(md, sig, minisignKey); err != nil {
			errs = append(errs, fmt.Errorf("%s: bad signature: %w", u, err))
			continue
		}
		return md, sig, nil
	}
	return nil, nil, errors.Join(errs...)
}
