package dpi

import (
	"errors"
	"fmt"
)

// UnsupportedRunner is the Runner for an OS without a DPI engine yet.
type UnsupportedRunner struct{}

func (UnsupportedRunner) Start(string, []string, string) (Process, error) {
	return nil, fmt.Errorf("dpi: %w", errors.ErrUnsupported)
}
