package firewall

import (
	"errors"
	"fmt"
)

// Unsupported is the Manager for an OS without firewall support yet: adding
// fails, removing has nothing to undo.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("firewall: %w", errors.ErrUnsupported)

func (Unsupported) Add(int) error                  { return errUnsupported }
func (Unsupported) Delete() error                  { return nil }
func (Unsupported) AddNamed(Rule) error            { return errUnsupported }
func (Unsupported) DeleteNamed(string) error       { return nil }
func (Unsupported) IsPublicNetwork() (bool, error) { return false, errUnsupported }
