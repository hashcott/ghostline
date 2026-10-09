// Package procs answers questions about processes and services: who holds
// a port, whether a process is still the one we started, and stopping a
// service that blocks Ghostline.
package procs

import (
	"errors"
	"fmt"
	"time"
)

// PortOwner is a process holding a local port.
type PortOwner struct {
	PID     uint32
	Name    string
	Service string // the service the process runs, if any
	Proto   string // "udp" | "tcp"
}

// Inspector reads and controls processes and services.
type Inspector interface {
	IsAdmin() bool
	PortOwners(port uint16) ([]PortOwner, error)
	StartTime(pid uint32) (time.Time, error)
	// Alive reports whether pid still runs and started at start (so a
	// reused PID does not count).
	Alive(pid uint32, start time.Time) bool
	WaitForExit(pid uint32) error
	StopService(name string, wait time.Duration) error
	// ProcessNames lists the names of the running processes.
	ProcessNames() ([]string, error)
}

// Unsupported is the Inspector for an OS without support yet.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("procs: %w", errors.ErrUnsupported)

func (Unsupported) IsAdmin() bool                           { return false }
func (Unsupported) PortOwners(uint16) ([]PortOwner, error)  { return nil, errUnsupported }
func (Unsupported) StartTime(uint32) (time.Time, error)     { return time.Time{}, errUnsupported }
func (Unsupported) Alive(uint32, time.Time) bool            { return false }
func (Unsupported) WaitForExit(uint32) error                { return errUnsupported }
func (Unsupported) StopService(string, time.Duration) error { return errUnsupported }
func (Unsupported) ProcessNames() ([]string, error)         { return nil, errUnsupported }
