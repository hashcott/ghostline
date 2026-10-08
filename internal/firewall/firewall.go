// Package firewall manages the inbound rules Ghostline needs to share the
// proxy and the DNS server on the LAN. Each OS has its own Manager.
package firewall

// Rule names are fixed so cleanup can always find them: state.json files
// written by earlier versions name these exact strings.
const (
	ProxyRule  = "Ghostline Proxy" // the proxy's LAN-sharing rule
	RuleDNSTCP = "Ghostline DNS (TCP)"
	RuleDNSUDP = "Ghostline DNS (UDP)"
	RuleSetup  = "Ghostline Setup"
	// RuleBlockPublic blocks every inbound connection to Ghostline on
	// Public networks. Block wins over Allow, so the Allow rules Windows
	// creates when someone answers its firewall prompt cannot open the
	// proxy or DNS server on public Wi-Fi.
	RuleBlockPublic = "Ghostline Block Public"
)

// BlockPublicRule is the Public-profile block rule.
var BlockPublicRule = Rule{Name: RuleBlockPublic, Block: true}

// AllRuleNames lists every inbound rule Ghostline may create, for cleanup.
var AllRuleNames = []string{ProxyRule, RuleDNSTCP, RuleDNSUDP, RuleSetup, RuleBlockPublic}

// Rule is a named inbound rule for some local ports.
type Rule struct {
	Name     string
	Protocol string // TCP | UDP
	Ports    []int
	// Block makes a Public-profile block rule for the exe (all protocols
	// and ports); Protocol and Ports are ignored.
	Block bool
}

// Manager creates and removes Ghostline's inbound rules. Removing a rule
// that does not exist is not an error.
type Manager interface {
	Add(port int) error // the proxy's LAN-sharing rule
	Delete() error
	AddNamed(r Rule) error
	DeleteNamed(name string) error
	// IsPublicNetwork reports whether a connected network is treated as
	// public (LAN devices cannot reach the proxy then).
	IsPublicNetwork() (bool, error)
}
