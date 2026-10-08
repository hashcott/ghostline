package core

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"sync"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/lists"
)

// catalog merges the built-in, signed remote, DNSCrypt and custom lists.
// Unsigned or tampered files on disk are ignored.
type catalog struct {
	paths store.Paths
	mu    sync.Mutex
	all   []model.Server
}

func newCatalog(p store.Paths) *catalog {
	c := &catalog{paths: p}
	c.reload()
	return c
}

func serverListKey() ed25519.PublicKey {
	b, _ := hex.DecodeString(brand.ServerListPublicKeyHex)
	return ed25519.PublicKey(b)
}

func (c *catalog) reload() {
	builtin, err := servers.ParseList(lists.BuiltinJSON)
	if err != nil {
		slog.Error("catalog: parsing the built-in server list failed", "err", err)
	}
	var remote servers.List
	if raw, err := os.ReadFile(c.paths.ServersRemote); err == nil {
		sig, err := os.ReadFile(c.paths.ServersRemoteSig)
		if err == nil {
			err = servers.VerifySigned(raw, sig, serverListKey())
		}
		if err != nil {
			slog.Warn("catalog: downloaded server list ignored (signature)", "file", c.paths.ServersRemote, "err", err)
		} else if remote, err = servers.ParseList(raw); err != nil {
			slog.Warn("catalog: parsing the downloaded server list failed", "file", c.paths.ServersRemote, "err", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("catalog: reading the downloaded server list failed", "err", err)
	}
	var dnscrypt []model.Server
	if md, err := os.ReadFile(c.paths.ServersDNSCrypt); err == nil {
		sig, err := os.ReadFile(c.paths.ServersDNSCryptSig)
		if err == nil {
			err = servers.VerifyMinisign(md, sig, brand.DNSCryptMinisignKey)
		}
		if err != nil {
			slog.Warn("catalog: downloaded DNSCrypt list ignored (signature)", "file", c.paths.ServersDNSCrypt, "err", err)
		} else if dnscrypt, err = servers.ParseDNSCryptMarkdown(md); err != nil {
			slog.Warn("catalog: parsing the downloaded DNSCrypt list failed", "file", c.paths.ServersDNSCrypt, "err", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("catalog: reading the downloaded DNSCrypt list failed", "err", err)
	}
	if len(dnscrypt) == 0 {
		// Not downloaded yet (or the download is bad): the built-in copy.
		if err := servers.VerifyMinisign(lists.DNSCryptMD, lists.DNSCryptSig, brand.DNSCryptMinisignKey); err != nil {
			slog.Error("catalog: built-in DNSCrypt list signature check failed", "err", err)
		} else if dnscrypt, err = servers.ParseDNSCryptMarkdown(lists.DNSCryptMD); err != nil {
			slog.Error("catalog: parsing the built-in DNSCrypt list failed", "err", err)
		}
	}
	custom, err := c.loadCustom()
	if err != nil {
		slog.Warn("catalog: reading custom servers failed; ignoring them", "file", c.paths.ServersCustom, "err", err)
	}
	all := servers.Merge(builtin, remote, dnscrypt, custom)
	c.mu.Lock()
	c.all = all
	c.mu.Unlock()
}

func (c *catalog) get() []model.Server {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]model.Server(nil), c.all...)
}

func (c *catalog) loadCustom() ([]model.Server, error) {
	var out []model.Server
	if err := store.ReadJSON(c.paths.ServersCustom, &out); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return out, nil
}

func (c *catalog) saveCustom(s []model.Server) error {
	if s == nil {
		s = []model.Server{}
	}
	if err := store.WriteJSONAtomic(c.paths.ServersCustom, s); err != nil {
		return err
	}
	c.reload()
	return nil
}
