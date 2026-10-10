// Package brand holds every name, URL and key that identifies Ghostline.
// Renaming the app means editing this file (and frontend/src/brand.ts).
package brand

const (
	AppName          = "Ghostline"
	AppID            = "ghostline"
	RepoOwner        = "hashcott"
	RepoName         = "ghostline"
	RepoURL          = "https://github.com/" + RepoOwner + "/" + RepoName
	Author           = "Harry Nguyen"
	TaskAutostart    = "Ghostline"
	TaskRecovery     = "Ghostline Recovery"
	TaskGuard        = "Ghostline Network Guard" // guard.ps1, run as SYSTEM
	StateMutex       = `Local\Ghostline-State`
	SingleInstanceID = "io.github.hashcott.ghostline"

	ServerListURL    = "https://raw.githubusercontent.com/hashcott/ghostline/main/lists/servers.json"
	ServerListSigURL = ServerListURL + ".sig"
	// The zapret2 strategy list is signed with the server-list key.
	StrategyListURL    = "https://raw.githubusercontent.com/hashcott/ghostline/main/lists/strategies.json"
	StrategyListSigURL = StrategyListURL + ".sig"
	// ReleasesAPI lists releases (stable and pre-releases); /tags/<tag>
	// reads one.
	ReleasesAPI = "https://api.github.com/repos/hashcott/ghostline/releases"
	// InstallerAsset is the Windows installer's file name in every release.
	InstallerAsset = "ghostline-amd64-installer.exe"

	DNSCryptMinisignKey = "RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO3"
)

// DNSCryptListURLs are tried in order to fetch the public resolvers list.
var DNSCryptListURLs = []string{
	"https://download.dnscrypt.info/resolvers-list/v3/public-resolvers.md",
	"https://raw.githubusercontent.com/DNSCrypt/dnscrypt-resolvers/master/v3/public-resolvers.md",
}

// Version is overridden at build time with -ldflags -X.
var Version = "dev"

// ServerListPublicKeyHex is the ed25519 public key that signs lists/servers.json.
var ServerListPublicKeyHex = "e32c272e2a6ab33facb7e2d58c948c37a0c394724c63fa96cc9d870044bdd45c"
