# Ghostline Linux L1 (Cross-platform Foundation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The whole repo builds, vets and tests on Linux (GTK4) as well as Windows. Every OS-specific call sits behind a small interface, and one package, `internal/platform`, picks the implementations. Windows behaves exactly as v0.5.1, and CI guardrails keep it that way.

**Architecture:**
- Windows-only helpers still called directly from shared code (`winutil` firewall, LAN addresses, port owners, DPAPI, Task Scheduler, network key/SSID) move into five neutral packages: `firewall`, `netid`, `procs`, `secrets` and `startup.Manager`. Each package keeps its logic in an unsuffixed file and its OS calls in `_windows.go`, plus an `Unsupported` stub.
- `internal/app` stops importing Wails, so the future daemon can build without CGO.
- `internal/platform` builds a `Deps` struct per OS. `shell` and `headless.go` read only from it. On Linux, L1 wires the stubs, so the GUI runs in-process and refuses to Connect; L2 replaces this with the daemon.

**Tech Stack:** Go 1.26 (`golang.org/x/sys/unix`, `golang.org/x/sys/windows`, both already dependencies), Wails v3 beta.27 (GTK4/WebKitGTK 6.0 on Linux), GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-07-ghostline-linux-design.md` (read it with this plan; § numbers refer to it, mainly §4, §13, §17, §20, §21).

## Global Constraints

- **Windows behaviour is unchanged:**
  - The same netsh arguments, firewall rule names, DPAPI blob format (base64 of the DPAPI output), Task Scheduler XML, data paths (`%APPDATA%\Ghostline`, `%ProgramData%\Ghostline`, portable `data\`) and `state.json` v3.
  - All existing Windows tests keep passing unmodified, apart from import paths.
- **No new Go module dependencies** in L1.
- **Shared code** (any file without an OS suffix) never uses `runtime.GOOS` and never imports `golang.org/x/sys/windows` or `internal/winutil`. OS code lives only in `_windows.go` / `_linux.go`.
- **Only `internal/platform` picks implementations.** The single exception is Wails-window details inside `internal/shell` (`preflight_<os>.go`, `winopts_<os>.go`, `iconsize_<os>.go`).
- **Stub semantics** (every `Unsupported` type):
  - Operations that change the system return an error wrapping `errors.ErrUnsupported`.
  - Operations that remove or restore something are no-ops that return `nil`. Nothing was ever applied, so there is nothing to undo.
- **Interface names are OS-neutral** (spec §20). No D-Bus, netlink, nftables, systemd or WinDivert names appear in shared types.
- **Commits:**
  - Style is `type(scope): sentence`, on branch `feat/linux`.
  - **Never** add a `Co-Authored-By` trailer or a "Generated with" line.
- **Verify** after every task:
  - Linux: `CGO_ENABLED=0 go test` on the packages the task touches. `internal/app` compiles without CGO only from Task 6 on, and `internal/shell` only from Task 9 on (with WebKitGTK 6.0 installed); before that, check them with the Windows type-check.
  - Windows type-check: `GOOS=windows go vet ./internal/... ./tools/...`. From Task 9 on, the full `./...`.
  - Windows tests run only on CI. Push `feat/linux` only after asking the user.

**Deviations from spec §21 (Task 12 updates the spec):**
- `sysdns.Backend` and `state.json` v4 move from L1 to L3. The Linux snapshot fields depend on the L3 NetworkManager spike, and defining them now would mean redoing them.
- `internal/netwatch` moves to L3. In L1 the watch functions are fields on `platform.Deps`.
- `cmd/ghostlined` stays in L2.

## Review Focus

1. **Recovery after the rewiring (Windows `--restore`, `--watchdog`, startup restore).**
   - Risk: `headless.go` and `shell.go` both build `watchdog.Deps` today. If one copy loses `DeleteRule`, `RemoveCert` or `SweepSession`, firewall rules or Fake SNI roots stay behind after a crash.
   - Test: Task 9, `TestRecoveryDeps_WiresEveryCleanup`.
2. **Rule names stored in v0.5 `state.json` files.**
   - Risk: a renamed constant in the new `firewall` package strands a rule on users' machines.
   - Tests: Task 1, `TestRuleNames_Unchanged` and `TestWatchdogCleansEveryRule`.
3. **Upstream proxy passwords (`passEnc`) saved by v0.5.**
   - Risk: they must still decrypt after DPAPI moves to `secrets`; a changed encoding silently drops the password.
   - Test: Task 4, `TestEncodeString_IsBase64OfProtected`, plus the moved Windows DPAPI round-trip tests.
4. **Typed Wails events after `RegisterEvent` leaves `internal/app`.**
   - Risk: the frontend loses event typings if the registration is no longer linked.
   - Check: Task 9, binding generation must still declare all 16 event names; CI's Windows job runs `npm run build` on them.
5. **Pressing Connect on the Linux L1 dev build.**
   - Expected: it fails with a clear error before touching anything, and Disconnect/restore never error on the stubs.
   - Test: Task 7, `TestManagerOverUnsupported_FailsBeforeAnyChange`.

## Before you start (local Linux machine)

- [ ] Install the Wails CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27`
- [ ] Ask the user to install WebKitGTK 6.0, which is needed from Task 9 on to compile `internal/shell` and `main`. On Arch/CachyOS: `sudo pacman -S --needed webkitgtk-6.0`. Until then, use `CGO_ENABLED=0` and skip `internal/shell`.
- [ ] Give `main`'s `//go:embed all:frontend/dist` something to embed for compile-only checks: `mkdir -p frontend/dist && touch frontend/dist/index.html`. The directory is gitignored. Task 9 replaces the placeholder with a real frontend build.

---

### Task 1: `internal/firewall` (fixes the Linux build break)

**Files:**
- Create: `internal/firewall/firewall.go`, `internal/firewall/netsh.go`, `internal/firewall/netsh_windows.go`, `internal/firewall/netsh_test.go`, `internal/firewall/unsupported.go`
- Delete: `internal/winutil/firewall.go`, `internal/winutil/firewall_windows.go`
- Split: `internal/winutil/firewall_test.go`. Firewall tests go to `internal/firewall/netsh_test.go`; LAN-address tests stay in a renamed `internal/winutil/lanip_test.go`.
- Modify: `internal/app/deps.go`, `internal/app/dnsphase.go`, `internal/app/proxyphase.go`, `internal/app/setup.go`, `internal/app/*_test.go` (import paths), `internal/watchdog/recover.go:45`, `internal/shell/proxywire.go:186-209`, `internal/shell/shell.go`, `headless.go`

**Interfaces:**
- Produces, in `firewall.go`:
  ```go
  type Rule struct{ Name, Protocol string; Ports []int; Block bool }
  const ProxyRule = "Ghostline Proxy"
  const RuleDNSTCP = "Ghostline DNS (TCP)"; RuleDNSUDP = "Ghostline DNS (UDP)"; RuleSetup = "Ghostline Setup"; RuleBlockPublic = "Ghostline Block Public"
  var BlockPublicRule = Rule{Name: RuleBlockPublic, Block: true}
  var AllRuleNames = []string{ProxyRule, RuleDNSTCP, RuleDNSUDP, RuleSetup, RuleBlockPublic}
  type Manager interface {
      Add(port int) error            // the proxy's LAN-sharing rule
      Delete() error
      AddNamed(r Rule) error
      DeleteNamed(name string) error
      IsPublicNetwork() (bool, error)
  }
  type Unsupported struct{}         // in unsupported.go
  ```
- Produces, in `netsh.go` (neutral and testable):
  - `type Netsh struct{ Exe string; Run func(args []string) ([]byte, error); Public func() (bool, error) }`, which implements `Manager`.
  - `func AddArgs(port int, exe string) []string`, `func DeleteArgs() []string`, `func RuleArgs(r Rule, exe string) []string`, `func parsePublic(out string) bool`.
- Produces, in `netsh_windows.go`: `func NewNetsh(exe string) *Netsh`. It wires `Run` to `system32\netsh.exe` via `winutil.HiddenCmd`, and `Public` to the PowerShell `Get-NetConnectionProfile` check moved from `winutil.CurrentNetworkIsPublic`.
- `app.Firewall` keeps its four methods, with `AddNamed(r firewall.Rule) error`.

- [ ] **Step 1: Write the failing tests** in `internal/firewall/netsh_test.go`:
  - Move `TestFirewallArgs` and the `fakeNetsh` tests from `winutil/firewall_test.go`, renamed to the new functions, with the arguments byte-identical. The fake is injected through `Netsh.Run`.
  - Add these new tests:
  ```go
  func TestRuleNames_Unchanged(t *testing.T) {
      // v0.5 state.json files name these rules; renaming one strands it on users' machines.
      require.Equal(t, []string{"Ghostline Proxy", "Ghostline DNS (TCP)", "Ghostline DNS (UDP)", "Ghostline Setup", "Ghostline Block Public"}, AllRuleNames)
  }
  func TestUnsupported_RemovesAreNoOps(t *testing.T) {
      var m Manager = Unsupported{}
      require.NoError(t, m.Delete())
      require.NoError(t, m.DeleteNamed(RuleSetup))
      require.ErrorIs(t, m.Add(8080), errors.ErrUnsupported)
      require.ErrorIs(t, m.AddNamed(BlockPublicRule), errors.ErrUnsupported)
      _, err := m.IsPublicNetwork()
      require.ErrorIs(t, err, errors.ErrUnsupported)
  }
  ```
  - In `internal/watchdog`, add `TestWatchdogCleansEveryRule`: `require.Equal(t, firewall.AllRuleNames, watchdog.AllFirewallRules)`.
- [ ] **Step 2: Run the tests to verify they fail.** `CGO_ENABLED=0 go test ./internal/firewall/ ./internal/watchdog/` → FAIL (package `firewall` has no non-test files).
- [ ] **Step 3: Implement.**
  - Move the code from `winutil/firewall*.go` into the files above. `Netsh` methods keep the current "a missing rule is not an error" logic: deletion failed and `show` also fails → `nil`.
  - In `recover.go`, set `watchdog.AllFirewallRules = firewall.AllRuleNames`.
  - Replace `winutil.FirewallRule`/`winutil.Rule*` in `app` with the `firewall` names.
  - In `shell`:
    - Delete the `firewall` struct in `proxywire.go`.
    - Pass `firewall.NewNetsh(o.Executable)` into `app.Deps.Firewall`.
    - Give `proxyWiring` a field `fw firewall.Manager` for `lanInfo`'s `IsPublicNetwork`.
  - In `headless.go`, `DeleteRule` uses `firewall.NewNetsh(exe).DeleteNamed`.
- [ ] **Step 4: Run the tests to verify they pass.**
  - `CGO_ENABLED=0 go test ./internal/firewall/ ./internal/watchdog/` → PASS. `internal/app` still imports Wails until Task 6, so it is checked by the Windows vet below.
  - `CGO_ENABLED=0 GOOS=linux go vet ./internal/winutil/` → no output.
  - `GOOS=windows go vet ./internal/... .` → no output.
- [ ] **Step 5: Commit.** `refactor(firewall): move the netsh rules out of winutil into internal/firewall behind a Manager interface`

### Task 2: `internal/netid` (LAN addresses, network key, SSID)

**Files:**
- Create: `internal/netid/lanip.go` (moved from `winutil/lanip.go`), `internal/netid/lanip_test.go` (moved from `winutil/lanip_test.go`), `internal/netid/netid.go`, `internal/netid/netid_windows.go`, `internal/netid/netid_test.go`
- Modify:
  - `internal/engine/serve.go:17,171` and `internal/dnsserver/setuppage.go:14,96` (`winutil.IsPrivateOrLocal` → `netid.IsPrivateOrLocal`)
  - `internal/shell/proxywire.go:105,198` (`LocalUnicastAddrs`, `LocalLANAddrs`)
  - `internal/shell/shell.go` (`networkKey`, `CurrentSSID`, `WifiNames`, `liveAdapters`, `LANAddrs`)
  - `internal/shell/toolswire.go` (`liveAdapter` → `netid.LiveAdapter`) and its test
  - `internal/shell/system_windows.go`: remove `adaptersAddresses`, `networkKey`, `gatewayMAC`, `liveAdapters`, which move to `netid_windows.go` unchanged.

**Interfaces:**
- Produces, in `netid.go`:
  ```go
  type LiveAdapter struct{ DNS []string; Gateway string } // an up adapter's DNS servers and default gateway
  type Source interface {
      NetworkKey() string            // scanner.NetworkKey(gateway IP, gateway MAC)
      CurrentSSID() (string, error)
      WifiNames() ([]string, error)
      LiveAdapters() []LiveAdapter
  }
  type Unsupported struct{} // NetworkKey() == scanner.NetworkKey("none", "none"); SSID calls → errors.ErrUnsupported; LiveAdapters() → nil
  ```
- Produces, in `netid_windows.go`: `func NewWindows() Source`. `CurrentSSID`/`WifiNames` delegate to `winutil.CurrentSSID`/`winutil.WifiNames`.
- `LANAddrs`, `LocalLANAddrs`, `LocalUnicastAddrs` and `IsPrivateOrLocal` keep their exact signatures.

- [ ] **Step 1: Write the failing test** in `netid_test.go`:
  ```go
  func TestUnsupported(t *testing.T) {
      var s Source = Unsupported{}
      require.Equal(t, scanner.NetworkKey("none", "none"), s.NetworkKey())
      _, err := s.CurrentSSID()
      require.ErrorIs(t, err, errors.ErrUnsupported)
      _, err = s.WifiNames()
      require.ErrorIs(t, err, errors.ErrUnsupported)
      require.Empty(t, s.LiveAdapters())
  }
  ```
- [ ] **Step 2: Run the test to verify it fails.** `CGO_ENABLED=0 go test ./internal/netid/` → FAIL (undefined).
- [ ] **Step 3: Implement.** Move the files, update callers, and keep the one-minute ARP cache exactly as it is in `gatewayMAC`.
- [ ] **Step 4: Run the tests to verify they pass.**
  - `CGO_ENABLED=0 go test ./internal/netid/ ./internal/engine/ ./internal/dnsserver/` → PASS.
  - `GOOS=windows go vet ./internal/... .` → no output.
- [ ] **Step 5: Commit.** `refactor(netid): network key, Wi-Fi names and LAN addresses move out of shell and winutil into internal/netid`

### Task 3: `internal/procs` (port owners, process liveness, services)

**Files:**
- Create: `internal/procs/procs.go`, `internal/procs/procs_windows.go`, `internal/procs/procs_test.go`
- Create: `internal/shell/system.go`. The neutral `system` type moves here out of `system_windows.go`.
- Modify: `internal/app/deps.go:63` (`[]winutil.PortOwner` → `[]procs.PortOwner`), `internal/app/fakes_test.go:24,234-242`, `internal/shell/shell.go` (`StopService`, watchdog `Alive`), `headless.go` (`Alive`, `WaitForExit`)

**Interfaces:**
- Produces, in `procs.go`:
  ```go
  type PortOwner struct{ PID uint32; Name, Service, Proto string } // Proto: "udp" | "tcp"
  type Inspector interface {
      IsAdmin() bool
      PortOwners(port uint16) ([]PortOwner, error)
      StartTime(pid uint32) (time.Time, error)
      Alive(pid uint32, start time.Time) bool
      WaitForExit(pid uint32) error
      StopService(name string, wait time.Duration) error
  }
  type Unsupported struct{} // IsAdmin false; Alive false; the rest → errors.ErrUnsupported
  ```
- Produces, in `procs_windows.go`: `func NewWindows() Inspector`. It delegates to `winutil` and converts `winutil.PortOwner` field by field. `procs` imports `winutil`, never the other way round.
- Produces, in `shell/system.go`: `type system struct{ procs procs.Inspector }`, which implements `app.System`. `ListenFree` and `IPv6Available` move unchanged; `SelfPID` uses `procs.StartTime`.

- [ ] **Step 1: Write the failing test** in `procs_test.go`, called `TestUnsupported`:
  - `IsAdmin()` is false.
  - `PortOwners(53)`, `StartTime(1)`, `WaitForExit(1)` and `StopService("x", time.Second)` each return an error matching `errors.ErrUnsupported`.
  - `Alive(1, time.Time{})` is false.
- [ ] **Step 2: Run the test to verify it fails.** `CGO_ENABLED=0 go test ./internal/procs/` → FAIL.
- [ ] **Step 3: Implement.** Write the files above and update the callers.
- [ ] **Step 4: Run the tests to verify they pass.**
  - `CGO_ENABLED=0 go test ./internal/procs/` → PASS.
  - `GOOS=windows go vet ./internal/... .` → no output.
- [ ] **Step 5: Commit.** `refactor(procs): port owners, process liveness and service stop behind procs.Inspector`

### Task 4: `internal/secrets` (DPAPI behind `Protector`)

**Files:**
- Create: `internal/secrets/secrets.go`, `internal/secrets/secrets_test.go`, `internal/secrets/dpapi_windows.go` (moved from `winutil/dpapi_windows.go`), `internal/secrets/dpapi_windows_test.go` (moved)
- Delete: `internal/winutil/dpapi_windows.go`, `internal/winutil/dpapi_windows_test.go`
- Modify: `internal/shell/shell.go:250` (`Protect`), `internal/shell/proxywire.go:87` (`UnprotectString`), `internal/shell/certwire.go:38-47` (`machineProtector` → `secrets.NewMachineDPAPI()`)

**Interfaces:**
- Produces, in `secrets.go`:
  ```go
  type Protector interface {
      Protect(plain []byte) ([]byte, error)
      Unprotect(blob []byte) ([]byte, error)
  }
  // EncodeString is base64.StdEncoding of p.Protect([]byte(s)), the format v0.5 stored in passEnc.
  func EncodeString(p Protector, s string) (string, error)
  // DecodeString reverses EncodeString; an empty decoded blob is an error.
  func DecodeString(p Protector, b64 string) (string, error)
  type Unsupported struct{} // both methods → errors.ErrUnsupported
  ```
- Produces, in `dpapi_windows.go`:
  - `func NewUserDPAPI() Protector`: the current-user scope, as `winutil.ProtectString` used.
  - `func NewMachineDPAPI() Protector`: machine scope, as `winutil.ProtectMachine` used.

- [ ] **Step 1: Write the failing tests** in `secrets_test.go`, using a test-only `xorProt` that XORs each byte with `0x5a`:
  ```go
  func TestEncodeString_IsBase64OfProtected(t *testing.T) {
      got, err := EncodeString(xorProt{}, "pässwörd")
      require.NoError(t, err)
      require.Equal(t, base64.StdEncoding.EncodeToString(xor([]byte("pässwörd"))), got)
  }
  func TestDecodeString_RoundTrip(t *testing.T) { /* Encode then Decode == "pässwörd" */ }
  func TestDecodeString_RejectsEmptyAndBadBase64(t *testing.T) {
      _, err := DecodeString(xorProt{}, "")
      require.Error(t, err)
      _, err = DecodeString(xorProt{}, "%%%")
      require.Error(t, err)
  }
  func TestUnsupported(t *testing.T) { /* Protect and Unprotect → errors.ErrUnsupported */ }
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `CGO_ENABLED=0 go test ./internal/secrets/` → FAIL.
- [ ] **Step 3: Implement.** Move the DPAPI code. The Windows `Protect` for the user scope must produce exactly what `winutil.ProtectString` produced before base64; the moved round-trip tests in `dpapi_windows_test.go` pin this on Windows CI.
- [ ] **Step 4: Run the tests to verify they pass.**
  - `CGO_ENABLED=0 go test ./internal/secrets/` → PASS.
  - `GOOS=windows go vet ./internal/... .` → no output.
- [ ] **Step 5: Commit.** `refactor(secrets): DPAPI behind secrets.Protector; passEnc keeps its base64 format`

### Task 5: `startup.Manager` and a neutral `safety`

**Files:**
- Create: `internal/startup/startup.go`, `internal/startup/startup_test.go`, `internal/shell/safety.go`
- Modify: `internal/startup/tasks_windows.go` (add `NewTaskScheduler`), `internal/shell/system_windows.go` (remove `safety`), `internal/shell/shell.go:220-230`

**Interfaces:**
- Produces, in `startup.go`:
  ```go
  type Manager interface {
      SetAutostart(on bool) error // Windows: the "Ghostline Autostart" logon task
      CreateRecovery() error      // Windows: the --restore logon task
      DeleteRecovery() error
  }
  type Unsupported struct{} // SetAutostart(true), CreateRecovery → errors.ErrUnsupported; SetAutostart(false), DeleteRecovery → nil
  ```
- Produces, in `tasks_windows.go`: `func NewTaskScheduler(exe string) Manager`. It wraps the existing `Create(AutostartTask(exe))`, `Delete(brand.TaskAutostart)`, `Create(RecoveryTask(exe))` and `Delete(RecoveryTask(exe).Name)`.
- Produces, in `shell/safety.go`: `type safety struct{ startup startup.Manager; startWatchdog func(pid uint32, start time.Time) (stop func() error, err error) }`, which implements `app.Safety`. On Windows, `startWatchdog` is the current `winutil.StartDetached(exe, "--watchdog", …)` body; Task 8 moves it into `platform`.

- [ ] **Step 1: Write the failing test.** In `startup_test.go`, `TestUnsupported` asserts the stub semantics listed above.
- [ ] **Step 2: Run the test to verify it fails.** `CGO_ENABLED=0 go test ./internal/startup/` → FAIL.
- [ ] **Step 3: Implement.** `OnSettingsChanged` calls `startup.SetAutostart(n.StartWithWindows)` and logs errors as it does now.
- [ ] **Step 4: Run the tests to verify they pass.**
  - `CGO_ENABLED=0 go test ./internal/startup/` → PASS (`xml_test.go` included).
  - `GOOS=windows go vet ./internal/... .` → no output.
- [ ] **Step 5: Commit.** `refactor(startup): autostart and recovery tasks behind startup.Manager`

### Task 6: `internal/app` without Wails, and the dependency guard

**Files:**
- Create: `internal/shell/events.go`. It takes the `init()` with the 16 `application.RegisterEvent` calls from `internal/app/events.go:82-99`.
- Modify: `internal/app/events.go` (drop the `init` and the `wails` import; the event name constants and payload types stay in `app`)
- Create: `tools/depcheck/main.go`, `tools/depcheck/rules.go`, `tools/depcheck/depcheck_test.go`

**Interfaces:**
- Produces, in `tools/depcheck`:
  ```go
  type Rule struct {
      GOOS   string   // target OS for `go list`
      Pkg    string   // package pattern, e.g. "." or "./internal/app"
      Forbid []string // import-path prefixes; "a/b" matches "a/b" and "a/b/..." but not "a/bc"
      Why    string   // one line, printed on failure
  }
  var Rules []Rule
  func Violations(deps, forbid []string) []string // the deps that match a forbid entry, in deps order
  ```
- `main` runs `go list -e -deps <Pkg>` for each rule with `GOOS=<GOOS>` and `CGO_ENABLED=1` (so cgo files count as they do in a real build). It prints each violation with its `Why` and exits 1 if there are any, else prints `depcheck: <n> rules ok`.
- `Rules` in this task (Task 9 adds one more):

  | GOOS | Pkg | Forbid | Why |
  |---|---|---|---|
  | windows | `./internal/app` | `github.com/wailsapp/wails`, `github.com/hashcott/ghostline/internal/winutil`, `golang.org/x/sys/windows` | app is shared by the GUI and the future daemon |
  | linux | `./internal/app` | same | same |
  | windows | `.` | `github.com/google/nftables`, `github.com/godbus/dbus`, `github.com/hashcott/ghostline/internal/rpc`, `github.com/hashcott/ghostline/internal/daemon`, `github.com/hashcott/ghostline/internal/sessionagent` | Linux-only code must not reach the Windows exe (spec §17) |

- [ ] **Step 1: Write the failing tests** in `depcheck_test.go`:
  ```go
  func TestViolations(t *testing.T) {
      deps := []string{"github.com/hashcott/ghostline/internal/model", "github.com/wailsapp/wails/v3/pkg/application"}
      require.Equal(t, []string{"github.com/wailsapp/wails/v3/pkg/application"}, Violations(deps, []string{"github.com/wailsapp/wails"}))
      require.Empty(t, Violations(deps, []string{"github.com/google/nftables"}))
  }
  func TestViolations_PrefixIsPathAware(t *testing.T) {
      require.Empty(t, Violations([]string{"github.com/hashcott/ghostline/internal/rpcx"}, []string{"github.com/hashcott/ghostline/internal/rpc"}))
  }
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./tools/depcheck/` → FAIL.
- [ ] **Step 3: Implement.** Write depcheck and move the event registration.
- [ ] **Step 4: Run and verify.**
  - `go test ./tools/depcheck/` → PASS.
  - `go run ./tools/depcheck` → `depcheck: 3 rules ok`.
  - `CGO_ENABLED=0 GOOS=linux go test ./internal/app/...` → PASS. This is the L1 spike: `app` now builds without CGO.
  - If an app test fails on Linux only because of an OS assumption (a `\` path literal, a Windows-only error string), make the test OS-neutral (`filepath.Join`, `errors.Is`). Never skip it.
- [ ] **Step 5: Commit.** `refactor(app): event registration moves to shell so app has no Wails dependency; tools/depcheck guards it`

### Task 7: `Unsupported` stubs for the remaining OS interfaces, and asset tags

**Files:**
- Create: `internal/sysdns/unsupported.go`, `internal/sysproxy/unsupported.go`, `internal/certstore/unsupported.go`, `internal/dpi/unsupported.go`, and one `unsupported_test.go` per package
- Rename: `assets/goodbyedpi/embed.go` → `embed_windows.go`, `assets/goodbyedpi/embed_test.go` → `embed_windows_test.go`; the same for `assets/zapret2`. Add `doc.go` (package clause and doc comment only) to both, so the packages exist on Linux.
- Modify: `internal/certstore/store_integration_test.go:1` (`//go:build integration` → `//go:build windows && integration`)

**Interfaces:**
- Produces:
  - `sysdns.Unsupported`, which implements `sysdns.API`: `Adapters`, `GetDNS`, `SetDNS` and `NetshSetDNS` → `errors.ErrUnsupported`; `Flush` → `nil`.
  - `sysproxy.Unsupported`, which implements `sysproxy.API`: `Query` → zero snapshot and `errors.ErrUnsupported`; `Set` → `errors.ErrUnsupported`.
  - `certstore.Unsupported`, which implements `certstore.Store`: `Install` → `errors.ErrUnsupported`; `Remove` → `nil`; `List` → `nil, nil`.
  - `dpi.UnsupportedRunner`, which implements `dpi.Runner`: `Start` → `nil, errors.ErrUnsupported`.
  - `dpi.NoServices`, which implements `dpi.Services`: `Find` → `nil, nil`; `Running` → `false, nil`; `Stop`, `Delete` → `nil`.

- [ ] **Step 1: Write the failing tests.**
  - One `TestUnsupported` per package asserting the semantics above.
  - In `internal/sysdns/unsupported_test.go`, add:
  ```go
  func TestManagerOverUnsupported_FailsBeforeAnyChange(t *testing.T) {
      m := NewManager(Unsupported{}, func(time.Duration) {})
      _, err := m.Select("auto", nil)
      require.ErrorIs(t, err, errors.ErrUnsupported) // Connect stops at the first DNS step
      require.Empty(t, m.Restore(nil))               // and a restore with nothing recorded succeeds
      require.NoError(t, m.Flush())
  }
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `CGO_ENABLED=0 go test ./internal/sysdns/ ./internal/sysproxy/ ./internal/certstore/ ./internal/dpi/` → FAIL.
- [ ] **Step 3: Implement.** Write the stubs and rename the asset files.
- [ ] **Step 4: Run the tests to verify they pass.**
  - The same command → PASS.
  - `CGO_ENABLED=0 GOOS=linux go vet ./assets/...` → no output.
  - `GOOS=windows go vet ./internal/... ./assets/... .` → no output.
- [ ] **Step 5: Commit.** `feat(linux): Unsupported stubs for system DNS, proxy, cert store and DPI; engine assets embed only on Windows`

### Task 8: `internal/platform`

**Files:**
- Create: `internal/platform/platform.go`, `internal/platform/platform_windows.go`, `internal/platform/platform_linux.go`, `internal/platform/flock_linux.go`, `internal/platform/platform_test.go`, `internal/platform/flock_linux_test.go`

**Interfaces:**
- Produces, in `platform.go`:
  ```go
  type Deps struct {
      Paths          store.Paths
      Lock           store.Locker
      DNS            sysdns.API
      WatchNetwork   func(onChange func()) (stop func(), err error)
      SysProxy       sysproxy.API
      WatchSysProxy  func(onChange func()) (stop func(), err error)
      Certs          certstore.Store
      Firewall       firewall.Manager
      DPIRunner      dpi.Runner
      DPIServices    dpi.Services
      DPIEngines     func(list func() strategies.List) []dpi.Installed
      Startup        startup.Manager
      StartWatchdog  func(pid uint32, start time.Time) (stop func() error, err error)
      UserSecrets    secrets.Protector
      MachineSecrets secrets.Protector
      SecureDir      func(dir string) error            // nil: no ACL step on this OS
      OwnedByAdmins  func(path string) (bool, error)   // nil: no ownership check on this OS
      NetID          netid.Source
      Procs          procs.Inspector
      AttachConsole  func()
      UsesDaemon     bool // false on both OSes in L1; Linux turns true in L2
  }
  func New(exe string) (Deps, error) // one per OS file
  ```
- **Windows `New`:** exactly today's wiring.
  - `Paths`: `store.WithMachineDir(store.ResolvePaths(exe, os.Getenv("APPDATA")), filepath.Join(os.Getenv("ProgramData"), brand.AppName))`.
  - `Lock`: `winutil.NewNamedMutex(brand.StateMutex)`.
  - `sysdns.NewWindowsAPI()`, `sysdns.Watch`, `sysproxy.NewWindowsAPI()`, `sysproxy.Watch`, `certstore.NewWindows(certstore.LocalMachine)`, `firewall.NewNetsh(exe)`.
  - `dpi.NewWindowsRunner()`, `dpi.NewWindowsServices()`.
  - `DPIEngines`: GoodbyeDPI and zapret2 with the `assets/goodbyedpi` / `assets/zapret2` filesystems (moved from `shell/strategies.go:61-69`).
  - `startup.NewTaskScheduler(exe)`.
  - `StartWatchdog`: the `winutil.StartDetached` body from Task 5.
  - `secrets.NewUserDPAPI()`, `secrets.NewMachineDPAPI()`, `winutil.SecureDir`, `winutil.OwnedByAdmins`.
  - `netid.NewWindows()`, `procs.NewWindows()`.
  - `AttachConsole`: `func() { winutil.AttachParentConsole() }`.
- **Linux `New` (L1, a dev stub):**
  - `Paths`: `store.ResolvePaths(exe, <os.UserConfigDir()>)`. This is temporary; L2 moves data to `/var/lib/ghostline/data`.
  - `Lock`: `newFileLock(filepath.Join(paths.DataDir, "state.lock"))`.
  - Every interface is its `Unsupported` stub.
  - Watch funcs return `nil, errors.ErrUnsupported`.
  - `DPIEngines` returns `nil`.
  - `StartWatchdog` returns `nil, errors.ErrUnsupported`.
  - `SecureDir` and `OwnedByAdmins` are `nil`; `AttachConsole` is a no-op.
- Produces, in `flock_linux.go`: `func newFileLock(path string) *fileLock`, which implements `store.Locker`. `Lock` opens the file with `O_CREATE|O_RDWR` and mode `0o600`, then calls `unix.Flock(fd, unix.LOCK_EX)`. `Unlock` calls `LOCK_UN` and closes the file.

- [ ] **Step 1: Write the failing tests.**
  - `platform_test.go`, untagged, runs on whichever OS runs it:
  ```go
  // Every Deps field must be set by every OS, so adding a field forces each platform file to decide.
  func TestNew_FillsEveryField(t *testing.T) {
      d, err := New(filepath.Join(t.TempDir(), "ghostline"))
      require.NoError(t, err)
      v := reflect.ValueOf(d)
      for i := 0; i < v.NumField(); i++ {
          name := v.Type().Field(i).Name
          if name == "SecureDir" || name == "OwnedByAdmins" || name == "UsesDaemon" {
              continue // documented as optional
          }
          f := v.Field(i)
          switch f.Kind() {
          case reflect.Func, reflect.Interface:
              require.False(t, f.IsNil(), name)
          }
      }
      require.NotEmpty(t, d.Paths.DataDir)
  }
  ```
  - `flock_linux_test.go`:
  ```go
  func TestFileLock_Exclusive(t *testing.T) {
      p := filepath.Join(t.TempDir(), "state.lock")
      a, b := newFileLock(p), newFileLock(p)
      require.NoError(t, a.Lock())
      got := make(chan struct{})
      go func() { _ = b.Lock(); close(got) }()
      select {
      case <-got:
          t.Fatal("second lock acquired while the first is held")
      case <-time.After(100 * time.Millisecond):
      }
      require.NoError(t, a.Unlock())
      select {
      case <-got:
      case <-time.After(2 * time.Second):
          t.Fatal("second lock never acquired")
      }
      require.NoError(t, b.Unlock())
  }
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `CGO_ENABLED=0 go test ./internal/platform/` → FAIL.
- [ ] **Step 3: Implement** the four files.
- [ ] **Step 4: Run the tests to verify they pass.**
  - `CGO_ENABLED=0 go test ./internal/platform/` → PASS.
  - `GOOS=windows go vet ./internal/platform/` → no output.
- [ ] **Step 5: Commit.** `feat(platform): internal/platform builds every OS dependency in one place (Windows as today, Linux stubs)`

### Task 9: Shell and headless read only from `platform.Deps`; the Linux GUI builds

**Files:**
- Create:
  - `internal/shell/recovery.go`, `internal/shell/recovery_test.go`
  - `internal/shell/preflight_windows.go`: `webView2Installed`, `messageBox` and `fatalBox` from `system_windows.go`.
  - `internal/shell/preflight_linux.go`
  - `internal/shell/winopts_windows.go`, `internal/shell/winopts_linux.go`
  - `internal/shell/iconsize_windows.go`, `internal/shell/iconsize_linux.go`
- Delete: `internal/shell/system_windows.go` (its contents now live in `netid`, `system.go`, `safety.go`, `preflight_windows.go` and `platform`)
- Modify:
  - `internal/shell/shell.go`: `Options` loses `GoodbyeDPIAssets`/`Zapret2Assets` and gains `Platform platform.Deps`. Every `winutil.*`, `NewWindows*`, `sysdns.Watch`/`sysproxy.Watch` and `startup.*` call reads from `o.Platform`.
  - `internal/shell/strategies.go:61`: `NewDPIManager(paths store.Paths, p platform.Deps, list func() strategies.List) *dpi.Manager`
  - `internal/shell/certwire.go`, `internal/shell/proxywire.go`, `internal/shell/ui.go:171,232` (`trayIconSize()`)
  - `main.go`, `headless.go`, `stopdpi.go`
  - `tools/depcheck/rules.go`: add the Linux `.` rule.

**Interfaces:**
- Consumes `platform.Deps` (Task 8), `firewall.Manager` (Task 1) and `procs.Inspector` (Task 3).
- Produces, in `recovery.go`: `func RecoveryDeps(p platform.Deps, states *store.StateStore, stopDPI func() error, log *slog.Logger) watchdog.Deps`. This is the single builder used by `shell.Run` (replacing `shell.go:86-98`) and `runHeadless` (replacing `headless.go:56-71`).
  - `DNS`: `sysdns.NewManager(p.DNS, time.Sleep)`.
  - `Alive`: `p.Procs.Alive`.
  - `RestoreSysProxy`: `sysproxy.Manager{API: p.SysProxy}.RestoreIfOurs`.
  - `DeleteRule`: `p.Firewall.DeleteNamed`.
  - `RemoveCert`: `certstore.RemoveIfPrefix(p.Certs, t, certs.SessionPrefix)`.
  - `SweepSession`: `certstore.Sweep(p.Certs, certs.SessionPrefix, keep)`.
- Produces, in the `preflight_<os>.go` pair: `func preflight() error` (Windows: the WebView2 check plus the message box; Linux: `nil`) and `func fatalBox(err error)` (Linux: writes to `os.Stderr`).
- Produces, in the `winopts_<os>.go` pair: `func applyPlatformOptions(opts *application.Options, orch *app.Orchestrator)`.
  - Windows: today's `WindowsOptions{DisableQuitOnLastWindowClosed, WndProcInterceptor}` block.
  - Linux: the `LinuxOptions` equivalent of "do not quit on last window closed", if beta.27 has one; otherwise nothing.
- Produces, in the `iconsize_<os>.go` pair: `func trayIconSize() int` (Windows: `winutil.SmallIconSize()`; Linux: `32`).
- `main.go` calls `platform.New(exe)`. An error exits 1, after `fatalBox` for the GUI mode. It passes the result to `shell.Run` and to `runHeadless(mode, p)`. `main.go` no longer imports `assets/*`.
- New depcheck rule: `{GOOS: "linux", Pkg: ".", Forbid: ["github.com/hashcott/ghostline/internal/winutil", "github.com/hashcott/ghostline/assets/goodbyedpi", "golang.org/x/sys/windows"], Why: "Windows-only code must not reach the Linux binary"}`.

- [ ] **Step 1: Write the failing test** in `recovery_test.go`:
  ```go
  // Review Focus 1: both the GUI and the headless --restore/--watchdog paths use this builder;
  // a missing hook leaves firewall rules or Fake SNI roots behind after a crash.
  func TestRecoveryDeps_WiresEveryCleanup(t *testing.T) {
      p := platform.Deps{DNS: sysdns.Unsupported{}, SysProxy: sysproxy.Unsupported{}, Certs: certstore.Unsupported{},
          Firewall: firewall.Unsupported{}, Procs: procs.Unsupported{}}
      d := RecoveryDeps(p, store.NewStateStore(filepath.Join(t.TempDir(), "state.json"), nil), func() error { return nil }, slog.Default())
      require.NotNil(t, d.DNS)
      require.NotNil(t, d.Alive)
      require.NotNil(t, d.StopDPI)
      require.NotNil(t, d.RestoreSysProxy)
      require.NotNil(t, d.DeleteRule)
      require.NotNil(t, d.RemoveCert)
      require.NotNil(t, d.SweepSession)
  }
  ```
- [ ] **Step 2: Run the test to verify it fails.** `go test ./internal/shell/ -run TestRecoveryDeps` → FAIL (undefined). This needs WebKitGTK 6.0 installed; see "Before you start".
- [ ] **Step 3: Implement.** Rewire the shell and headless code as listed under Files and Interfaces, and add the new depcheck rule.
- [ ] **Step 4: Run the tests and checks.**
  - `go test ./...` → PASS (Linux, CGO on).
  - `GOOS=windows go vet ./...` → no output.
  - `go run ./tools/depcheck` → `depcheck: 4 rules ok`.
  - Real frontend: `cd frontend && npm ci && cd .. && wails3 generate bindings -clean=true -ts -i && cd frontend && npm run build && cd ..`. Then `grep -rhoE '"(state|stats|log|query|scan:progress|dpi:autotune|update|proxy:stats|proxy:conn|rules:compiled|lists:progress|dnsserver:stats|certs:changed|setup:countdown|tools:scan|tools:cfscan)"' frontend/bindings | sort -u | wc -l` → `16` (Review Focus 4).
  - Manual check: `go run .` opens the Ghostline window on Linux. Pressing Connect shows an error. `ip`/`resolvectl status` show DNS unchanged, and `ls ~/.config/Ghostline/state.json` either does not exist or has `"phase": "clean"` (Review Focus 5).
- [ ] **Step 5: Commit.** `refactor(shell): GUI and headless modes wire everything from platform.Deps; the Linux GUI builds and refuses to connect`

### Task 10: CI on Windows and Linux

**Files:**
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- `jobs.test` keeps `runs-on: windows-latest`, and every step stays as it is.
- New `jobs.linux` on `ubuntu-24.04`:
  1. Checkout, then `setup-go` 1.27 and `setup-node` 24, as in `test`.
  2. `sudo apt-get update && sudo apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev`.
  3. Install wails3 v3.0.0-beta.27, `npm ci`, generate bindings, `npm run build`.
  4. `golangci-lint` action (same config).
  5. `go test ./...`.
  6. `CGO_ENABLED=1 go test -race ./internal/rules/... ./internal/proxy/... ./internal/app/...`.
  7. `GOOS=windows go vet ./...`, so Windows breakage from Linux-side work shows on every push.
  8. `go run ./tools/depcheck`.
  9. `go build -o /dev/null .` (a real Linux GUI build; packaging is L5).
- Frontend tests stay only in the Windows job, since they are OS-independent.

- [ ] **Step 1: Edit the workflow** as described.
- [ ] **Step 2: Validate the YAML locally.** `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml` → no output. This runs without adding a module dependency.
- [ ] **Step 3: Ask the user before pushing `feat/linux`.** After they agree, push and confirm both jobs are green with `gh run watch`. If the Windows job fails, fix it here; that failure is the first real run of the Windows tests on the moved code.
- [ ] **Step 4: Commit.** `ci: Linux job (GTK4 build, tests, race, cross-vet for Windows, depcheck) next to the Windows job`

### Task 11: Release guard on the Windows exe size

**Files:**
- Create: `tools/sizecheck/main.go`, `tools/sizecheck/sizecheck_test.go`
- Modify: `.github/workflows/release.yml`, adding a step between "Package" and "Checksums".

**Interfaces:**
- Produces: `func Grew(oldSize, newSize int64, maxPct float64) (pct float64, ok bool)`, where `pct = (new-old)/old*100` and `ok = pct <= maxPct`.
- CLI: `sizecheck -old <zip> -new <zip> -entry ghostline.exe -max 2`. It reads the uncompressed size of `entry` from each zip with `archive/zip`, prints `ghostline.exe: <old> → <new> bytes (<pct>%)`, and exits 1 when `!ok`.
- Workflow step (pwsh):
  - `$prev = gh release view --json tagName -q .tagName`. This is the newest published release, because the current one is not created yet.
  - If `$prev` is empty, skip.
  - Otherwise, `gh release download $prev -p "*portable.zip" -D prev`, then `go run ./tools/sizecheck -old (Get-ChildItem prev/*.zip).FullName -new bin/Ghostline-$v-portable.zip -entry ghostline.exe -max 2`.
  - Raising the limit is an explicit edit of `-max` in the workflow.

- [ ] **Step 1: Write the failing test:**
  ```go
  func TestGrew(t *testing.T) {
      pct, ok := Grew(100_000_000, 101_500_000, 2)
      require.InDelta(t, 1.5, pct, 1e-9)
      require.True(t, ok)
      _, ok = Grew(100_000_000, 102_500_000, 2)
      require.False(t, ok)
      _, ok = Grew(100_000_000, 90_000_000, 2) // shrinking is always fine
      require.True(t, ok)
  }
  ```
- [ ] **Step 2: Run the test to verify it fails.** `go test ./tools/sizecheck/` → FAIL.
- [ ] **Step 3: Implement** the tool and the workflow step.
- [ ] **Step 4: Run and verify.**
  - `go test ./tools/sizecheck/` → PASS.
  - `actionlint .github/workflows/release.yml` → no output.
- [ ] **Step 5: Commit.** `ci(release): fail the release when the Windows exe grows more than 2% over the previous release`

### Task 12: Docs: spec deviations and `docs/platforms.md`

**Files:**
- Modify: `docs/superpowers/specs/2026-10-07-ghostline-linux-design.md`
  - §21: the L1 row drops `sysdns.Backend` and `state.json` v4, and the L3 row gains them together with `netwatch`.
  - §4.3: `netwatch` is marked "L3".
- Create: `docs/platforms.md`

**Interfaces:**
- `docs/platforms.md` is one table, with rows for: data paths, state lock, system DNS, network watch, system proxy, cert store, firewall, DPI runner/filter, autostart/recovery, watchdog, secrets, network key/SSID, port owners, GUI preflight. Each row has three columns:
  - **Windows:** the mechanism and its file.
  - **Linux:** "stub (L3)", "stub (L4)", or the real mechanism where L1 already has one (the state lock).
  - **Tests:** the test file(s).
- The table starts with one line: "Edit this table in the same commit that changes a row."

- [ ] **Step 1: Write both edits.**
- [ ] **Step 2: Check.** Every file path named in `docs/platforms.md` exists: `grep -oE '\(?internal/[A-Za-z0-9_/.-]+\.go' docs/platforms.md | tr -d '(' | xargs ls` → no "No such file".
- [ ] **Step 3: Commit.** `docs: platform matrix for maintainers; the Linux spec moves sysdns.Backend, state.json v4 and netwatch to L3`
