package desktop

import (
	"bufio"
	"context"
	"net"
	"os/exec"
	"strings"
	"sync"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/sysproxy"
)

// gnomeKeys are every org.gnome.system.proxy* key Ghostline changes or
// whose value decides what a proxy-aware app does.
var gnomeKeys = [][2]string{
	{"org.gnome.system.proxy", "mode"},
	{"org.gnome.system.proxy", "autoconfig-url"},
	{"org.gnome.system.proxy", "ignore-hosts"},
	{"org.gnome.system.proxy", "use-same-proxy"},
	{"org.gnome.system.proxy.http", "host"},
	{"org.gnome.system.proxy.http", "port"},
	{"org.gnome.system.proxy.http", "enabled"},
	{"org.gnome.system.proxy.https", "host"},
	{"org.gnome.system.proxy.https", "port"},
	{"org.gnome.system.proxy.socks", "host"},
	{"org.gnome.system.proxy.socks", "port"},
	{"org.gnome.system.proxy.ftp", "host"},
	{"org.gnome.system.proxy.ftp", "port"},
}

// gnome sets GNOME's (and Cinnamon's, Budgie's) proxy through gsettings.
type gnome struct{ run runner }

func (g gnome) get(schema, key string) (string, error) {
	out, err := g.run("gsettings", "get", schema, key)
	return strings.TrimSpace(string(out)), err
}

func (g gnome) set(schema, key, v string) error {
	_, err := g.run("gsettings", "set", schema, key, v)
	return err
}

func quoted(s string) string { return "'" + s + "'" }

func (g gnome) isOurs(addr string) (bool, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false, err
	}
	mode, err := g.get("org.gnome.system.proxy", "mode")
	if err != nil {
		return false, err
	}
	h, err := g.get("org.gnome.system.proxy.http", "host")
	if err != nil {
		return false, err
	}
	p, err := g.get("org.gnome.system.proxy.http", "port")
	if err != nil {
		return false, err
	}
	return mode == "'manual'" && h == quoted(host) && p == port, nil
}

// snapshot records the keys the user set; Ghostline's own leftover means
// "defaults".
func (g gnome) snapshot(ours string) (model.ProxySnapshot, error) {
	s := model.ProxySnapshot{Backend: "gnome", GNOME: &model.GNOMEProxy{Values: map[string]string{}}}
	if mine, err := g.isOurs(ours); err == nil && mine {
		s.GNOME.Values = nil
		return s, nil
	}
	// dconf answers only for keys the user set; the rest are defaults,
	// which restore resets rather than pins to today's default value.
	for _, k := range gnomeKeys {
		out, err := g.run("dconf", "read", dconfPath(k[0], k[1]))
		if err != nil {
			return model.ProxySnapshot{}, err
		}
		if v := strings.TrimSpace(string(out)); v != "" {
			s.GNOME.Values[k[0]+" "+k[1]] = v
		}
	}
	return s, nil
}

// dconfPath is where dconf keeps a GNOME schema key:
// org.gnome.system.proxy.http host → /system/proxy/http/host.
func dconfPath(schema, key string) string {
	return "/" + strings.ReplaceAll(strings.TrimPrefix(schema, "org.gnome."), ".", "/") + "/" + key
}

func (g gnome) apply(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	for _, kv := range [][3]string{
		{"org.gnome.system.proxy.http", "host", quoted(host)},
		{"org.gnome.system.proxy.http", "port", port},
		{"org.gnome.system.proxy.https", "host", quoted(host)},
		{"org.gnome.system.proxy.https", "port", port},
		{"org.gnome.system.proxy", "ignore-hosts", sysproxy.BypassGNOME()},
		{"org.gnome.system.proxy", "mode", "'manual'"}, // last: hosts first
	} {
		if err := g.set(kv[0], kv[1], kv[2]); err != nil {
			return err
		}
	}
	return nil
}

// restore sets each recorded key back and resets the ones not recorded.
func (g gnome) restore(s model.ProxySnapshot) error {
	vals := map[string]string{}
	if s.GNOME != nil {
		vals = s.GNOME.Values
	}
	for _, k := range gnomeKeys {
		v, ok := vals[k[0]+" "+k[1]]
		var err error
		if ok {
			err = g.set(k[0], k[1], v)
		} else {
			_, err = g.run("gsettings", "reset", k[0], k[1])
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// watch emits on every gsettings change of the proxy and HTTP schemas.
func (gnome) watch(ctx context.Context, emit func()) error {
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, schema := range []string{"org.gnome.system.proxy", "org.gnome.system.proxy.http"} {
		cmd := exec.CommandContext(ctx, "gsettings", "monitor", schema)
		out, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(out)
			for sc.Scan() {
				emit()
			}
			errs <- cmd.Wait()
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return <-errs
}
