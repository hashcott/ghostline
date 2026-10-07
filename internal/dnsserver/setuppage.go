package dnsserver

import (
	"context"
	"embed"
	"html/template"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/netid"
)

// SetupFiles are what the phone setup page offers. All of it is public.
type SetupFiles struct {
	CRT         []byte // LAN CA, DER
	Fingerprint string // SHA-256 of the LAN CA, shown for comparison
	// MobileConfig builds the iOS profile for a home Wi-Fi name; nil when
	// no profile can be offered.
	MobileConfig func(ssid string) ([]byte, error)
	// SSID is the home Wi-Fi name saved in Ghostline, the default for the
	// phone's form (the phone user may enter another one).
	SSID string
}

// maxSSID is the longest Wi-Fi name (802.11: 32 bytes).
const maxSSID = 32

//go:embed setuppage_vi.html setuppage_en.html
var pages embed.FS

var tmpl = template.Must(template.ParseFS(pages, "setuppage_vi.html", "setuppage_en.html"))

// SetupPage is a small HTTP server that lives for a limited time.
type SetupPage struct {
	files SetupFiles
	now   func() time.Time
	// OnStop runs once the page has stopped (expiry or Stop).
	OnStop func()

	mu      sync.Mutex
	srvs    []*http.Server
	timer   *time.Timer
	closeAt time.Time
}

// NewSetupPage creates a stopped page.
func NewSetupPage(files SetupFiles, now func() time.Time) *SetupPage {
	return &SetupPage{files: files, now: now}
}

// Handler serves the page and the files to private/local clients only.
func (p *SetupPage) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ghostline-lan-ca.crt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", `attachment; filename="ghostline-lan-ca.crt"`)
		_, _ = w.Write(p.files.CRT)
	})
	mux.HandleFunc("/ghostline.mobileconfig", func(w http.ResponseWriter, r *http.Request) {
		if p.files.MobileConfig == nil {
			http.NotFound(w, r)
			return
		}
		ssid := strings.TrimSpace(r.URL.Query().Get("ssid"))
		if ssid == "" {
			ssid = p.files.SSID
		}
		if ssid == "" || len(ssid) > maxSSID {
			http.Error(w, "enter your home Wi-Fi name (up to 32 bytes)", http.StatusBadRequest)
			return
		}
		b, err := p.files.MobileConfig(ssid)
		if err != nil {
			http.Error(w, "could not build the profile", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-apple-aspen-config")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		name := "setuppage_en.html"
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Accept-Language")), "vi") {
			name = "setuppage_vi.html"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.ExecuteTemplate(w, name, map[string]any{
			"Fingerprint": p.files.Fingerprint, "HasProfile": p.files.MobileConfig != nil, "SSID": p.files.SSID,
		})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ap, err := netip.ParseAddrPort(r.RemoteAddr)
		if err != nil || !netid.IsPrivateOrLocal(ap.Addr()) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

// Start listens on every address that binds and stops after life. It
// fails only when no address binds.
func (p *SetupPage) Start(addrs []netip.AddrPort, life time.Duration) error {
	_ = p.Stop()
	h := p.Handler()
	var srvs []*http.Server
	var firstErr error
	for _, a := range addrs {
		ln, err := net.Listen("tcp", a.String())
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = srv.Serve(ln) }()
		srvs = append(srvs, srv)
	}
	if len(srvs) == 0 {
		return firstErr
	}
	p.mu.Lock()
	p.srvs = srvs
	p.closeAt = p.now().Add(life)
	p.timer = time.AfterFunc(life, func() { _ = p.Stop() })
	p.mu.Unlock()
	return nil
}

// Stop closes the page now. Stopping a stopped page does nothing.
func (p *SetupPage) Stop() error {
	p.mu.Lock()
	srvs := p.srvs
	p.srvs = nil
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.closeAt = time.Time{}
	onStop := p.OnStop
	p.mu.Unlock()
	if srvs == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, s := range srvs {
		_ = s.Shutdown(ctx)
	}
	if onStop != nil {
		onStop()
	}
	return nil
}

// Running reports whether the page is open.
func (p *SetupPage) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.srvs != nil
}

// Remaining is the time until the page closes, 0 when stopped.
func (p *SetupPage) Remaining() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.srvs == nil {
		return 0
	}
	return max(p.closeAt.Sub(p.now()), 0)
}
