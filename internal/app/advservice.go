package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/scanner/advanced"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
)

// ServerFilter picks servers from the catalog; empty fields match all.
type ServerFilter struct {
	Protocols  []string `json:"protocols"`
	Tags       []string `json:"tags"` // a server must carry every tag
	Sources    []string `json:"sources"`
	PinnedOnly bool     `json:"pinnedOnly"`
}

// AdvScanRequest starts an advanced scan of the filtered catalog, or of the
// pasted servers when Filter is nil.
type AdvScanRequest struct {
	Filter        *ServerFilter `json:"filter,omitempty"`
	Pasted        string        `json:"pasted"`
	PoisonDomains []string      `json:"poisonDomains"`
}

// AdvScanStart reports how many servers will be scanned and the pasted
// lines that were rejected.
type AdvScanStart struct {
	Total   int      `json:"total"`
	Skipped int      `json:"skipped"` // filtered servers left out by the 500 cap
	Bad     []string `json:"bad"`
}

// AdvScanProgress is sent on EventToolsScan.
type AdvScanProgress struct {
	Done    int              `json:"done"`
	Total   int              `json:"total"`
	Result  *advanced.Result `json:"result,omitempty"`
	Running bool             `json:"running"`
}

// AdvRow is one graded server.
type AdvRow struct {
	Result advanced.Result `json:"result"`
	Server model.Server    `json:"server"`
	Pasted bool            `json:"pasted"` // not in the catalog yet
	Pinned bool            `json:"pinned"`
}

// advJob holds the last advanced scan (RAM only, spec 3 §6.3).
type advJob struct {
	servers map[string]model.Server
	pasted  map[string]bool
	results []advanced.Result
}

func toolBusy(tool string) error { return appErr(CodeToolBusy, nil, "tool", tool) }

// StartAdvancedScan grades servers in the background; progress arrives on
// EventToolsScan. It runs outside the connect lock.
func (s *Service) StartAdvancedScan(req AdvScanRequest) (AdvScanStart, error) {
	start := AdvScanStart{Bad: []string{}}
	st := s.x.Settings.Get()
	maxN := st.Tools.Scanner.MaxServers
	if maxN <= 0 {
		maxN = advanced.DefaultMaxServers
	}
	var list []model.Server
	pasted := map[string]bool{}
	if req.Filter != nil {
		list = s.filterCatalog(*req.Filter)
		if len(list) > maxN {
			// More than one scan grades: take the most useful maxN.
			list = s.rankForScan(list)
			start.Skipped = len(list) - maxN
			list = list[:maxN]
		}
	} else {
		add, bad := servers.ParseImport([]byte(req.Pasted))
		start.Bad = append(start.Bad, bad...)
		for _, sv := range add {
			pasted[sv.ID] = true
		}
		list = add
	}
	start.Total = len(list)
	if len(list) > maxN {
		return start, appErr(CodeScanTooMany, fmt.Errorf("%d servers, at most %d per scan", len(list), maxN), "count", len(list), "max", maxN)
	}
	poison := req.PoisonDomains
	if len(poison) == 0 {
		poison = st.ProbeSites
	}
	c := advanced.Checker{Build: s.x.BuildUpstream, Opt: advanced.Options{
		Rounds: st.Tools.Scanner.Rounds, Timeout: time.Duration(st.Tools.Scanner.TimeoutMs) * time.Millisecond,
		TestDomain: firstOr(store.TestDomains(st.TestDomain), "www.google.com"), PoisonDomains: poison, MaxServers: maxN,
	}}

	s.mu.Lock()
	if s.advCancel != nil {
		s.mu.Unlock()
		return start, toolBusy("advanced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.advCancel = cancel
	s.adv = advJob{servers: map[string]model.Server{}, pasted: pasted}
	for _, sv := range list {
		s.adv.servers[sv.ID] = sv
	}
	s.mu.Unlock()

	go func() {
		var rs []advanced.Result
		defer func() {
			if p := recover(); p != nil {
				slog.Error("tools: advanced scan panic", "panic", p, "stack", string(debug.Stack()))
			}
			s.mu.Lock()
			s.adv.results = rs
			s.advCancel = nil
			s.mu.Unlock()
			cancel()
			s.x.Bus.Emit(EventToolsScan, AdvScanProgress{Done: len(rs), Total: len(list), Running: false})
		}()
		rs, _ = advanced.Scan(ctx, list, c, st.Tools.Scanner.Workers, func(done, total int, r advanced.Result) {
			s.x.Bus.Emit(EventToolsScan, AdvScanProgress{Done: done, Total: total, Result: &r, Running: true})
		})
	}()
	return start, nil
}

func (s *Service) filterCatalog(f ServerFilter) []model.Server {
	pinned := s.x.Settings.Get().Pinned
	var out []model.Server
	for _, sv := range s.x.Catalog() {
		switch {
		case len(f.Protocols) > 0 && !slices.Contains(f.Protocols, string(sv.Protocol)):
		case len(f.Sources) > 0 && !slices.Contains(f.Sources, string(sv.Source)):
		case f.PinnedOnly && !slices.Contains(pinned, sv.ID):
		case slices.ContainsFunc(f.Tags, func(t string) bool { return !slices.Contains(sv.Tags, t) }):
		default:
			out = append(out, sv)
		}
	}
	return out
}

// CountScanServers is how many catalog servers match f.
func (s *Service) CountScanServers(f ServerFilter) int { return len(s.filterCatalog(f)) }

// rankForScan orders servers by how useful a grade is: pinned first, then
// those that worked in the last scan (fastest first), then by source
// (built-in, remote list, custom, DNSCrypt list).
func (s *Service) rankForScan(list []model.Server) []model.Server {
	pinned := s.x.Settings.Get().Pinned
	lat := map[string]time.Duration{}
	for _, r := range s.o.ScanResults() {
		if r.OK {
			lat[r.ServerID] = r.Latency
		}
	}
	srcRank := map[model.Source]int{model.SourceBuiltin: 0, model.SourceRemote: 1, model.SourceCustom: 2, model.SourceDNSCrypt: 3}
	rank := func(sv model.Server) (int, time.Duration) {
		if slices.Contains(pinned, sv.ID) {
			return 0, 0
		}
		if l, ok := lat[sv.ID]; ok {
			return 1, l
		}
		return 2 + srcRank[sv.Source], 0
	}
	out := slices.Clone(list)
	slices.SortStableFunc(out, func(a, b model.Server) int {
		ra, la := rank(a)
		rb, lb := rank(b)
		if ra != rb {
			return ra - rb
		}
		return int(la - lb)
	})
	return out
}

// CancelAdvancedScan stops a running advanced scan.
func (s *Service) CancelAdvancedScan() {
	s.mu.Lock()
	c := s.advCancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// AdvancedResults returns the last advanced scan, sorted.
func (s *Service) AdvancedResults() []AdvRow {
	pinned := s.x.Settings.Get().Pinned
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []AdvRow{}
	for _, r := range s.adv.results {
		out = append(out, AdvRow{Result: r, Server: s.adv.servers[r.ServerID], Pasted: s.adv.pasted[r.ServerID], Pinned: slices.Contains(pinned, r.ServerID)})
	}
	return out
}

// AddScannedServers adds pasted servers from the last scan to the custom list.
func (s *Service) AddScannedServers(ids []string) (int, error) {
	s.mu.Lock()
	var addrs []string
	for _, id := range ids {
		if sv, ok := s.adv.servers[id]; ok && s.adv.pasted[id] {
			addrs = append(addrs, sv.Address)
		}
	}
	s.mu.Unlock()
	if len(addrs) == 0 {
		return 0, errors.New("no pasted server selected")
	}
	n, bad := s.AddServers(strings.Join(addrs, "\n"))
	if len(bad) > 0 {
		return n, fmt.Errorf("could not add: %s", strings.Join(bad, ", "))
	}
	return n, nil
}

// ExportAdvancedCSV saves the last scan as CSV (UTF-8 with BOM for Excel).
func (s *Service) ExportAdvancedCSV(ctx context.Context) error {
	var b bytes.Buffer
	b.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"server", "protocol", "ok", "median_ms", "p90_ms", "jitter_ms", "loss", "dnssec", "ad_filter", "poisoned"})
	for _, row := range s.AdvancedResults() {
		r := row.Result
		_ = w.Write([]string{
			csvSafe(row.Server.Name), string(row.Server.Protocol), strconv.FormatBool(r.Reach.OK),
			strconv.FormatInt(r.MedianMs, 10), strconv.FormatInt(r.P90Ms, 10),
			strconv.FormatFloat(r.JitterMs, 'f', 1, 64), strconv.FormatFloat(r.Loss, 'f', 2, 64),
			string(r.DNSSEC), string(r.AdFilter), csvSafe(strings.Join(r.Poisoned, " ")),
		})
	}
	w.Flush()
	name := "ghostline-scan-" + time.Now().Format("2006-01-02") + ".csv"
	return s.x.SaveFile(ctx, name, b.Bytes())
}

// csvSafe stops spreadsheet apps from running a cell as a formula: server
// names come from downloaded lists.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@	", rune(v[0])) {
		return "'" + v
	}
	return v
}

// firstOr returns the first of ds, or def when there is none.
func firstOr(ds []string, def string) string {
	if len(ds) == 0 {
		return def
	}
	return ds[0]
}
