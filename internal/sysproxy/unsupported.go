package sysproxy

import (
	"errors"
	"fmt"
)

// Unsupported is the Backend for an OS without system proxy support yet.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("sysproxy: %w", errors.ErrUnsupported)

func (Unsupported) Snapshot(string) (Snapshot, error)        { return Snapshot{}, errUnsupported }
func (Unsupported) Existing(Snapshot) (string, string, bool) { return "", "", false }
func (Unsupported) Apply(string) error                       { return errUnsupported }
func (Unsupported) IsOurs(string) (bool, error)              { return false, errUnsupported }
func (Unsupported) RestoreIfOurs(string, Snapshot) (bool, error) {
	return false, errUnsupported
}
func (Unsupported) Watch(func()) (func(), error) { return nil, errUnsupported }
func (Unsupported) Info() Info                   { return Info{} }
