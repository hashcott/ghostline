package backup_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/backup"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

var okValidators = backup.Validators{
	Settings: func(store.Settings) error { return nil },
	List:     func(lists.List) error { return nil },
}

// fileWith builds an export file from d, then lets mut edit its JSON.
func fileWith(t *testing.T, d backup.Data, mut func(m map[string]any)) []byte {
	t.Helper()
	b, err := backup.Build(d, backup.AllSections, "0.5.0", now)
	require.NoError(t, err)
	if mut == nil {
		return b
	}
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	mut(m)
	b, err = json.Marshal(m)
	require.NoError(t, err)
	return b
}

func sec(m map[string]any, name string) map[string]any {
	return m["sections"].(map[string]any)[name].(map[string]any)
}

func current() backup.Data {
	s := store.DefaultSettings()
	s.AdapterGUIDs = []string{"{HERE}"}
	s.Adapters = "manual"
	s.FullWindow = store.WindowSize{Width: 900, Height: 600}
	s.DNSServer.IOSSSID = "MyWifi"
	s.FakeSNI.AckVersion = 1
	s.Proxy.Upstreams = []store.UpstreamProxy{{ID: "corp", Type: "socks5", Addr: "10.0.0.1:1080", PassEnc: "local-secret"}}
	return backup.Data{
		Settings:  s,
		Rules:     store.RulesFile{Version: 1, Rules: []rules.Rule{{Pattern: "old.com", Action: rules.Action{Block: true}, Enabled: true}}, Lists: []lists.List{}},
		Custom:    []model.Server{{ID: "x", Address: "https://old.example/dns-query", Source: model.SourceCustom}},
		Blacklist: "old.com\n",
	}
}

func TestParse_Rejects(t *testing.T) {
	_, err := backup.Parse([]byte(strings.Repeat(" ", backup.MaxSize+1)), current(), okValidators)
	require.ErrorIs(t, err, backup.ErrInvalid)
	_, err = backup.Parse([]byte("{nope"), current(), okValidators)
	require.ErrorIs(t, err, backup.ErrInvalid)
	_, err = backup.Parse([]byte(`{"format":"other","formatVersion":1}`), current(), okValidators)
	require.ErrorIs(t, err, backup.ErrInvalid)
	_, err = backup.Parse([]byte(`{"formatVersion":1}`), current(), okValidators)
	require.ErrorIs(t, err, backup.ErrInvalid)
	_, err = backup.Parse([]byte(`{"format":"ghostline-backup","formatVersion":2}`), current(), okValidators)
	require.ErrorIs(t, err, backup.ErrNewer)
}

func TestParse_OldSettings(t *testing.T) {
	b := fileWith(t, sampleData(), func(m map[string]any) {
		s := sec(m, "settings")
		s["version"] = 4
		delete(s, "tools")
	})
	p, err := backup.Parse(b, current(), okValidators)
	require.NoError(t, err)
	got, _, err := p.Result(current(), backup.Choices{Sections: []string{"settings"}})
	require.NoError(t, err)
	require.Equal(t, 5, got.Settings.Version)
	require.Equal(t, store.DefaultTools(), got.Settings.Tools)
}

func TestParse_SafetyFlags(t *testing.T) {
	d := sampleData()
	d.Settings.FakeSNI = store.FakeSNISettings{Enabled: true, AckVersion: 9}
	d.Settings.Terms = store.TermsSettings{AckVersion: 9, AckAt: "2026-01-01T00:00:00Z"}
	d.Settings.DNSServer.Enabled, d.Settings.DNSServer.ShareLAN = true, true
	d.Settings.Proxy.ShareLAN = true
	d.Settings.StartWithWindows = true
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	var flagOff []string
	for _, w := range p.Preview.Warnings {
		if w.Code == "flag_off" {
			flagOff = append(flagOff, w.Detail)
		}
	}
	require.ElementsMatch(t, []string{"fakeSni.enabled", "dnsServer.enabled", "dnsServer.shareLan", "proxy.shareLan", "startWithWindows"}, flagOff)
	got, changed, err := p.Result(current(), backup.Choices{Sections: []string{"settings"}})
	require.NoError(t, err)
	require.Equal(t, []string{"settings"}, changed)
	s := got.Settings
	require.False(t, s.FakeSNI.Enabled || s.DNSServer.Enabled || s.DNSServer.ShareLAN || s.Proxy.ShareLAN || s.StartWithWindows)
	require.Equal(t, 1, s.FakeSNI.AckVersion, "ack comes from this machine")
	require.Equal(t, current().Settings.Terms, s.Terms, "the terms are accepted on this machine")
}

func TestResult_ReplaceKeepsMachineFields(t *testing.T) {
	p, err := backup.Parse(fileWith(t, sampleData(), nil), current(), okValidators)
	require.NoError(t, err)
	got, _, err := p.Result(current(), backup.Choices{Sections: []string{"settings"}})
	require.NoError(t, err)
	require.Equal(t, []string{"{HERE}"}, got.Settings.AdapterGUIDs)
	require.Equal(t, "manual", got.Settings.Adapters)
	require.Equal(t, store.WindowSize{Width: 900, Height: 600}, got.Settings.FullWindow)
	require.Equal(t, "MyWifi", got.Settings.DNSServer.IOSSSID)
	require.Equal(t, "local-secret", got.Settings.Proxy.Upstreams[0].PassEnc, "same upstream ID keeps this machine's password")
	require.Equal(t, []string{"cf"}, got.Settings.Pinned, "everything else comes from the file")
}

func TestParse_TrustedReset(t *testing.T) {
	d := sampleData()
	d.Rules.Lists = []lists.List{
		{ID: "u", Name: "U", Source: "url", URL: "https://x/u.txt", Format: "ghostline", Action: "perLine", Enabled: true, TrustedForSNI: true},
		{ID: "p", Name: "P", Source: "url", URL: "https://x/p.txt", Format: "ghostline", Action: "perLine", Enabled: true, TrustedForSNI: true, Signed: true},
	}
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	require.Contains(t, p.Preview.Warnings, backup.Warning{Code: "trusted_reset", Detail: "U"})
	got, _, err := p.Result(current(), backup.Choices{Sections: []string{"rules"}, SNIRules: "accept"})
	require.NoError(t, err)
	require.False(t, got.Rules.Lists[0].TrustedForSNI)
	require.True(t, got.Rules.Lists[1].TrustedForSNI)
}

func TestParse_HTTPList(t *testing.T) {
	d := sampleData()
	d.Rules.Lists[0].URL = "http://x/ads.txt"
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	require.Contains(t, p.Preview.Warnings, backup.Warning{Code: "http_list", Detail: "Ads"})
	_, _, err = p.Result(current(), backup.Choices{Sections: []string{"rules"}})
	require.NoError(t, err)
}

func TestParse_SNIRules(t *testing.T) {
	d := sampleData()
	d.Rules.Rules = []rules.Rule{
		{Pattern: "keep.com", Action: rules.Action{Block: true}, Enabled: true},
		{Pattern: "front.com", Action: rules.Action{SNI: "cdn.example"}, Enabled: true},
		{Pattern: "hop.com", Action: rules.Action{Connect: "other.example"}, Enabled: true},
	}
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	require.Len(t, p.Preview.SNIRules, 2)

	_, _, err = p.Result(current(), backup.Choices{Sections: []string{"rules"}})
	require.ErrorIs(t, err, backup.ErrSNIUnconfirmed)
	_, _, err = p.Result(current(), backup.Choices{Sections: []string{"settings"}})
	require.NoError(t, err, "confirmation only matters when rules are imported")

	got, _, err := p.Result(current(), backup.Choices{Sections: []string{"rules"}, SNIRules: "drop"})
	require.NoError(t, err)
	require.Equal(t, []string{"keep.com"}, patterns(got.Rules.Rules))
	got, _, err = p.Result(current(), backup.Choices{Sections: []string{"rules"}, SNIRules: "accept"})
	require.NoError(t, err)
	require.Equal(t, []string{"keep.com", "front.com", "hop.com"}, patterns(got.Rules.Rules))
}

func patterns(rs []rules.Rule) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Pattern)
	}
	return out
}

func TestParse_Counts(t *testing.T) {
	d := sampleData()
	d.Rules.Rules = []rules.Rule{
		{Pattern: "old.com", Action: rules.Action{Block: true}, Enabled: true},
		{Pattern: "new.com", Action: rules.Action{IPs: []netip.Addr{netip.MustParseAddr("1.2.3.4")}}, Enabled: true},
		{Pattern: "bad..com", Action: rules.Action{Block: true}, Enabled: true},
	}
	d.Custom = append(d.Custom, model.Server{Name: "Plain", Address: "udp://1.1.1.1"})
	d.Blacklist = "old.com\nnew.com\n"
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	byName := map[string]backup.SectionPreview{}
	for _, s := range p.Preview.Sections {
		byName[s.Name] = s
	}
	// New: items in the file that are not here now; Replaced: items here now
	// that a replace would remove.
	require.Equal(t, 2, byName["rules"].New, "new.com and the ads list; old.com is already here")
	require.Equal(t, 0, byName["rules"].Replaced)
	require.Len(t, byName["rules"].Errors, 1)
	require.Len(t, byName["customServers"].Errors, 1)
	require.Equal(t, 1, byName["customServers"].New)
	require.Equal(t, 1, byName["customServers"].Replaced)
	require.Equal(t, 1, byName["dpiBlacklist"].New)
	require.Equal(t, "0.5.0", p.Preview.AppVersion)

	_, _, err = p.Result(current(), backup.Choices{Sections: []string{"rules"}})
	require.ErrorIs(t, err, backup.ErrInvalid, "a section with errors cannot be imported")
	_, _, err = p.Result(current(), backup.Choices{Sections: []string{"dpiBlacklist"}})
	require.NoError(t, err)
}

func TestParse_SettingsInvalid(t *testing.T) {
	v := okValidators
	v.Settings = func(store.Settings) error { return errors.New("bad port") }
	p, err := backup.Parse(fileWith(t, sampleData(), nil), current(), v)
	require.NoError(t, err)
	require.Equal(t, []string{"bad port"}, p.Preview.Sections[0].Errors)
}

func TestResult_MergeNoDuplicates(t *testing.T) {
	d := sampleData()
	d.Rules.Rules = []rules.Rule{{Pattern: "old.com", Action: rules.Action{Block: true}, Enabled: true}, {Pattern: "n.com", Action: rules.Action{Block: true}, Enabled: true}}
	d.Custom = []model.Server{{Name: "Old", Address: "https://old.example/dns-query"}, {Name: "New", Address: "https://new.example/dns-query"}}
	d.Blacklist = "old.com\nnew.com\n"
	d.AutoHostlist = []string{"a.com"}
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	got, changed, err := p.Result(current(), backup.Choices{Sections: []string{"rules", "customServers", "dpiBlacklist", "dpiAutoHostlist"}, Merge: true})
	require.NoError(t, err)
	require.Equal(t, []string{"rules", "customServers", "dpiBlacklist", "dpiAutoHostlist"}, changed)
	require.Equal(t, []string{"old.com", "n.com"}, patterns(got.Rules.Rules))
	require.Len(t, got.Rules.Lists, 1)
	require.Len(t, got.Custom, 2)
	require.Equal(t, "New", got.Custom[1].Name)
	require.Equal(t, model.SourceCustom, got.Custom[1].Source)
	require.Equal(t, "old.com\nnew.com\n", got.Blacklist)
	require.Equal(t, []string{"a.com"}, got.AutoHostlist)

	got, _, err = p.Result(current(), backup.Choices{Sections: []string{"customServers"}})
	require.NoError(t, err)
	require.Len(t, got.Custom, 2, "replace takes the file's list")
	require.Equal(t, current().Rules, got.Rules, "unchosen sections are untouched")
}

func TestParse_RejectsUnsafeListIDs(t *testing.T) {
	d := sampleData()
	d.Rules.Lists = []lists.List{
		{ID: `..\..\evil`, Name: "Evil", Source: "url", URL: "https://x/e.txt", Format: "auto", Action: "block", Enabled: true},
		{ID: "dup", Name: "A", Source: "url", URL: "https://x/a.txt", Format: "auto", Action: "block", Enabled: true},
		{ID: "dup", Name: "B", Source: "url", URL: "https://x/b.txt", Format: "auto", Action: "block", Enabled: true},
	}
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	var errs []string
	for _, s := range p.Preview.Sections {
		if s.Name == "rules" {
			errs = s.Errors
		}
	}
	require.Len(t, errs, 2, "the unsafe ID and the duplicate")
	_, _, err = p.Result(current(), backup.Choices{Sections: []string{"rules"}})
	require.ErrorIs(t, err, backup.ErrInvalid)
}

func TestBuild_OmitsEmptyBlacklist(t *testing.T) {
	d := sampleData()
	d.Blacklist = "# only a comment\n\n"
	b, err := backup.Build(d, backup.AllSections, "0.5.0", now)
	require.NoError(t, err)
	require.NotContains(t, string(b), `"dpiBlacklist"`, "an empty blacklist must never wipe another PC's list")
}

func TestParse_EmptyBlacklistIsAnError(t *testing.T) {
	b := fileWith(t, sampleData(), func(m map[string]any) { m["sections"].(map[string]any)["dpiBlacklist"] = "  \n" })
	p, err := backup.Parse(b, current(), okValidators)
	require.NoError(t, err)
	for _, s := range p.Preview.Sections {
		if s.Name == "dpiBlacklist" {
			require.NotEmpty(t, s.Errors)
		}
	}
}

func TestResult_BlacklistScopeNeedsEntries(t *testing.T) {
	d := sampleData()
	d.Settings.DPI.Scope = "blacklist"
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	empty := current()
	empty.Blacklist = ""
	_, _, err = p.Result(empty, backup.Choices{Sections: []string{"settings"}})
	require.ErrorIs(t, err, backup.ErrInvalid, "scope=blacklist with no blacklist bypasses nothing")
	_, _, err = p.Result(empty, backup.Choices{Sections: []string{"settings", "dpiBlacklist"}})
	require.NoError(t, err, "importing the file's blacklist too is fine")
	_, _, err = p.Result(current(), backup.Choices{Sections: []string{"settings"}})
	require.NoError(t, err, "this PC already has a blacklist")
}

func TestResult_UpstreamRefsCheckedAgainstFinalSettings(t *testing.T) {
	d := sampleData() // its settings have upstream "corp"
	d.Rules.Rules = []rules.Rule{{Pattern: "a.com", Action: rules.Action{Upstream: "corp"}, Enabled: true}}
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	here := current()
	here.Settings.Proxy.Upstreams = nil // this PC has no "corp" proxy
	_, _, err = p.Result(here, backup.Choices{Sections: []string{"rules"}})
	require.ErrorIs(t, err, backup.ErrInvalid, "rules alone would point at a proxy that does not exist here")
	_, _, err = p.Result(here, backup.Choices{Sections: []string{"settings", "rules"}})
	require.NoError(t, err, "importing the settings brings the proxy along")

	// Replacing settings must not orphan rules already here.
	here = current()
	here.Rules.Rules = []rules.Rule{{Pattern: "b.com", Action: rules.Action{Upstream: "local"}, Enabled: true}}
	here.Settings.Proxy.Upstreams = append(here.Settings.Proxy.Upstreams, store.UpstreamProxy{ID: "local", Type: "http", Addr: "10.0.0.2:8080"})
	_, _, err = p.Result(here, backup.Choices{Sections: []string{"settings"}})
	require.ErrorIs(t, err, backup.ErrInvalid)
}

func TestResult_RuleCountCapped(t *testing.T) {
	d := sampleData()
	d.Rules.Rules = nil
	for i := range 6000 {
		d.Rules.Rules = append(d.Rules.Rules, rules.Rule{Pattern: fmt.Sprintf("f%d.com", i), Action: rules.Action{Block: true}, Enabled: true})
	}
	p, err := backup.Parse(fileWith(t, d, nil), current(), okValidators)
	require.NoError(t, err)
	here := current()
	for i := range 6000 {
		here.Rules.Rules = append(here.Rules.Rules, rules.Rule{Pattern: fmt.Sprintf("h%d.com", i), Action: rules.Action{Block: true}, Enabled: true})
	}
	_, _, err = p.Result(here, backup.Choices{Sections: []string{"rules"}, Merge: true})
	require.ErrorIs(t, err, backup.ErrInvalid)
	_, _, err = p.Result(here, backup.Choices{Sections: []string{"rules"}})
	require.NoError(t, err, "replace keeps only the file's 6000")
}

func TestBuild_EmptySectionsCanReplace(t *testing.T) {
	d := sampleData()
	d.Custom, d.AutoHostlist = nil, nil
	b, err := backup.Build(d, backup.AllSections, "0.5.0", now)
	require.NoError(t, err)
	require.Contains(t, string(b), `"customServers": []`)
	require.Contains(t, string(b), `"dpiAutoHostlist": []`)
	p, err := backup.Parse(b, current(), okValidators)
	require.NoError(t, err)
	got, changed, err := p.Result(current(), backup.Choices{Sections: []string{"customServers", "dpiAutoHostlist"}})
	require.NoError(t, err)
	require.Equal(t, []string{"customServers", "dpiAutoHostlist"}, changed)
	require.Empty(t, got.Custom)
}

func TestResult_MergeRenamesCollidingListID(t *testing.T) {
	d := sampleData()
	d.Rules.Lists = []lists.List{{ID: "ads", Name: "Other ads", Source: "url", URL: "https://y/other.txt", Format: "auto", Action: "block", Enabled: true}}
	here := current()
	here.Rules.Lists = []lists.List{{ID: "ads", Name: "Ads", Source: "url", URL: "https://x/ads.txt", Format: "auto", Action: "block", Enabled: true}}
	p, err := backup.Parse(fileWith(t, d, nil), here, okValidators)
	require.NoError(t, err)
	got, _, err := p.Result(here, backup.Choices{Sections: []string{"rules"}, Merge: true})
	require.NoError(t, err)
	require.Len(t, got.Rules.Lists, 2, "a different list with the same ID is kept, not dropped")
	require.Equal(t, "https://y/other.txt", got.Rules.Lists[1].URL)
	require.NotEqual(t, "ads", got.Rules.Lists[1].ID)
	require.True(t, lists.ValidID(got.Rules.Lists[1].ID))
}
