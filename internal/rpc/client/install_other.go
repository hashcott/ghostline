//go:build !linux

package client

import "github.com/hashcott/ghostline/internal/app"

// installer: no service buttons outside Linux.
type installer struct{}

func newInstaller() *installer { return &installer{} }

func (*installer) info() app.InstallInfo { return app.InstallInfo{} }
func (*installer) install() error        { return &app.AppError{Code: app.CodeServiceActionUnsupported} }
func (*installer) start() error          { return &app.AppError{Code: app.CodeServiceActionUnsupported} }
