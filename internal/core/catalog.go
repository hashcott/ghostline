package core

import (
	"crypto/ed25519"
	"encoding/hex"
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
	builtin, _ := servers.ParseList(lists.BuiltinJSON)
	var remote servers.List
	if raw, err := os.ReadFile(c.paths.ServersRemote); err == nil {
		if sig, err := os.ReadFile(c.paths.ServersRemoteSig); err == nil && servers.VerifySigned(raw, sig, serverListKey()) == nil {
			remote, _ = servers.ParseList(raw)
		}
	}
	var dnscrypt []model.Server
	if md, err := os.ReadFile(c.paths.ServersDNSCrypt); err == nil {
		if sig, err := os.ReadFile(c.paths.ServersDNSCryptSig); err == nil && servers.VerifyMinisign(md, sig, brand.DNSCryptMinisignKey) == nil {
			dnscrypt, _ = servers.ParseDNSCryptMarkdown(md)
		}
	}
	if len(dnscrypt) == 0 && servers.VerifyMinisign(lists.DNSCryptMD, lists.DNSCryptSig, brand.DNSCryptMinisignKey) == nil {
		// Not downloaded yet (or the download is bad): the built-in copy.
		dnscrypt, _ = servers.ParseDNSCryptMarkdown(lists.DNSCryptMD)
	}
	custom, _ := c.loadCustom()
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
