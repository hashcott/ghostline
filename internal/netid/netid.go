// Package netid tells networks apart (gateway address and hardware
// address, Wi-Fi name) and lists this machine's LAN addresses.
package netid

import (
	"errors"
	"fmt"

	"github.com/hashcott/ghostline/internal/scanner"
)

// LiveAdapter is an up adapter's current DNS servers and default gateway.
type LiveAdapter struct {
	DNS     []string
	Gateway string
}

// Source reads the identity of the network this machine is on.
type Source interface {
	// NetworkKey identifies the current network by its default gateway's
	// address and hardware address (scanner.NetworkKey).
	NetworkKey() string
	CurrentSSID() (string, error)
	// WifiNames lists the Wi-Fi networks this machine has a profile for.
	WifiNames() ([]string, error)
	// LiveAdapters lists every up adapter that has a default gateway.
	LiveAdapters() []LiveAdapter
}

// Unsupported is the Source for an OS without support yet: every network
// looks the same, and there is no Wi-Fi information.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("netid: %w", errors.ErrUnsupported)

func (Unsupported) NetworkKey() string           { return scanner.NetworkKey("none", "none") }
func (Unsupported) CurrentSSID() (string, error) { return "", errUnsupported }
func (Unsupported) WifiNames() ([]string, error) { return nil, errUnsupported }
func (Unsupported) LiveAdapters() []LiveAdapter  { return nil }
