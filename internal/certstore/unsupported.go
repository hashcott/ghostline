package certstore

import (
	"errors"
	"fmt"
)

// Unsupported is the Store for an OS without support yet: installing
// fails; it holds no certificates, so removing has nothing to undo.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("certstore: %w", errors.ErrUnsupported)

func (Unsupported) Install([]byte) error        { return errUnsupported }
func (Unsupported) Remove(string) error         { return nil }
func (Unsupported) List(string) ([]Cert, error) { return nil, nil }
