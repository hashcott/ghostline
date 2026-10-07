package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/formats"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
)

// DefaultMaxEntries caps the entries of all enabled lists together.
const DefaultMaxEntries = 2_000_000

// RulesView is the Rules page data.
type RulesView struct {
	Rules []rules.Rule `json:"rules"`
	Text  string       `json:"text"`
	Lists []lists.List `json:"lists"`
}

// RulesCompiled is emitted after every recompile.
type RulesCompiled struct {
	Count int   `json:"count"`
	Ms    int64 `json:"ms"`
}

// ListsProgress reports a list download.
type ListsProgress struct {
	ID      string `json:"id"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

func (s *Service) upstreamIDs() []string {
	var ids []string
	for _, u := range s.x.Settings.Get().Proxy.Upstreams {
		ids = append(ids, u.ID)
	}
	return ids
}

// LoadRules reads rules.json and compiles it with the cached lists. The
// shell calls it at startup; it is a function so Wails does not bind it.
func LoadRules(s *Service) (recovered bool, err error) {
	f, recovered, err := store.LoadRules(s.x.RulesPath)
	s.rmu.Lock()
	defer s.rmu.Unlock()
	s.rf = f
	s.recompileLocked("")
	return recovered, err
}

// recompileLocked rebuilds the matcher from s.rf and the list caches. When
// the entry cap is exceeded, the list named changed is disabled. Callers
// hold rmu.
func (s *Service) recompileLocked(changed string) {
	if s.x.Rules == nil {
		return
	}
	start := time.Now()
	maxEntries := s.x.MaxEntries
	if maxEntries == 0 {
		maxEntries = DefaultMaxEntries
	}
	total := 0
	for _, r := range s.rf.Rules {
		if r.Enabled {
			total++
		}
	}
	var sets []rules.ListSet
	disabled := ""
	for i, l := range s.rf.Lists {
		if !l.Enabled || s.x.Fetcher == nil {
			continue
		}
		res, err := s.x.Fetcher.LoadCached(l)
		if err != nil {
			continue
		}
		if total+len(res.Entries) > maxEntries {
			if l.ID == changed {
				s.rf.Lists[i].Enabled = false
				s.rf.Lists[i].LastError = CodeListTooLarge
				disabled = l.ID
			}
			continue
		}
		set, err := lists.ToListSet(l, res)
		if err != nil {
			continue
		}
		total += len(res.Entries)
		sets = append(sets, set)
	}
	c, err := rules.Compile(s.rf.Rules, sets)
	if err != nil {
		s.o.log("rules", CodeRulesParse, "line", 0)
		return
	}
	s.x.Rules.Store(c)
	if disabled != "" {
		_ = store.SaveRules(s.x.RulesPath, s.rf)
		s.o.log("rules", CodeListTooLarge, "id", disabled)
	}
	s.x.Bus.Emit(EventRulesCompiled, RulesCompiled{Count: c.Count(), Ms: time.Since(start).Milliseconds()})
	s.o.OnRulesCompiled()
}

func (s *Service) saveRulesLocked() error { return store.SaveRules(s.x.RulesPath, s.rf) }

// GetRules returns user rules (table and text) and lists.
func (s *Service) GetRules() RulesView {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	rs := slices.Clone(s.rf.Rules)
	if rs == nil {
		rs = []rules.Rule{}
	}
	ls := slices.Clone(s.rf.Lists)
	if ls == nil {
		ls = []lists.List{}
	}
	return RulesView{Rules: rs, Text: rules.FormatText(rs), Lists: ls}
}

// SaveRulesTable validates and stores rules edited in the table. On any
// error nothing is saved; Line is the 1-based row.
func (s *Service) SaveRulesTable(rs []rules.Rule) []rules.LineError {
	ids := s.upstreamIDs()
	errs := []rules.LineError{}
	if len(rs) > rules.MaxUserRules {
		return append(errs, rules.LineError{Line: rules.MaxUserRules + 1, Msg: "too many rules"})
	}
	for i, r := range rs {
		if err := rules.ValidateRule(r, ids); err != nil {
			errs = append(errs, rules.LineError{Line: i + 1, Msg: err.Error()})
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return s.storeRules(rs)
}

// SaveRulesText parses and stores rules from the text editor. On any error
// nothing is saved.
func (s *Service) SaveRulesText(text string) []rules.LineError {
	rs, errs := rules.ParseText(text, s.upstreamIDs())
	if len(errs) > 0 {
		return errs
	}
	return s.storeRules(rs)
}

func (s *Service) storeRules(rs []rules.Rule) []rules.LineError {
	if rs == nil {
		rs = []rules.Rule{}
	}
	s.rmu.Lock()
	defer s.rmu.Unlock()
	old := s.rf.Rules
	s.rf.Rules = rs
	if err := s.saveRulesLocked(); err != nil {
		s.rf.Rules = old
		return []rules.LineError{{Line: 0, Msg: err.Error()}}
	}
	s.recompileLocked("")
	return []rules.LineError{}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func newListID(name string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(slug) > 24 {
		slug = slug[:24]
	}
	if slug == "" {
		slug = "list"
	}
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return slug + "-" + hex.EncodeToString(b)
}

func (s *Service) validateList(l lists.List) error {
	res := lists.Result{}
	if l.Format == string(formats.Ghostline) {
		res.Format = formats.Ghostline // per-line actions: no shared action to check
	}
	if _, err := lists.ToListSet(l, res); err != nil {
		return err
	}
	if up, ok := strings.CutPrefix(l.Action, "upstream="); ok && !slices.Contains(s.upstreamIDs(), up) {
		return fmt.Errorf("unknown upstream %q", up)
	}
	switch l.Source {
	case "url":
		if _, _, err := lists.NormalizeURL(l.URL); err != nil {
			return err
		}
	case "file":
		if s.x.NoFileLists {
			return lists.ErrFileListsOff
		}
		if l.Path == "" {
			return errors.New("lists: file path is empty")
		}
	default:
		return errors.New("lists: source must be url or file")
	}
	return nil
}

// AddList adds a list and downloads it in the background.
func (s *Service) AddList(l lists.List) (lists.List, error) {
	if l.Format == "" {
		l.Format = "auto"
	}
	if l.UpdateHours == 0 && l.Source == "url" {
		l.UpdateHours = 24
	}
	l.Enabled = true
	if err := s.validateList(l); err != nil {
		return lists.List{}, err
	}
	l.ID = newListID(l.Name)
	s.rmu.Lock()
	s.rf.Lists = append(s.rf.Lists, l)
	err := s.saveRulesLocked()
	s.rmu.Unlock()
	if err != nil {
		return lists.List{}, err
	}
	s.background(func(ctx context.Context) { _ = s.refresh(ctx, l.ID) })
	return l, nil
}

// UpdateList replaces a list's settings (name, URL, action, format,
// schedule, enabled) and recompiles.
func (s *Service) UpdateList(l lists.List) error {
	if err := s.validateList(l); err != nil {
		return err
	}
	s.rmu.Lock()
	defer s.rmu.Unlock()
	i := slices.IndexFunc(s.rf.Lists, func(x lists.List) bool { return x.ID == l.ID })
	if i < 0 {
		return fmt.Errorf("lists: no list %q", l.ID)
	}
	cur := s.rf.Lists[i]
	cur.Name, cur.Source, cur.URL, cur.Path, cur.Format, cur.Action, cur.Enabled, cur.UpdateHours =
		l.Name, l.Source, l.URL, l.Path, l.Format, l.Action, l.Enabled, l.UpdateHours
	s.rf.Lists[i] = cur
	if err := s.saveRulesLocked(); err != nil {
		return err
	}
	s.recompileLocked(l.ID)
	return nil
}

// DeleteList removes a list and its cache.
func (s *Service) DeleteList(id string) error {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	n := len(s.rf.Lists)
	s.rf.Lists = slices.DeleteFunc(s.rf.Lists, func(x lists.List) bool { return x.ID == id })
	if len(s.rf.Lists) == n {
		return fmt.Errorf("lists: no list %q", id)
	}
	if err := s.saveRulesLocked(); err != nil {
		return err
	}
	if s.x.Fetcher != nil {
		_ = os.Remove(filepath.Join(s.x.Fetcher.Dir, id+".txt"))
		incs, _ := filepath.Glob(filepath.Join(s.x.Fetcher.Dir, id+"@*.txt"))
		for _, p := range incs {
			_ = os.Remove(p)
		}
	}
	s.recompileLocked("")
	return nil
}

// MoveList moves a list to position to (0-based); order sets priority.
func (s *Service) MoveList(id string, to int) error {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	i := slices.IndexFunc(s.rf.Lists, func(x lists.List) bool { return x.ID == id })
	if i < 0 {
		return fmt.Errorf("lists: no list %q", id)
	}
	l := s.rf.Lists[i]
	s.rf.Lists = slices.Delete(s.rf.Lists, i, i+1)
	to = max(0, min(to, len(s.rf.Lists)))
	s.rf.Lists = slices.Insert(s.rf.Lists, to, l)
	if err := s.saveRulesLocked(); err != nil {
		return err
	}
	s.recompileLocked("")
	return nil
}

// RefreshList downloads one list now ("" = every enabled list), in the
// background; progress arrives as events.
func (s *Service) RefreshList(id string) error {
	var ids []string
	s.rmu.Lock()
	for _, l := range s.rf.Lists {
		if id == "" && l.Enabled || l.ID == id {
			ids = append(ids, l.ID)
		}
	}
	s.rmu.Unlock()
	if id != "" && len(ids) == 0 {
		return fmt.Errorf("lists: no list %q", id)
	}
	s.background(func(ctx context.Context) {
		for _, x := range ids {
			_ = s.refresh(ctx, x)
		}
	})
	return nil
}

// RefreshListNow downloads one list synchronously (used by the scheduler).
func RefreshListNow(s *Service, ctx context.Context, id string) error { return s.refresh(ctx, id) }

func (s *Service) refresh(ctx context.Context, id string) error {
	s.rmu.Lock()
	i := slices.IndexFunc(s.rf.Lists, func(x lists.List) bool { return x.ID == id })
	if i < 0 || s.x.Fetcher == nil {
		s.rmu.Unlock()
		return fmt.Errorf("lists: no list %q", id)
	}
	l := s.rf.Lists[i]
	s.rmu.Unlock()

	s.x.Bus.Emit(EventListsProgress, ListsProgress{ID: id, Running: true})
	_, err := s.x.Fetcher.Fetch(ctx, &l)

	s.rmu.Lock()
	if j := slices.IndexFunc(s.rf.Lists, func(x lists.List) bool { return x.ID == id }); j >= 0 {
		cur := s.rf.Lists[j]
		// Keep the user's edits made meanwhile; take the fetch metadata.
		cur.LastUpdated, cur.ETag, cur.LastModified, cur.Detected = l.LastUpdated, l.ETag, l.LastModified, l.Detected
		cur.Counts, cur.Skipped, cur.SkippedSamples, cur.LastError = l.Counts, l.Skipped, l.SkippedSamples, l.LastError
		s.rf.Lists[j] = cur
		_ = s.saveRulesLocked()
		s.recompileLocked(id)
	}
	s.rmu.Unlock()

	p := ListsProgress{ID: id}
	if err != nil {
		code := CodeListFetch
		switch {
		case errors.Is(err, lists.ErrUnsupported):
			code = CodeListUnsupported
		case errors.Is(err, lists.ErrTooLarge):
			code = CodeListTooLarge
		}
		p.Error = code
		s.o.log("rules", code, "id", id)
	}
	s.x.Bus.Emit(EventListsProgress, p)
	return err
}

// Lists returns a copy of the list metadata (for the scheduler).
func (s *Service) listsSnapshot() []lists.List {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	return slices.Clone(s.rf.Lists)
}

// ListsForScheduler exposes the list metadata to the shell's scheduler
// without binding it to the UI.
func ListsForScheduler(s *Service) []lists.List { return s.listsSnapshot() }

// Catalog returns the quick-add catalog.
func (s *Service) Catalog() []lists.CatalogItem { return lists.Catalog() }

// Explain says which rule or list decides host.
func (s *Service) Explain(host string) rules.Decision {
	if s.x.Rules == nil {
		return rules.Decision{}
	}
	return s.x.Rules.Load().Explain(host)
}

// background runs fn in a goroutine tracked for shutdown and tests.
func (s *Service) background(fn func(ctx context.Context)) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		fn(ctx)
	}()
}

func (s *Service) waitBackground() { s.bg.Wait() }
