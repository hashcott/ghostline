// Package zapret2 embeds the official zapret2 builds, unmodified, plus the
// Lua libraries strategies call. The Windows build carries windows-x86_64
// winws2 (MIT), the Cygwin runtime it needs (LGPLv3) and the WinDivert
// driver it ships with (LGPLv3); the Linux build carries linux-x86_64
// nfqws2 (MIT, statically linked). Each OS embeds only its own files.
package zapret2
