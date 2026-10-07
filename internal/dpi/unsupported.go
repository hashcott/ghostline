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

// NoServices is Services for an OS where engines need no driver service:
// there is never one to find, stop or delete.
type NoServices struct{}

func (NoServices) Find(string) ([]string, error) { return nil, nil }
func (NoServices) Running(string) (bool, error)  { return false, nil }
func (NoServices) Stop(string) error             { return nil }
func (NoServices) Delete(string) error           { return nil }
