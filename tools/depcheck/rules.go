package main

// Rule forbids a package, built for one OS, from depending on some import
// paths (spec Linux §17).
type Rule struct {
	GOOS   string   // target OS for `go list`
	Pkg    string   // package pattern, e.g. "." or "./internal/app"
	Forbid []string // import-path prefixes; "a/b" matches "a/b" and "a/b/..." but not "a/bc"
	Why    string   // printed on failure
}

const mod = "github.com/hashcott/ghostline"

// internal/app is shared by the Windows GUI and the Linux daemon, so it must
// not need the GUI toolkit. On Windows it reaches winutil legitimately
// through the packages' _windows.go files; the Linux rule proves no shared
// file imports Windows code.
var wails = []string{"github.com/wailsapp/wails"}

// Rules is checked by `go run ./tools/depcheck` in CI.
var Rules = []Rule{
	{GOOS: "windows", Pkg: "./internal/app", Forbid: wails, Why: "internal/app is shared by the GUI and the daemon"},
	{GOOS: "linux", Pkg: "./internal/app", Forbid: append(wails, mod+"/internal/winutil", "golang.org/x/sys/windows"), Why: "internal/app is shared by the GUI and the daemon"},
	{GOOS: "windows", Pkg: ".", Forbid: []string{"github.com/google/nftables", "github.com/godbus/dbus",
		mod + "/internal/rpc", mod + "/internal/daemon", mod + "/internal/session"},
		Why: "Linux-only code must not reach the Windows exe"},
	{GOOS: "linux", Pkg: ".", Forbid: []string{mod + "/internal/winutil", mod + "/assets/goodbyedpi", "golang.org/x/sys/windows"},
		Why: "Windows-only code must not reach the Linux binary"},
	{GOOS: "windows", Pkg: "./internal/core", Forbid: wails, Why: "core is shared by the Windows GUI and the Linux daemon"},
	{GOOS: "linux", Pkg: "./internal/core", Forbid: wails, Why: "core is shared by the Windows GUI and the Linux daemon"},
	{GOOS: "windows", Pkg: "./internal/headless", Forbid: wails, Why: "headless modes are shared by the Windows exe and the Linux daemon"},
	{GOOS: "linux", Pkg: "./internal/headless", Forbid: wails, Why: "headless modes are shared by the Windows exe and the Linux daemon"},
	{GOOS: "linux", Pkg: "./cmd/ghostlined", Forbid: append(wails, "golang.org/x/sys/windows", mod+"/internal/winutil", mod+"/assets/goodbyedpi"),
		Why: "the daemon runs without a GUI toolkit and without Windows code"},
}
