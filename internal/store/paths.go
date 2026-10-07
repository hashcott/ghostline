// Package store owns Ghostline's on-disk data: paths, settings and state.
package store

import (
	"os"
	"path/filepath"

	"github.com/hashcott/ghostline/internal/brand"
)

// Paths lists every file Ghostline keeps in its data directory.
type Paths struct {
	DataDir            string
	Settings           string
	State              string
	Meta               string
	ScanCache          string
	CFScanCache        string
	ServersRemote      string
	ServersRemoteSig   string
	ServersDNSCrypt    string
	ServersDNSCryptSig string
	ServersCustom      string
	DPIBlacklist       string
	DPIStrategies      string
	DPIStrategiesSig   string
	DPIAutoHostlist    string
	LogDir             string
	BinDir             string
	Rules              string
	FragCache          string
	ListsDir           string
	// LAN CA files live in MachineDir (see WithMachineDir); until it is
	// set they default to DataDir, which tests use.
	MachineDir string
	LANCACert  string
	LANCAKey   string
	Portable   bool
}

// ResolvePaths picks the data directory: <exe dir>\data when a "portable"
// marker sits next to the executable, otherwise %APPDATA%\Ghostline.
func ResolvePaths(exePath, appData string) Paths {
	exeDir := filepath.Dir(exePath)
	dataDir, portable := filepath.Join(appData, brand.AppName), false
	if _, err := os.Stat(filepath.Join(exeDir, "portable")); err == nil {
		dataDir, portable = filepath.Join(exeDir, "data"), true
	}
	p := PathsIn(dataDir, filepath.Join(dataDir, "logs"))
	p.MachineDir = "" // set by WithMachineDir
	p.Portable = portable
	return p
}

// PathsIn lays every file out under dataDir, with the logs in logDir. The
// LAN CA stays in dataDir (MachineDir): the Linux daemon's data directory
// is root-only.
func PathsIn(dataDir, logDir string) Paths {
	p := Paths{DataDir: dataDir, MachineDir: dataDir}
	j := func(name string) string { return filepath.Join(p.DataDir, name) }
	p.Settings = j("settings.json")
	p.State = j("state.json")
	p.LANCACert = j("lan-ca.crt")
	p.LANCAKey = j("lan-ca.key")
	p.Meta = j("meta.json")
	p.ScanCache = j("scan-cache.json")
	p.CFScanCache = j("cfscan-cache.json")
	p.ServersRemote = j("servers-remote.json")
	p.ServersRemoteSig = j("servers-remote.json.sig")
	p.ServersDNSCrypt = j("servers-dnscrypt.md")
	p.ServersDNSCryptSig = j("servers-dnscrypt.md.minisig")
	p.ServersCustom = j("servers-custom.json")
	p.DPIBlacklist = j("dpi-blacklist.txt")
	p.DPIStrategies = j("dpi-strategies.json")
	p.DPIStrategiesSig = j("dpi-strategies.json.sig")
	p.DPIAutoHostlist = j("dpi-autohostlist.txt")
	p.LogDir = logDir
	p.BinDir = j("bin")
	p.Rules = j("rules.json")
	p.FragCache = j("frag-cache.json")
	p.ListsDir = j("lists")
	return p
}

// WithMachineDir moves the LAN CA files to dir, a machine-wide directory
// only SYSTEM and Administrators can open (%ProgramData%\Ghostline): the
// user-writable data directory must never hold a key that ends up in the
// Root store.
func WithMachineDir(p Paths, dir string) Paths {
	p.MachineDir = dir
	p.LANCACert = filepath.Join(dir, "lan-ca.crt")
	p.LANCAKey = filepath.Join(dir, "lan-ca.key")
	return p
}
