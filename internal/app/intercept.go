package app

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// dnsInterceptor is a program known to take DNS packets bound for port 53
// when one of its features is on (so its process alone proves nothing).
type dnsInterceptor struct {
	id   string   // i18n key of the hint telling which feature to turn off
	name string   // shown to the user
	exes []string // lower-case image names; a trailing "*" matches a prefix
}

// dnsInterceptors are matched in order. Add one only with evidence that it
// redirects loopback DNS.
var dnsInterceptors = []dnsInterceptor{
	// DNS protection filters every port 53 packet (seen on Windows 10).
	{"adguard", "AdGuard", []string{"adguardsvc.exe", "adguard.exe"}},
	// Real Site / Fake Website Shield send DNS to the vendor's resolver.
	{"avast", "Avast", []string{"avastsvc.exe", "avastui.exe"}},
	{"avg", "AVG", []string{"avgsvc.exe", "avgui.exe"}},
	// Intercepts DNS at the system level by design.
	{"yogadns", "YogaDNS", []string{"yogadns.exe"}},
	// Reroutes DNS that bypasses its resolver; v1 names its binary
	// portmaster-core_v1-x-y.exe.
	{"portmaster", "Portmaster", []string{"portmaster-core*"}},
}

func (d dnsInterceptor) matches(exe string) bool {
	for _, e := range d.exes {
		if p, ok := strings.CutSuffix(e, "*"); ok && strings.HasPrefix(exe, p) || e == exe {
			return true
		}
	}
	return false
}

// findDNSInterceptors lists the known interceptors among the running
// process names, each once, in list order.
func findDNSInterceptors(names []string) []dnsInterceptor {
	var found []dnsInterceptor
	for _, d := range dnsInterceptors {
		for _, n := range names {
			if d.matches(strings.ToLower(filepath.Base(strings.ReplaceAll(n, `\`, "/")))) {
				found = append(found, d)
				break
			}
		}
	}
	return found
}

// selfTestErr explains a failed engine self test, once the engine has
// released port 53. When a datagram sent to 127.0.0.1:53 does not arrive
// either, another program takes loopback DNS and the engine is not at
// fault; known programs are named. Params: name ("" when none is known,
// else the names joined by ", ") and hint (the first one's id).
func (o *Orchestrator) selfTestErr(err error) *AppError {
	lerr := o.d.System.LoopbackUDP(53)
	if lerr == nil {
		return appErr(CodeEngineSelfTest, err)
	}
	names, perr := o.d.System.ProcessNames()
	if perr != nil {
		slog.Warn("app: listing processes failed", "err", perr)
	}
	var shown []string
	hint := ""
	for i, d := range findDNSInterceptors(names) {
		if i == 0 {
			hint = d.id
		}
		shown = append(shown, d.name)
	}
	return appErr(CodeDNSIntercepted, fmt.Errorf("%w; loopback probe: %v", err, lerr),
		"name", strings.Join(shown, ", "), "hint", hint)
}
