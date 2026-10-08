package app

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"

	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/sysdns"
)

// Why the leak check failed, as the "reason" param of VERIFY_LEAK.
const (
	verifyTimeout     = "timeout"      // no answer at all
	verifyNXDomain    = "nxdomain"     // another resolver said the name does not exist
	verifyLookupError = "lookup_error" // any other resolver error
	verifyWrongAnswer = "wrong_answer" // an answer, but not Ghostline's
	verifyNotSeen     = "not_seen"     // Ghostline's answer, yet the engine never got the query
)

// verifyLeakError explains a failed leak check. It runs while the adapters
// still point at loopback, so the report shows what Windows had in hand:
// adapters Ghostline does not manage that carry their own DNS ("others"),
// and managed adapters whose DNS something changed back ("drifted").
// Params carry adapter names and server IPs only, never queried names.
func (o *Orchestrator) verifyLeakError(lookupErr error, ips []netip.Addr, saw bool, snaps []model.AdapterSnapshot, v6 bool) *AppError {
	kv := []any{"saw", saw}
	switch {
	case lookupErr != nil:
		reason, detail := verifyLookupError, lookupErr.Error()
		var de *net.DNSError
		if errors.As(lookupErr, &de) {
			detail = de.Err // without the queried name
			switch {
			case de.IsTimeout:
				reason = verifyTimeout
			case de.IsNotFound:
				reason = verifyNXDomain
			}
		}
		kv = append(kv, "reason", reason, "detail", detail)
	case !slices.Contains(ips, engine.VerifyAnswer):
		kv = append(kv, "reason", verifyWrongAnswer, "answer", joinAddrs(ips))
	default:
		kv = append(kv, "reason", verifyNotSeen)
	}

	// Only a backend with adapters (Windows) can say which ones bypass
	// Ghostline; elsewhere the reason alone is reported.
	rep, ok := o.d.DNS.(DNSReporter)
	if !ok {
		return appErr(CodeVerifyLeak, lookupErr, kv...)
	}
	report, err := rep.Report()
	if err != nil {
		kv = append(kv, "reportError", err.Error())
		return appErr(CodeVerifyLeak, lookupErr, kv...)
	}
	managed := map[string]bool{}
	for _, s := range snaps {
		managed[s.GUID] = true
	}
	var others, drifted, names []string
	for _, a := range report {
		if managed[a.GUID] {
			if !isLoopbackDNS(a.IPv4, "127.0.0.1") || (v6 && a.HasIPv6 && !isLoopbackDNS(a.IPv6, "::1")) {
				drifted = append(drifted, describeAdapter(a))
				names = append(names, a.Alias)
			}
			continue
		}
		if len(a.IPv4)+len(a.IPv6) > 0 && !isLoopbackDNS(a.IPv4, "127.0.0.1") {
			others = append(others, describeAdapter(a))
			names = append(names, a.Alias)
		}
	}
	if len(others) > 0 {
		kv = append(kv, "others", strings.Join(others, "; "))
	}
	if len(drifted) > 0 {
		kv = append(kv, "drifted", strings.Join(drifted, "; "))
	}
	if len(names) > 0 {
		kv = append(kv, "adapters", strings.Join(names, ", "))
	}
	return appErr(CodeVerifyLeak, lookupErr, kv...)
}

// isLoopbackDNS reports whether servers is exactly Ghostline's loopback.
func isLoopbackDNS(servers []string, lo string) bool {
	return slices.Equal(servers, []string{lo})
}

func describeAdapter(a sysdns.AdapterDNS) string {
	s := fmt.Sprintf("%s [%s]", a.Alias, ifTypeName(a.IfType))
	if len(a.IPv4) > 0 {
		s += " v4=" + strings.Join(a.IPv4, ",")
	}
	if len(a.IPv6) > 0 {
		s += " v6=" + strings.Join(a.IPv6, ",")
	}
	return s
}

// ifTypeName names the IANA interface types seen on Windows PCs.
func ifTypeName(t uint32) string {
	switch t {
	case 6:
		return "ethernet"
	case 71:
		return "wifi"
	case 23:
		return "ppp"
	case 53:
		return "virtual"
	case 131:
		return "tunnel"
	case 243, 244:
		return "wwan"
	}
	return fmt.Sprintf("type %d", t)
}

func joinAddrs(ips []netip.Addr) string {
	out := make([]string, len(ips))
	for i, ip := range ips {
		out[i] = ip.String()
	}
	return strings.Join(out, ",")
}
