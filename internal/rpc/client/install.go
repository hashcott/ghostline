package client

import (
	"time"

	"github.com/hashcott/ghostline/internal/app"
)

// daemonWait bounds the wait for the daemon after the service was
// installed or started.
var daemonWait = 10 * time.Second

// ServiceInstall says which service buttons to show (the GUI answers it,
// not the daemon, which may be down).
func (s *Service) ServiceInstall() app.InstallInfo { return s.inst.info() }

// InstallService installs (or updates) the background service through
// pkexec, then waits for the daemon.
func (s *Service) InstallService() error {
	if err := s.inst.install(); err != nil {
		return err
	}
	return s.waitDaemon()
}

// StartService enables and starts the background service through pkexec,
// then waits for the daemon.
func (s *Service) StartService() error {
	if err := s.inst.start(); err != nil {
		return err
	}
	return s.waitDaemon()
}

func (s *Service) waitDaemon() error {
	deadline := time.Now().Add(daemonWait)
	for {
		_, err := s.conn.client()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
}
