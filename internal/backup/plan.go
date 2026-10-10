package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
)

// Validators check imported values the way the app checks a save.
type Validators struct {
	Settings func(store.Settings) error
	List     func(lists.List) error
}

// Warning is something the import changed or the user should know.
type Warning struct {
	Code   string `json:"code"` // flag_off | trusted_reset | http_list
	Detail string `json:"detail"`
}

// SectionPreview counts what importing a section would do. New: items in
// the file that are not here now. Replaced: items here now that a replace
// removes. A section with Errors cannot be imported.
type SectionPreview struct {
	Name     string   `json:"name"`
	New      int      `json:"new"`
	Replaced int      `json:"replaced"`
	Errors   []string `json:"errors"`
}

// Preview is shown before the user confirms an import.
type Preview struct {
	Sections   []SectionPreview `json:"sections"`
	Warnings   []Warning        `json:"warnings"`
	SNIRules   []rules.Rule     `json:"sniRules"`
	AppVersion string           `json:"appVersion"`
	CreatedAt  time.Time        `json:"createdAt"`
}

// Plan is a parsed, sanitised import waiting for the user's choices.
type Plan struct {
	Preview Preview

	present   map[string]bool
	errs      map[string]bool
	settings  store.Settings
	rules     []rules.Rule
	lists     []lists.List
	custom    []model.Server
	blacklist string
	autoHost  []string
}

var (
	ErrInvalid        = errors.New("backup: invalid file")
	ErrNewer          = errors.New("backup: made by a newer Ghostline")
	ErrSNIUnconfirmed = errors.New("backup: rules with sni= or connect= need confirmation")
)

// scopeBlacklist is dpi.ScopeBlacklist (DPI bypass only for listed sites).
const scopeBlacklist = "blacklist"

// Choices are the user's answers on the preview.
type Choices struct {
	Sections []string `json:"sections"`
	Merge    bool     `json:"merge"`    // rules, custom servers, DPI lists: add what is missing, keep the rest
	SNIRules string   `json:"sniRules"` // accept | drop
}

// Parse reads an export file and prepares it against the current data,
// applying the import safety rules (spec 3 §9.4).
func Parse(b []byte, cur Data, v Validators) (*Plan, error) {
	if len(b) > MaxSize {
		return nil, fmt.Errorf("%w: larger than 8 MB", ErrInvalid)
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	switch {
	case f.Format != Format:
		return nil, fmt.Errorf("%w: not a Ghostline backup", ErrInvalid)
	case f.FormatVersion > FormatVersion:
		return nil, ErrNewer
	case f.FormatVersion < 1:
		return nil, fmt.Errorf("%w: bad formatVersion", ErrInvalid)
	}
	p := &Plan{present: map[string]bool{}, errs: map[string]bool{}}
	p.Preview = Preview{AppVersion: f.AppVersion, CreatedAt: f.CreatedAt, Warnings: []Warning{}, SNIRules: []rules.Rule{}, Sections: []SectionPreview{}}
	target := cur.Settings
	sec := f.Sections

	if sec.Settings != nil {
		sp := SectionPreview{Name: SecSettings, Replaced: 1}
		s, err := store.MigrateSettings(sec.Settings)
		if err != nil {
			sp.Errors = append(sp.Errors, err.Error())
		} else {
			s = p.sanitiseSettings(s, cur.Settings)
			if err := v.Settings(s); err != nil {
				sp.Errors = append(sp.Errors, err.Error())
			}
			p.settings, target = s, s
		}
		p.add(sp)
	}
	if sec.Rules != nil {
		p.parseRules(sec.Rules, cur, target, v)
	}
	if sec.CustomServers != nil {
		sp := SectionPreview{Name: SecCustom}
		for _, s := range *sec.CustomServers {
			srv, err := servers.FromAddress(s.Address, model.SourceCustom)
			if err != nil {
				sp.Errors = append(sp.Errors, fmt.Sprintf("%s: %v", s.Address, err))
				continue
			}
			if s.Name != "" {
				srv.Name = s.Name
			}
			p.custom = append(p.custom, srv)
		}
		sp.New, sp.Replaced = diff(p.custom, cur.Custom, func(s model.Server) string { return s.Address })
		p.add(sp)
	}
	if sec.DPIBlacklist != nil {
		p.blacklist = *sec.DPIBlacklist
		sp := SectionPreview{Name: SecBlacklist}
		if len(lines(p.blacklist)) == 0 {
			sp.Errors = append(sp.Errors, "the DPI blacklist is empty")
		}
		sp.New, sp.Replaced = diff(lines(p.blacklist), lines(cur.Blacklist), func(s string) string { return s })
		p.add(sp)
	}
	if sec.DPIAutoHostlist != nil {
		p.autoHost = *sec.DPIAutoHostlist
		sp := SectionPreview{Name: SecAutoHostlist}
		sp.New, sp.Replaced = diff(p.autoHost, cur.AutoHostlist, func(s string) string { return s })
		p.add(sp)
	}
	return p, nil
}

func (p *Plan) add(sp SectionPreview) {
	if sp.Errors == nil {
		sp.Errors = []string{}
	}
	p.present[sp.Name] = true
	p.errs[sp.Name] = len(sp.Errors) > 0
	p.Preview.Sections = append(p.Preview.Sections, sp)
}

func (p *Plan) warn(code, detail string) {
	p.Preview.Warnings = append(p.Preview.Warnings, Warning{Code: code, Detail: detail})
}

// sanitiseSettings turns off what could expose this machine, and keeps the
// values that belong to this machine.
func (p *Plan) sanitiseSettings(s, cur store.Settings) store.Settings {
	for _, f := range []struct {
		name string
		v    *bool
	}{
		{"fakeSni.enabled", &s.FakeSNI.Enabled},
		{"dnsServer.enabled", &s.DNSServer.Enabled},
		{"dnsServer.shareLan", &s.DNSServer.ShareLAN},
		{"proxy.shareLan", &s.Proxy.ShareLAN},
		{"startWithWindows", &s.StartWithWindows},
	} {
		if *f.v {
			*f.v = false
			p.warn("flag_off", f.name)
		}
	}
	s.FakeSNI.AckVersion = cur.FakeSNI.AckVersion
	s.Terms = cur.Terms
	s.Adapters, s.AdapterGUIDs = cur.Adapters, cur.AdapterGUIDs
	s.FullWindow = cur.FullWindow
	s.DNSServer.IOSSSID = cur.DNSServer.IOSSSID
	for i, u := range s.Proxy.Upstreams {
		if u.PassEnc != "" {
			continue
		}
		if j := slices.IndexFunc(cur.Proxy.Upstreams, func(c store.UpstreamProxy) bool { return c.ID == u.ID }); j >= 0 {
			s.Proxy.Upstreams[i].PassEnc = cur.Proxy.Upstreams[j].PassEnc
		}
	}
	return s
}

func (p *Plan) parseRules(rf *store.RulesFile, cur Data, target store.Settings, v Validators) {
	sp := SectionPreview{Name: SecRules}
	var ids []string
	for _, u := range target.Proxy.Upstreams {
		ids = append(ids, u.ID)
	}
	for _, r := range rf.Rules {
		if err := rules.ValidateRule(r, ids); err != nil {
			sp.Errors = append(sp.Errors, fmt.Sprintf("%s: %v", r.Pattern, err))
			continue
		}
		p.rules = append(p.rules, r)
		if r.SNI != "" || r.Connect != "" {
			p.Preview.SNIRules = append(p.Preview.SNIRules, r)
		}
	}
	for _, l := range rf.Lists {
		l = lists.List{ID: l.ID, Name: l.Name, Source: l.Source, URL: l.URL, Format: l.Format, Action: l.Action,
			Enabled: l.Enabled, UpdateHours: l.UpdateHours, TrustedForSNI: l.TrustedForSNI, Signed: l.Signed}
		if !lists.ValidID(l.ID) || slices.ContainsFunc(p.lists, func(o lists.List) bool { return o.ID == l.ID }) {
			sp.Errors = append(sp.Errors, fmt.Sprintf("%s: invalid or duplicate list id %q", l.Name, l.ID))
			continue
		}
		if l.Source != "url" {
			sp.Errors = append(sp.Errors, fmt.Sprintf("%s: only URL lists can be imported", l.Name))
			continue
		}
		if err := v.List(l); err != nil {
			sp.Errors = append(sp.Errors, fmt.Sprintf("%s: %v", l.Name, err))
			continue
		}
		if l.TrustedForSNI && !l.Signed {
			l.TrustedForSNI = false
			p.warn("trusted_reset", l.Name)
		}
		if strings.HasPrefix(strings.ToLower(l.URL), "http://") {
			p.warn("http_list", l.Name)
		}
		p.lists = append(p.lists, l)
	}
	ruleKey := func(r rules.Rule) string { b, _ := json.Marshal(r); return string(b) }
	n1, r1 := diff(p.rules, cur.Rules.Rules, ruleKey)
	n2, r2 := diff(p.lists, cur.Rules.Lists, func(l lists.List) string { return l.URL })
	sp.New, sp.Replaced = n1+n2, r1+r2
	p.add(sp)
}

// diff counts file items missing here (added) and items here missing from
// the file (removed by a replace).
func diff[T any](file, here []T, key func(T) string) (added, removed int) {
	in := func(xs []T, k string) bool { return slices.ContainsFunc(xs, func(x T) bool { return key(x) == k }) }
	for _, x := range file {
		if !in(here, key(x)) {
			added++
		}
	}
	for _, x := range here {
		if !in(file, key(x)) {
			removed++
		}
	}
	return
}

func lines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

// Result is cur with the chosen sections imported, and the names of the
// sections that change, in file order.
func (p *Plan) Result(cur Data, c Choices) (Data, []string, error) {
	out := cur
	var changed []string
	for _, name := range AllSections {
		if !slices.Contains(c.Sections, name) || !p.present[name] {
			continue
		}
		if p.errs[name] {
			return cur, nil, fmt.Errorf("%w: section %s has errors", ErrInvalid, name)
		}
		switch name {
		case SecSettings:
			out.Settings = p.settings
		case SecRules:
			rs := p.rules
			if len(p.Preview.SNIRules) > 0 {
				switch c.SNIRules {
				case "accept":
				case "drop":
					rs = slices.DeleteFunc(slices.Clone(rs), func(r rules.Rule) bool { return r.SNI != "" || r.Connect != "" })
				default:
					return cur, nil, ErrSNIUnconfirmed
				}
			}
			out.Rules = mergeRules(cur.Rules, rs, p.lists, c.Merge)
		case SecCustom:
			out.Custom = mergeBy(cur.Custom, p.custom, c.Merge, func(s model.Server) string { return s.Address })
		case SecBlacklist:
			out.Blacklist = p.blacklist
			if c.Merge {
				out.Blacklist = mergeText(cur.Blacklist, p.blacklist)
			}
		case SecAutoHostlist:
			out.AutoHostlist = mergeBy(cur.AutoHostlist, p.autoHost, c.Merge, func(s string) string { return s })
		}
		changed = append(changed, name)
	}
	if slices.Contains(changed, SecSettings) || slices.Contains(changed, SecRules) {
		if err := checkUpstreamRefs(out); err != nil {
			return cur, nil, err
		}
	}
	if len(out.Rules.Rules) > rules.MaxUserRules {
		return cur, nil, fmt.Errorf("%w: more than %d rules", ErrInvalid, rules.MaxUserRules)
	}
	if out.Settings.DPI.Scope == scopeBlacklist && len(lines(out.Blacklist)) == 0 &&
		(slices.Contains(changed, SecSettings) || slices.Contains(changed, SecBlacklist)) {
		return cur, nil, fmt.Errorf("%w: DPI scope is the blacklist but the blacklist is empty", ErrInvalid)
	}
	return out, changed, nil
}

// checkUpstreamRefs makes sure every upstream= in the final rules and lists
// names a proxy in the final settings.
func checkUpstreamRefs(d Data) error {
	ids := map[string]bool{}
	for _, u := range d.Settings.Proxy.Upstreams {
		ids[u.ID] = true
	}
	for _, r := range d.Rules.Rules {
		if r.Upstream != "" && !ids[r.Upstream] {
			return fmt.Errorf("%w: rule %s uses upstream proxy %q, which this PC does not have", ErrInvalid, r.Pattern, r.Upstream)
		}
	}
	for _, l := range d.Rules.Lists {
		if up, ok := strings.CutPrefix(l.Action, "upstream="); ok && !ids[up] {
			return fmt.Errorf("%w: list %s uses upstream proxy %q, which this PC does not have", ErrInvalid, l.Name, up)
		}
	}
	return nil
}

// freeListID returns id, or id with a numeric suffix when another list
// already uses it.
func freeListID(id string, have []lists.List) string {
	taken := func(x string) bool { return slices.ContainsFunc(have, func(o lists.List) bool { return o.ID == x }) }
	if !taken(id) {
		return id
	}
	for n := 2; ; n++ {
		suf := fmt.Sprintf("-%d", n)
		base := id
		if len(base)+len(suf) > 64 {
			base = base[:64-len(suf)]
		}
		if !taken(base + suf) {
			return base + suf
		}
	}
}

func mergeRules(cur store.RulesFile, rs []rules.Rule, ls []lists.List, merge bool) store.RulesFile {
	out := store.RulesFile{Version: 1}
	if !merge {
		out.Rules = slices.Clone(rs)
		out.Lists = slices.Clone(ls)
	} else {
		out.Rules = slices.Clone(cur.Rules)
		for _, r := range rs {
			if !slices.ContainsFunc(out.Rules, func(o rules.Rule) bool { return o.Pattern == r.Pattern && reflect.DeepEqual(o.Action, r.Action) }) {
				out.Rules = append(out.Rules, r)
			}
		}
		out.Lists = slices.Clone(cur.Lists)
		for _, l := range ls {
			if slices.ContainsFunc(out.Lists, func(o lists.List) bool { return o.URL == l.URL }) {
				continue // already subscribed
			}
			l.ID = freeListID(l.ID, out.Lists)
			out.Lists = append(out.Lists, l)
		}
	}
	if out.Rules == nil {
		out.Rules = []rules.Rule{}
	}
	if out.Lists == nil {
		out.Lists = []lists.List{}
	}
	return out
}

func mergeBy[T any](cur, file []T, merge bool, key func(T) string) []T {
	if !merge {
		return slices.Clone(file)
	}
	out := slices.Clone(cur)
	for _, x := range file {
		if !slices.ContainsFunc(out, func(o T) bool { return key(o) == key(x) }) {
			out = append(out, x)
		}
	}
	return out
}

func mergeText(cur, file string) string {
	have := lines(cur)
	out := strings.TrimRight(cur, "\n")
	for _, l := range lines(file) {
		if !slices.Contains(have, l) {
			if out != "" {
				out += "\n"
			}
			out += l
			have = append(have, l)
		}
	}
	return out + "\n"
}
