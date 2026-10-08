package goodbyedpi

import "embed"

//go:embed goodbyedpi.exe WinDivert.dll WinDivert64.sys
var FS embed.FS
