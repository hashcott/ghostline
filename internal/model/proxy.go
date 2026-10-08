package model

import "encoding/json"

// ProxySnapshot is the system proxy configuration Ghostline replaced, so it
// can be put back. Exactly one branch is set, named by Backend.
type ProxySnapshot struct {
	Backend string        `json:"backend"` // "windows" | "gnome" | "kde"
	UID     int           `json:"uid,omitempty"`
	Windows *WinINETProxy `json:"windows,omitempty"`
	GNOME   *GNOMEProxy   `json:"gnome,omitempty"`
	KDE     *KDEProxy     `json:"kde,omitempty"`
}

// WinINETProxy is the WinINET per-connection proxy configuration.
type WinINETProxy struct {
	Flags         uint32 `json:"flags"`
	Server        string `json:"server"`
	Bypass        string `json:"bypass"`
	AutoconfigURL string `json:"autoconfigUrl"`
}

// GNOMEProxy holds every org.gnome.system.proxy* key Ghostline may change:
// "<schema> <key>" → the raw value `gsettings get` printed.
type GNOMEProxy struct {
	Values map[string]string `json:"values"`
}

// KDEProxy holds the [Proxy Settings] keys of kioslaverc; Present lists the
// keys that existed, so restoring deletes the ones that did not.
type KDEProxy struct {
	Values  map[string]string `json:"values"`
	Present []string          `json:"present"`
}

// UnmarshalJSON also reads the v4 form, a bare WinINET snapshot (state.json
// files written before the snapshot became backend-neutral).
func (s *ProxySnapshot) UnmarshalJSON(b []byte) error {
	type plain ProxySnapshot
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	if _, ok := keys["backend"]; !ok {
		if _, legacy := keys["flags"]; legacy {
			var w WinINETProxy
			if err := json.Unmarshal(b, &w); err != nil {
				return err
			}
			*s = ProxySnapshot{Backend: "windows", Windows: &w}
			return nil
		}
	}
	return json.Unmarshal(b, (*plain)(s))
}

// MarshalJSON also writes a Windows snapshot's WinINET fields at the top
// level, the form v0.5 reads, so a crash followed by a downgrade still
// restores the right proxy. Readers of the current form ignore them.
func (s ProxySnapshot) MarshalJSON() ([]byte, error) {
	type plain ProxySnapshot
	if s.Backend != "windows" || s.Windows == nil {
		return json.Marshal(plain(s))
	}
	return json.Marshal(struct {
		plain
		WinINETProxy
	}{plain(s), *s.Windows})
}
