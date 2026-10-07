package sysproxy

import (
	"errors"
	"fmt"

	"github.com/hashcott/ghostline/internal/store"
)

// Unsupported is the API for an OS without system proxy support yet.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("sysproxy: %w", errors.ErrUnsupported)

func (Unsupported) Query() (store.SysProxySnapshot, error) {
	return store.SysProxySnapshot{}, errUnsupported
}
func (Unsupported) Set(store.SysProxySnapshot) error { return errUnsupported }
