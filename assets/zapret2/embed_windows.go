package zapret2

import "embed"

//go:embed winws2.exe cygwin1.dll WinDivert.dll WinDivert64.sys lua/zapret-lib.lua lua/zapret-antidpi.lua lua/zapret-auto.lua
var FS embed.FS
