package sysdns

import "github.com/hashcott/ghostline/internal/netwatch"

// resolvedDropInDir holds systemd-resolved's runtime drop-ins (/run: gone
// at reboot).
const resolvedDropInDir = "/run/systemd/resolved.conf.d"

// detect picks the first usable backend: NetworkManager (running and in
// charge of DNS), systemd-resolved (running), then fallback (resolv.conf).
// nm or resolved may be nil when D-Bus is unavailable.
func detect(nm nmAPI, resolved resolvedAPI, units unitAPI, fallback Backend, flush func() error, watch netwatch.WatchFunc) Backend {
	resolvedUp := resolved != nil && resolved.Running()
	if nm != nil && nm.Running() {
		if mode, err := nm.DNSMode(); err == nil && mode != "none" {
			chain := "NetworkManager"
			if resolvedUp {
				chain += " → systemd-resolved"
			}
			return newNM(nm, flush, watch, chain)
		}
	}
	if resolvedUp {
		return newResolved(resolved, units, resolvedDropInDir, watch)
	}
	return fallback
}
