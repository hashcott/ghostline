package store

import (
	"errors"
	"io/fs"
	"log/slog"
	"time"
)

// Meta records when background jobs last ran (server lists daily, release
// check at start and every 6 hours).
type Meta struct {
	LastUpdateCheck time.Time `json:"lastUpdateCheck"`
	LastServerList  time.Time `json:"lastServerList"`
	LastDNSCrypt    time.Time `json:"lastDnsCrypt"`
	// LastStrategyList is the zapret2 strategy list download (daily).
	LastStrategyList time.Time `json:"lastStrategyList"`
	// LatestTag/URL remember the newest release seen, so the notice survives
	// restarts between the daily checks.
	LatestTag string `json:"latestTag,omitempty"`
	LatestURL string `json:"latestUrl,omitempty"`
	// ReconnectAfterUpdate is set just before the app starts an installer
	// while connected; the updated app connects again at its first start.
	ReconnectAfterUpdate bool `json:"reconnectAfterUpdate,omitempty"`
}

// LoadMeta reads meta.json; a missing or unreadable file is a zero Meta.
func LoadMeta(path string) Meta {
	var m Meta
	if err := ReadJSON(path, &m); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("store: reading meta.json failed; using defaults", "path", path, "err", err)
	}
	return m
}

// SaveMeta writes meta.json atomically.
func SaveMeta(path string, m Meta) error { return WriteJSONAtomic(path, m) }
