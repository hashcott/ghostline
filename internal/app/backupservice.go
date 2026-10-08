package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/backup"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
)

// importTTL is how long a preview stays valid (spec 3 §9.3).
const importTTL = 10 * time.Minute

// ImportPreview is what the user confirms before an import.
type ImportPreview struct {
	Token   string         `json:"token"` // "" when the file dialog was cancelled
	Path    string         `json:"path"`
	Preview backup.Preview `json:"preview"`
}

type importTicket struct {
	token   string
	plan    *backup.Plan
	expires time.Time
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// LoadBackupData reads everything a backup holds from the data directory
// (used by --export, which runs without the UI).
func LoadBackupData(p store.Paths) (backup.Data, error) {
	var d backup.Data
	var err error
	if d.Settings, _, err = store.LoadSettings(p.Settings); err != nil {
		return d, err
	}
	if d.Rules, _, err = store.LoadRules(p.Rules); err != nil {
		return d, err
	}
	if err := store.ReadJSON(p.ServersCustom, &d.Custom); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return d, err
	}
	if b, err := os.ReadFile(p.DPIBlacklist); err == nil {
		d.Blacklist = string(b)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return d, err
	}
	if b, err := os.ReadFile(p.DPIAutoHostlist); err == nil {
		d.AutoHostlist = cleanDomains(strings.Split(string(b), "\n"))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return d, err
	}
	return d, nil
}

// ExportTo writes a full backup of the data directory to path.
func ExportTo(p store.Paths, path, appVersion string) error {
	d, err := LoadBackupData(p)
	if err != nil {
		return err
	}
	b, err := backup.Build(d, backup.AllSections, appVersion, time.Now().UTC())
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (s *Service) currentBackupData() (backup.Data, error) {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	return s.currentBackupDataLocked()
}

// currentBackupDataLocked is currentBackupData for callers holding rmu.
func (s *Service) currentBackupDataLocked() (backup.Data, error) {
	d := backup.Data{Settings: s.x.Settings.Get(), Rules: s.rf}
	var err error
	if d.Custom, err = s.x.LoadCustom(); err != nil {
		return d, err
	}
	if d.Blacklist, err = s.GetDPIBlacklist(); err != nil {
		return d, err
	}
	d.AutoHostlist, err = s.GetDPIAutoHostlist()
	return d, err
}

// ExportSettings saves the chosen sections through the save dialog.
func (s *Service) ExportSettings(ctx context.Context, sections []string) (err error) {
	defer func() {
		if err != nil {
			slog.Warn("backup: export failed", "err", err)
		}
	}()
	d, err := s.currentBackupData()
	if err != nil {
		return appErr(CodeExportWriteFailed, err, "detail", err.Error())
	}
	b, err := backup.Build(d, sections, s.x.Info().Version, s.clock().UTC())
	if err != nil {
		return err
	}
	if err := s.x.SaveFile(ctx, backup.FileName(s.clock()), b); err != nil {
		return appErr(CodeExportWriteFailed, err, "detail", err.Error())
	}
	return nil
}

func (s *Service) backupValidators() backup.Validators {
	return backup.Validators{
		Settings: func(n store.Settings) error {
			if err := s.validateSettings(n); err != nil {
				return err
			}
			return store.ValidateDNSServer(n.DNSServer, n.Proxy.Port)
		},
		List: func(l lists.List) error { return s.validateList(l) },
	}
}

// PreviewImport asks for a backup file and returns what importing it would
// do. Nothing changes until ApplyImport.
func (s *Service) PreviewImport(ctx context.Context) (ImportPreview, error) {
	if s.x.OpenFile == nil {
		return ImportPreview{}, errors.New("no file dialog")
	}
	invalid := func(detail string, cause error) error { return appErr(CodeImportInvalid, cause, "detail", detail) }
	name, b, err := s.x.OpenFile(ctx, brand.AppName)
	if err != nil {
		return ImportPreview{}, invalid(err.Error(), err) // could not read it
	}
	if name == "" {
		return ImportPreview{}, nil // cancelled
	}
	if len(b) > backup.MaxSize {
		return ImportPreview{}, invalid("too_large", nil)
	}
	cur, err := s.currentBackupData()
	if err != nil {
		return ImportPreview{}, err
	}
	plan, err := backup.Parse(b, cur, s.backupValidators())
	switch {
	case errors.Is(err, backup.ErrNewer):
		return ImportPreview{}, invalid("newer", err)
	case err != nil:
		return ImportPreview{}, invalid(err.Error(), err)
	}
	tok := make([]byte, 16)
	_, _ = rand.Read(tok)
	t := &importTicket{token: hex.EncodeToString(tok), plan: plan, expires: s.clock().Add(importTTL)}
	s.mu.Lock()
	s.imp = t // a new preview replaces the previous one
	s.mu.Unlock()
	return ImportPreview{Token: t.token, Path: name, Preview: plan.Preview}, nil
}

// ApplyImport imports the chosen sections of the previewed file. It is
// refused unless Ghostline is disconnected. Files are written atomically;
// on any failure every file is restored.
func (s *Service) ApplyImport(token string, c backup.Choices) error {
	// Hold the connect lock for the whole import: a Connect that starts
	// meanwhile waits and then reads the imported files, never half of them.
	if !s.o.opMu.TryLock() {
		return appErr(CodeImportWhileConnected, nil)
	}
	defer s.o.opMu.Unlock()
	if st := s.o.Snapshot().Status; st != StatusDisconnected && st != StatusError {
		return appErr(CodeImportWhileConnected, nil)
	}
	s.mu.Lock()
	t := s.imp
	s.mu.Unlock()
	if t == nil || t.token != token || s.clock().After(t.expires) {
		return appErr(CodeImportExpired, nil)
	}
	// Hold the rules lock from read to reload so no rules save lands in
	// between and is lost, or overwrites the import.
	s.rmu.Lock()
	refresh, err := s.applyImportLocked(t, c)
	s.rmu.Unlock()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.imp = nil
	s.mu.Unlock()
	if refresh && s.x.Fetcher != nil {
		_ = s.RefreshList("") // download the subscribed lists
	}
	return nil
}

// applyImportLocked writes and reloads the import; callers hold rmu. It
// reports whether the rules changed.
func (s *Service) applyImportLocked(t *importTicket, c backup.Choices) (bool, error) {
	cur, err := s.currentBackupDataLocked()
	if err != nil {
		return false, err
	}
	target, changed, err := t.plan.Result(cur, c)
	switch {
	case errors.Is(err, backup.ErrSNIUnconfirmed):
		return false, appErr(CodeImportInvalid, err, "detail", "sni_unconfirmed")
	case err != nil:
		return false, appErr(CodeImportInvalid, err, "detail", err.Error())
	}
	ws, err := s.importWrites(target, changed)
	if err != nil {
		return false, err
	}
	if err := backup.Apply(ws); err != nil {
		var we *backup.WriteError
		file := ""
		if errors.As(err, &we) {
			file = filepath.Base(we.Path)
		}
		slog.Warn("backup: writing the import failed", "file", file, "err", err)
		return false, appErr(CodeImportWriteFailed, err, "file", file)
	}
	s.reloadAfterImportLocked(cur, target, changed)
	return slices.Contains(changed, backup.SecRules), nil
}

func (s *Service) autoHostlistPath() string {
	if s.o.d.AutoHostlistPath != "" {
		return s.o.d.AutoHostlistPath
	}
	return s.x.Paths.DPIAutoHostlist
}

func jsonFile(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	return append(b, '\n'), err
}

func (s *Service) importWrites(d backup.Data, changed []string) ([]backup.Write, error) {
	var ws []backup.Write
	for _, name := range changed {
		var w backup.Write
		var err error
		switch name {
		case backup.SecSettings:
			w.Path = s.x.Paths.Settings
			w.Data, err = jsonFile(d.Settings)
		case backup.SecRules:
			w.Path = s.x.RulesPath
			w.Data, err = jsonFile(d.Rules)
		case backup.SecCustom:
			custom := d.Custom
			if custom == nil {
				custom = []model.Server{}
			}
			w.Path = s.x.Paths.ServersCustom
			w.Data, err = jsonFile(custom)
		case backup.SecBlacklist:
			w.Path, w.Data = s.x.Paths.DPIBlacklist, []byte(d.Blacklist)
		case backup.SecAutoHostlist:
			text := strings.Join(cleanDomains(d.AutoHostlist), "\n")
			if text != "" {
				text += "\n"
			}
			w.Path, w.Data = s.autoHostlistPath(), []byte(text)
		default:
			return nil, fmt.Errorf("import: unknown section %q", name)
		}
		if err != nil {
			return nil, err
		}
		ws = append(ws, w)
	}
	return ws, nil
}

// reloadAfterImportLocked brings the running app in line with the written
// files; callers hold rmu.
func (s *Service) reloadAfterImportLocked(old, d backup.Data, changed []string) {
	for _, name := range changed {
		switch name {
		case backup.SecSettings:
			s.x.Settings.set(d.Settings)
			if s.x.OnSettingsChanged != nil {
				s.x.OnSettingsChanged(old.Settings, d.Settings)
			}
		case backup.SecRules:
			s.rf = d.Rules
			s.recompileLocked("")
		case backup.SecCustom:
			warnIgnored("custom servers save", s.x.SaveCustom(d.Custom)) // same content; reloads the catalog
		}
	}
}
