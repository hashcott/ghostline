package sysdns

import (
	"errors"
	"fmt"
)

// Unsupported is the API for an OS without support yet: changing DNS
// fails, and there is no cache to flush.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("sysdns: %w", errors.ErrUnsupported)

func (Unsupported) Adapters() ([]Adapter, error)             { return nil, errUnsupported }
func (Unsupported) GetDNS(string, bool) ([]string, error)    { return nil, errUnsupported }
func (Unsupported) SetDNS(string, bool, []string) error      { return errUnsupported }
func (Unsupported) NetshSetDNS(uint32, bool, []string) error { return errUnsupported }
func (Unsupported) Flush() error                             { return nil }
