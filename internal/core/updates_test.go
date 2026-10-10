package core

import (
	"errors"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/updater"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func fetchOK(tag string) (func() (updater.Release, error), *int) {
	n := 0
	return func() (updater.Release, error) {
		n++
		return updater.Release{Tag: tag, URL: "https://example/" + tag}, nil
	}, &n
}

func TestReleaseCheck_FindsNewerAndRemembersIt(t *testing.T) {
	var meta store.Meta
	fetch, _ := fetchOK("v0.1.1")
	r, ok := releaseCheck(&meta, now, "0.1.0", false, false, fetch)
	require.True(t, ok)
	require.Equal(t, "v0.1.1", r.Tag)
	require.Equal(t, "v0.1.1", meta.LatestTag)
	require.Equal(t, "https://example/v0.1.1", meta.LatestURL)
	require.Equal(t, now, meta.LastUpdateCheck)
}

// Between the periodic checks the release found earlier is still shown.
func TestReleaseCheck_RemembersAcrossRestartWithoutFetching(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-time.Hour), LatestTag: "v0.1.1", LatestURL: "https://example/v0.1.1"}
	fetch, calls := fetchOK("v9.9.9")
	r, ok := releaseCheck(&meta, now, "0.1.0", false, false, fetch)
	require.True(t, ok)
	require.Equal(t, "v0.1.1", r.Tag)
	require.Zero(t, *calls, "not due yet")
}

// After updating to the remembered version there is nothing to announce.
func TestReleaseCheck_NothingWhenAlreadyCurrent(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-time.Hour), LatestTag: "v0.1.1"}
	fetch, _ := fetchOK("v0.1.1")
	_, ok := releaseCheck(&meta, now, "0.1.1", false, false, fetch)
	require.False(t, ok)
}

// A failed check keeps what was known and retries later.
func TestReleaseCheck_FailureKeepsKnownRelease(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-48 * time.Hour), LatestTag: "v0.1.1", LatestURL: "u"}
	r, ok := releaseCheck(&meta, now, "0.1.0", false, false, func() (updater.Release, error) { return updater.Release{}, errors.New("offline") })
	require.True(t, ok)
	require.Equal(t, "v0.1.1", r.Tag)
	require.Equal(t, now.Add(-48*time.Hour), meta.LastUpdateCheck, "a failure is not a completed check")
}

// Meta written by v0.1.0/0.1.1 has a recent lastUpdateCheck but no
// latestTag: nothing is known, so check now instead of waiting a day.
func TestReleaseCheck_MissingLatestTagChecksNow(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-time.Hour)}
	fetch, calls := fetchOK("v0.2.0")
	r, ok := releaseCheck(&meta, now, "0.1.2", false, false, fetch)
	require.Equal(t, 1, *calls)
	require.True(t, ok)
	require.Equal(t, "v0.2.0", r.Tag)
}

// Every app start checks, even right after a previous check.
func TestReleaseCheck_ChecksOnStartup(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-time.Minute), LatestTag: "v0.1.2", LatestURL: "u"}
	fetch, calls := fetchOK("v0.2.0")
	r, ok := releaseCheck(&meta, now, "0.1.2", true, false, fetch)
	require.Equal(t, 1, *calls)
	require.True(t, ok)
	require.Equal(t, "v0.2.0", r.Tag)
}

// While running, the check repeats every 6 hours.
func TestReleaseCheck_EverySixHours(t *testing.T) {
	meta := store.Meta{LastUpdateCheck: now.Add(-5 * time.Hour), LatestTag: "v0.1.2"}
	fetch, calls := fetchOK("v0.2.0")
	_, ok := releaseCheck(&meta, now, "0.1.2", false, false, fetch)
	require.Zero(t, *calls)
	require.False(t, ok)
	meta.LastUpdateCheck = now.Add(-6 * time.Hour)
	_, ok = releaseCheck(&meta, now, "0.1.2", false, false, fetch)
	require.Equal(t, 1, *calls)
	require.True(t, ok)
}

func TestReleaseCheck_RememberedBetaOnlyOnBetaChannel(t *testing.T) {
	offline := func() (updater.Release, error) { return updater.Release{}, errors.New("offline") }
	meta := store.Meta{LatestTag: "v0.3.0-beta.1", LatestURL: "u", LastUpdateCheck: now}
	_, ok := releaseCheck(&meta, now, "0.2.0", false, false, offline)
	require.False(t, ok, "a beta remembered before switching to stable is not announced")
	r, ok := releaseCheck(&meta, now, "0.2.0", false, true, offline)
	require.True(t, ok)
	require.Equal(t, "v0.3.0-beta.1", r.Tag)
}
