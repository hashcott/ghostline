# Ghostline Linux L4 (DPI + System Integration) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** zapret2 DPI, the system proxy, Fake SNI certificates, LAN-sharing firewall rules and network identity work on Linux, every change is given back on Disconnect, crash and power loss, and the shared interfaces lose their Windows-only shapes.

**Architecture:**
- DPI: `dpi.Interceptor` replaces `dpi.Services`. Windows keeps WinDivert behind it. Linux installs an nftables table (`google/nftables`) generated from one shared port table (`zapret2.Filter`) and runs the embedded `nfqws2`.
- Proxy: `sysproxy.Backend` with `model.ProxySnapshot` (state v5). Windows wraps today's WinINET `Manager`. Linux forwards to GNOME/KDE code that runs in the user's session through `internal/session` (daemon spawns `ghostlined --session-agent` as the user).
- Certificates: a Linux `certstore.Store` composed of targets (system anchors, Firefox policy, user NSS via the agent).
- Firewall: firewalld (D-Bus), ufw (exec) or none, remembering ports in `data/firewall-rules.json`.
- Network identity: `/proc/net/route` + `/proc/net/arp`, SSID over NetworkManager D-Bus.
- UI: `Snapshot.Platform`, Linux string override bundles, capability flags (`DPIInfo`, `SysProxyInfo`).

**Tech Stack:** Go 1.26, `github.com/google/nftables` (new, Linux only), `github.com/godbus/dbus/v5`, `golang.org/x/sys/unix`, zapret2 v1.0.5.2 `nfqws2`, React + i18next.

**Spec:** `docs/superpowers/specs/2026-10-08-ghostline-linux-l4-design.md` (§ numbers below refer to it). Parents: the L3 spec and the Linux master spec.

## Global Constraints

- **Windows behaviour is unchanged.** Every existing test passes with only mechanical signature updates. `winws2` gets exactly today's arguments; WinDivert services are removed at the same moments; the WinINET proxy snapshot/restore is byte-for-byte the same.
- **Windows exe** grows ≤ 2% (`tools/sizecheck`); `tools/depcheck` keeps godbus, nftables, `x/sys/unix` out of the Windows build. Add a depcheck rule if `github.com/google/nftables` is not yet forbidden for `GOOS=windows`.
- **Shared code** never branches on `runtime.GOOS`. OS code lives in `_linux.go`/`_windows.go`; logic and fakes stay unsuffixed.
- **New module:** only `github.com/google/nftables` (and what it pulls). `go mod tidy` clean.
- **Pinned values:** zapret2 `v1.0.5.2`; `nfqws2` SHA-256 `de1414b1e0f9a5659d438cddcb0c7e9099b4533bf5c8a3cf841c7bc7e5aa1473`; queue `200`; fwmark `0x40000000`; drop to user `nobody`; table `inet ghostline`; chain `post` (postrouting, priority 99), chain `pre` (prerouting, priority -99); packets TCP out 20 / in 10, UDP out 5 / in 3.
- **Linux paths:** engine dir `/var/lib/ghostline/bin/zapret2` (`Paths.BinDir` = `<parent of DataDir>/bin`); `<DataDir>/firewall-rules.json`, `<DataDir>/session-queue.json`, `<DataDir>/nss-users.json`; Firefox `/etc/firefox/policies/policies.json`; anchors per §7; anchor file names `ghostline-<thumbprint>.crt`.
- **New codes (each with vi/en strings in the task that introduces it):** `DPI_KERNEL_UNSUPPORTED`, `PROXY_DESKTOP_UNSUPPORTED`, `SESSION_PENDING`, `CERT_PARTIAL`, `CERT_NSS_TOOL_MISSING`, `FIREWALL_UNKNOWN`.
- **New `Service` methods** (`DPIInfo`, `SysProxyInfo`) require `go generate ./...` (RPC proxy) and the Wails bindings regenerated; frontend test mocks get them.
- **Commits:** `type(scope): sentence` on branch `feat/linux-l4`. **Never** add a `Co-Authored-By` trailer or a "Generated with" line.
- **Verify after every task:** `go test ./...`; `GOOS=windows go vet ./...`; `go run ./tools/depcheck`; `CGO_ENABLED=0 go build ./cmd/ghostlined`; for frontend tasks `cd frontend && npx vitest run`.
- **Never** run `sudo`, change system DNS/proxy/firewall/certificates or load nft rules on the dev machine without asking the user first. Root checks are scripts the user runs.

## Review Focus

1. **The daemon restarts while DPI was running** (old table still present, `state.DPI.running`). Expected: `Prepare` replaces the table instead of failing or doubling rules; no stale `nfqws2`. Test: Task 4 `TestNft_InstallReplacesExistingTable` (root), Task 5 `TestLinuxInterceptor_PrepareTwice`.
2. **The active session changes between Apply and Restore** (user switch, or user B logged in after A logged out). Expected: restore targets the uid recorded in the snapshot, or queues for that uid; never touches B's settings. Test: Task 11 `TestLinuxProxy_RestoreUsesRecordedUID`.
3. **`policies.json` is malformed, or already has the admin's own `Certificates.Install` entries.** Expected: malformed → `ErrPartial` for Firefox and the file untouched; existing entries and other policies kept on install and removal. Test: Task 12 `TestFirefox_MalformedPolicyUntouched`, `TestFirefox_KeepsOtherEntries`.
4. **The user already had `ufw allow 8080/tcp` before Ghostline opened the same port.** Expected: Ghostline does not add a duplicate and does not delete the user's rule later. Test: Task 14 `TestUfw_PreexistingRuleNotRemoved`.
5. **A fresh KDE user with no `kioslaverc` (or no `[Proxy Settings]` group).** Expected: snapshot records "absent"; restore deletes the keys Ghostline wrote and leaves no group behind. Test: Task 10 `TestKDE_RestoreAbsentDeletesKeys`.

---

### Task 1: Spike — minimal nft table + `nfqws2` on the dev machine

**Files:**
- Create (scratchpad, not committed): `<scratchpad>/l4spike/spike.sh`
- Modify: `docs/superpowers/specs/2026-10-08-ghostline-linux-l4-design.md` (§3 spike row: result)

- [ ] **Step 1: Prepare.** Extract `nfqws2` and `lua/*.lua` from `zapret2-v1.0.5.2.tar.gz` into the scratchpad; check the SHA-256 above.
- [ ] **Step 2: Write `spike.sh`** (run as root by the user). It: loads modules (`modprobe nfnetlink_queue nft_queue`); creates `table inet ghostline_spike` with chain `post` (`type filter hook postrouting priority 99`) holding `meta mark and 0x40000000 == 0 oifname != "lo" tcp dport {80,443} ct original packets 1-20 queue num 200 bypass`; runs `nfqws2 --qnum=200 --fwmark=0x40000000 --user=nobody --lua-init=@lua/zapret-lib.lua --lua-init=@lua/zapret-antidpi.lua --filter-tcp=80,443 --lua-desync=fake:blob=fake_default_tls:tcp_md5 --debug=1` for 10 s while `curl -sI https://example.com` runs; then `kill -9` nfqws2 and curls again; then deletes the table. A `trap` always deletes the table and kills nfqws2. Output to `spike.log`.
- [ ] **Step 3: Ask the user to run it.** Expected: debug lines show packets from queue 200 while running; both curls succeed (the second one proves `bypass`); `nft list tables` after the script has no `ghostline_spike`.
- [ ] **Step 4: Record** the result (or the corrected flags/priorities) in spec §3 and commit: `docs(spec): L4 spike: nft queue and nfqws2 verified on the dev machine`. If the spike fails, stop and report — later tasks depend on it.

### Task 2: `dpi.Interceptor` replaces `dpi.Services`

**Files:**
- Create: `internal/dpi/interceptor.go` (interface, `InterceptorInfo`, `NoInterceptor`), `internal/dpi/interceptor_windows.go` (`NewWinDivert() Interceptor`)
- Modify: `internal/dpi/manager.go`, `internal/dpi/unsupported.go`, `internal/dpi/runner_windows.go` (services move), `internal/platform/platform.go`, `platform_windows.go`, `platform_linux.go`, `internal/core/strategies.go`, `internal/dpi/dpi_test.go`

**Interfaces:**
- Produces:
  ```go
  type Interceptor interface {
  	Prepare() error
  	Ready(pid int) bool
  	Cleanup() error
  	Info() InterceptorInfo
  }
  type InterceptorInfo struct{ Mechanism string; AVExclusions bool }
  func NewManager(binDir string, engines []Installed, r Runner, ic Interceptor, sleep func(time.Duration)) *Manager
  func (m *Manager) Info() InterceptorInfo
  type NoInterceptor struct{} // Prepare/Cleanup nil, Ready true, Info{}
  ```
  `platform.Deps.DPIInterceptor dpi.Interceptor` replaces `DPIServices`.
- Windows `Interceptor`: `Prepare` and `Cleanup` = today's "find `driverService` services, Stop+Delete each"; `Ready` = `Running(driverService)`; `Info{Mechanism: "WinDivert", AVExclusions: true}`.

- [ ] **Step 1: Failing tests** in `internal/dpi/dpi_test.go` with a recording fake interceptor:
  ```go
  func TestManager_StartCallsPrepareThenWaitsReady(t *testing.T) // calls: prepare, runner.start, ready(pid); PID returned
  func TestManager_NotReadyKillsAndFails(t *testing.T)           // Ready false → proc killed, errors.Is(err, ErrStartFailed)
  func TestManager_StopKillsThenCleansUp(t *testing.T)           // order: kill, cleanup
  func TestManager_PrepareErrorIsReturned(t *testing.T)          // Prepare error → returned, runner not called
  ```
  Existing tests that asserted WinDivert service calls move to `interceptor_windows_test.go` against `NewWinDivert` with the fake `Services` they used.
- [ ] **Step 2:** `go test ./internal/dpi/` → FAIL (compile).
- [ ] **Step 3: Implement.** `Start`: `stopLocked` → verify/extract → `ic.Prepare()` → lists → args → `runner.Start` → `sleep(2s)` → exited? (existing checks) → `!ic.Ready(pid)` → kill + `ErrStartFailed`. `stopLocked`: existing autohostlist copy and kill, then `ic.Cleanup()`. `Services` stays only inside `interceptor_windows.go`.
- [ ] **Step 4:** `go test ./internal/dpi/ ./...`, `GOOS=windows go test -c -o /dev/null ./internal/dpi/`, `GOOS=windows go vet ./...` → clean.
- [ ] **Step 5: Commit** `refactor(dpi): an Interceptor prepares, checks and cleans up packet capture; WinDivert services move behind it`

### Task 3: Shared port table, per-OS engine arguments and pins, `nfqws2` embedded

**Files:**
- Create: `internal/dpi/capture.go`, `internal/dpi/zapret2/filter.go`, `internal/dpi/zapret2/args_windows.go`, `internal/dpi/zapret2/args_linux.go`, `internal/dpi/zapret2/pins_windows.go`, `internal/dpi/zapret2/pins_linux.go`, `assets/zapret2/embed_linux.go`, `assets/zapret2/nfqws2` (fetched, committed like the Windows binaries are), tests `filter_test.go`, `args_windows_test.go`, `args_linux_test.go`
- Modify: `internal/dpi/zapret2/engine.go`, `internal/dpi/zapret2/pins.go` (Lua only), `tools/fetchdpi/main.go` (+ test), `assets/zapret2/doc.go`, `internal/platform/platform_linux.go` (`DPIEngines` returns zapret2 with `assets/zapret2.FS`)

**Interfaces:**
- Produces:
  ```go
  // internal/dpi/capture.go — in dpi because zapret2 imports dpi.
  type CaptureRule struct {
  	Proto      string // "tcp" | "udp"
  	Ports      []int
  	OutPackets int
  	InPackets  int
  	QUIC       bool
  }
  // internal/dpi/zapret2/filter.go
  var Filter = []dpi.CaptureRule{{"tcp", []int{80, 443}, 20, 10, false}, {"udp", []int{443}, 5, 3, true}}
  func interceptArgs(quic bool) []string // per-OS file
  var binaryPins map[string]string       // per-OS file; Pinned() merges it with the Lua pins
  const QueueNum = 200
  const FWMark = 0x40000000
  ```
- Windows `interceptArgs(quic)` = `--wf-tcp-out=80,443` [+ `--wf-udp-out=443`] + `--wf-dup-check=1`, built from `Filter`.
- Linux `interceptArgs(quic)` = `--qnum=200`, `--fwmark=0x40000000`, `--user=nobody` (no `--wf-*`).
- `Engine.Exe()` is `winws2.exe` on Windows, `nfqws2` on Linux (per-OS const).

- [ ] **Step 1: Failing tests.**
  ```go
  func TestArgs_WindowsUnchanged(t *testing.T)   // (windows) golden: today's full arg list for a TCP-only and a QUIC strategy, copied from the current tests
  func TestArgs_LinuxQueueAndUser(t *testing.T)  // (linux) contains "--qnum=200","--fwmark=0x40000000","--user=nobody"; no arg starts with "--wf-"; lua-init and profiles as Windows
  func TestFilter_PortsList(t *testing.T)        // portsCSV(Filter[0]) == "80,443"
  func TestPins_LinuxBinary(t *testing.T)        // (linux) Pinned()["nfqws2"] == "de1414b1…a3"; Lua pins present
  func TestFetch_LinuxBinaryVerified(t *testing.T) // fetchdpi: a sha256sum.txt line for binaries/linux-x86_64/nfqws2 that disagrees with the pin → error
  ```
- [ ] **Step 2:** `go test ./internal/dpi/... ./tools/fetchdpi/` and `GOOS=windows go test -c ./internal/dpi/zapret2/` → FAIL.
- [ ] **Step 3: Implement.** `engine.Args` keeps its order and replaces the literal `--wf-*` lines with `interceptArgs(len(quic) > 0)`. `fetchdpi` also extracts `binaries/linux-x86_64/nfqws2` into `assets/zapret2/` and checks it against `sha256sum.txt` and the pin. Run `go run ./tools/fetchdpi` to fetch the file. `embed_linux.go`: `//go:embed nfqws2 lua/zapret-lib.lua lua/zapret-antidpi.lua lua/zapret-auto.lua`. Update `doc.go` (the Linux build embeds `nfqws2`, MIT).
- [ ] **Step 4:** tests above PASS; `GOOS=windows go test ./internal/dpi/...` compiles; `go run ./tools/sizecheck` (or its test) unaffected for Windows.
- [ ] **Step 5: Commit** `feat(dpi): one port table drives both WinDivert and nftables; Linux embeds the pinned nfqws2`

### Task 4: nftables rules from the port table

**Files:**
- Create: `internal/dpi/nftrules.go` (pure), `internal/dpi/nftrules_test.go`, `internal/dpi/nft_linux.go` (`google/nftables`), `internal/dpi/nft_root_test.go` (`//go:build linux && integration_root`)
- Modify: `go.mod`/`go.sum`, `tools/depcheck/rules.go` (+ test) if needed

**Interfaces:**
- Produces:
  ```go
  // nftrules.go — a small description the netlink code translates; tests compare it.
  type nftRule struct {
  	Chain   string // "post" | "pre"
  	Family  string // "ip" | "ip6"
  	Proto   string
  	Ports   []int
  	Packets int    // ct original (post) / ct reply (pre) packets 1-N
  	Action  string // "queue" | "ctmark" | "drop-ttl"
  }
  func nftPlan(f []CaptureRule) []nftRule
  var skipNets4, skipNets6 []netip.Prefix // loopback, RFC1918, link-local, CGNAT 100.64/10; ::1, fe80::/10, fc00::/7
  // nft_linux.go
  func installTable(f []CaptureRule) error // deletes an existing "ghostline" table first, then adds table+chains+rules in one batch
  func deleteTable() error                  // missing table → nil
  func tableExists() (bool, error)
  ```
- Rules in chain `post` per family: `meta mark & 0x40000000 != 0 → accept`; `oifname "lo" → accept`; `daddr @skip → accept`; per Filter rule `proto dport {ports} ct original packets 1-OutPackets queue num 200 bypass`; `meta mark & 0x40000000 != 0 ct mark set ct mark | 0x40000000`. Chain `pre`: per rule `proto sport {ports} ct reply packets 1-InPackets queue num 200 bypass`; `icmp(v6) time-exceeded ct mark & 0x40000000 != 0 drop`.

- [ ] **Step 1: Failing tests.**
  ```go
  func TestNftPlan_QueuesBothFamilies(t *testing.T) // 2 Filter rules × 2 families × (post queue + pre queue) present with Packets 20/10 (tcp) and 5/3 (udp), Ports {80,443}/{443}
  func TestNftPlan_SkipsLanAndOwnMark(t *testing.T)  // first post rules are the mark, lo and skip-net accepts, before any queue
  func TestNftPlan_TTLGuard(t *testing.T)           // ctmark rule in post; drop-ttl rule per family in pre
  ```
  Root test (`nft_root_test.go`, skips unless euid 0):
  ```go
  func TestNft_InstallListDelete(t *testing.T)        // install → `nft list table inet ghostline` contains "queue num 200 bypass" and "priority 99"; delete → table gone
  func TestNft_InstallReplacesExistingTable(t *testing.T) // install twice → one table, rule count unchanged (Review Focus 1)
  func TestNft_DeleteMissingIsNil(t *testing.T)
  ```
- [ ] **Step 2:** `go test ./internal/dpi/` → FAIL.
- [ ] **Step 3: Implement** `nftPlan` and the netlink translation with `github.com/google/nftables` (`expr.Meta`, `expr.Ct` with `CtKeyPKTS` + direction, `expr.Queue{Num:200, Flag: QueueFlagBypass}`, anonymous sets for ports and skip nets). `go get github.com/google/nftables@latest`; confirm depcheck forbids it on Windows.
- [ ] **Step 4:** unit tests PASS; `go vet -tags integration_root ./internal/dpi/`; `GOOS=windows go vet ./...`; depcheck ok.
- [ ] **Step 5: Commit** `feat(dpi): the nftables table for nfqws2 is built from the shared port table over netlink`

### Task 5: Linux interceptor, runner and engine directory

**Files:**
- Create: `internal/dpi/interceptor_linux.go` (`NewNftables() Interceptor`), `internal/dpi/kernel.go` (pure parsing), `internal/dpi/kernel_test.go`, `internal/dpi/runner_linux.go` (`NewLinuxRunner() Runner`), `internal/dpi/runner_linux_test.go`, `internal/dpi/dirperm_linux.go`
- Modify: `internal/dpi/manager.go` (`ErrKernelUnsupported`), `internal/app/dpi.go` (`dpiErr`), `internal/app/errors.go`, `internal/platform/platform_linux.go` (`DPIRunner`, `DPIInterceptor`, `Paths.BinDir`), i18n `en.json`/`vi.json`, `internal/dpi/nft_root_test.go`

**Interfaces:**
- Consumes: Task 4 `installTable`, `deleteTable`, `tableExists`; Task 3 `CaptureRule`, `zapret2.Filter`, `QueueNum`.
- Produces:
  ```go
  func NewNftables(filter []CaptureRule) Interceptor // platform passes zapret2.Filter
  var ErrKernelUnsupported = errors.New("dpi: kernel lacks nfqueue/nft_queue")
  func missingModules(sysModule func(name string) bool) []string // of "nfnetlink_queue","nft_queue","nf_conntrack"
  func queueBound(procNfq []byte, num int) bool                // /proc/net/netfilter/nfnetlink_queue: a line whose 1st field is num and 2nd (portid) != 0
  func prepareEngineDir(dir, autoHostlist string) error         // dir 0750 root:<nobody's gid>; autoHostlist (created if missing) owned by nobody
  const CodeDPIKernelUnsupported = "DPI_KERNEL_UNSUPPORTED"
  ```
- Linux `Prepare`: missing modules → `modprobe` each (`exec`), re-check, still missing → `ErrKernelUnsupported`; then `installTable(zapret2.Filter)`. `Ready(pid)`: `queueBound(read /proc/net/netfilter/nfnetlink_queue, 200)`. `Cleanup`: `deleteTable()`. `Info{Mechanism: "nftables inet ghostline, queue 200", AVExclusions: false}`.
- Linux runner: `exec.Cmd` with `SysProcAttr{Pdeathsig: SIGKILL, Setpgid: true}`, stdout/stderr to the daemon's log writer; `Process` like `winProc`.
- Manager on Linux calls `prepareEngineDir` after extraction (an `afterExtract func(dir string) error` hook set by the Linux constructor; nil on Windows).

- [ ] **Step 1: Failing tests.**
  ```go
  func TestMissingModules(t *testing.T) // only nft_queue absent → ["nft_queue"]
  func TestQueueBound(t *testing.T)     // fixture "  200  31950     0 2 65531     0     0       12  1\n" → true for 200; portid 0 → false; 201 → false
  func TestLinuxInterceptor_PrepareTwice(t *testing.T) // fake table funcs: second Prepare deletes then installs; no error (Review Focus 1)
  func TestLinuxRunner_ChildDiesWithParentGroup(t *testing.T) // start "sleep 30"; Kill → Exited() within 2 s
  func TestDPIErr_KernelUnsupported(t *testing.T) // app: dpi.ErrKernelUnsupported → code DPI_KERNEL_UNSUPPORTED
  ```
  Root test addition: `TestNfqws2_FailOpen` — Prepare, start the real embedded `nfqws2` with Linux args, `Ready` within 2 s, `kill -9`, then an HTTPS request to `https://example.com` succeeds; Cleanup removes the table.
- [ ] **Step 2:** FAIL.
- [ ] **Step 3: Implement**; wire `platform_linux.go`: `DPIRunner: dpi.NewLinuxRunner(log)`, `DPIInterceptor: dpi.NewNftables(zapret2.Filter)`, `paths.BinDir = filepath.Join(filepath.Dir(dataDir), "bin")`. vi/en strings for `errors.DPI_KERNEL_UNSUPPORTED` (message + action suggesting the proxy).
- [ ] **Step 4:** unit tests PASS; `go vet -tags integration_root ./internal/dpi/`; platform tests updated for BinDir.
- [ ] **Step 5: Commit** `feat(dpi): on Linux nfqws2 runs behind an nftables queue that fails open, with kernel checks and dropped privileges`

### Task 6: `Service.DPIInfo` and the DPI page

**Files:**
- Modify: `internal/app/service.go` (+ `ServiceDeps`), `internal/core/core.go`, `internal/rpc/client/service_gen.go` (generated), frontend `pages/Dpi.tsx`, `modes/simple/SimpleView.tsx`, `app/strategies.ts`, bindings, i18n, tests (`dpi*.test.tsx` and every mock of `Service`)

**Interfaces:**
- Produces: `type DPIInfo struct{ Engines []string \`json:"engines"\`; Mechanism string \`json:"mechanism"\`; AVExclusions bool \`json:"avExclusions"\` }`; `func (s *Service) DPIInfo() DPIInfo` (engines = installed engine IDs in display order, from `dpi.Manager`).
- Frontend: engine choices come from `DPIInfo().engines`; the antivirus-exclusion section renders only when `avExclusions`; a "Packet capture: {mechanism}" row.

- [ ] **Step 1: Failing tests.** Go: `TestService_DPIInfo` (fake manager with zapret2 only → `Engines == ["zapret2"]`, flags passed through). Frontend: `dpi-linux.test.tsx` — with `DPIInfo` `{engines:["zapret2"], avExclusions:false, mechanism:"nftables…"}` there is no GoodbyeDPI option and no AV section, and the mechanism text shows; with the Windows-shaped info both appear (existing tests).
- [ ] **Step 2:** FAIL.
- [ ] **Step 3: Implement**; `go generate ./...`; regenerate Wails bindings (`wails3 generate bindings -clean=true -ts -i`); add `DPIInfo` to frontend mocks.
- [ ] **Step 4:** `go test ./...`; `cd frontend && npx vitest run` → all pass.
- [ ] **Step 5: Commit** `feat(ui): the DPI page lists the engines and capture mechanism the backend reports`

### Task 7: Neutral proxy snapshot, `sysproxy.Backend`, state v5

**Files:**
- Create: `internal/model/proxy.go`, `internal/sysproxy/backend.go`, `internal/sysproxy/wininet.go` (Windows backend logic, unsuffixed, over `API`), `internal/sysproxy/bypass.go` (+ tests), `internal/store/statev5_test.go`
- Modify: `internal/sysproxy/sysproxy.go` (→ `wininet.go`), `api_windows.go`, `unsupported.go`, `watch_windows.go`, `internal/store/state.go`, `internal/app/deps.go`, `proxyphase.go`, `internal/watchdog/recover.go`, `internal/platform/platform.go`, `platform_windows.go`, `platform_linux.go`, `recovery.go`, `internal/core/core.go`, `internal/app/orchestrator.go` (`st.Version` 5), all affected tests

**Interfaces:**
- Produces:
  ```go
  // model
  type ProxySnapshot struct {
  	Backend string        `json:"backend"`
  	UID     int           `json:"uid,omitempty"`
  	Windows *WinINETProxy `json:"windows,omitempty"`
  	GNOME   *GNOMEProxy   `json:"gnome,omitempty"`
  	KDE     *KDEProxy     `json:"kde,omitempty"`
  }
  type WinINETProxy struct{ Flags uint32; Server, Bypass, AutoconfigURL string } // same JSON names as store.SysProxySnapshot
  type GNOMEProxy struct{ Values map[string]string `json:"values"` } // "schema key" → raw `gsettings get` text
  type KDEProxy struct{ Values map[string]string `json:"values"`; Present []string `json:"present"` } // keys that existed
  // sysproxy
  type Snapshot = model.ProxySnapshot
  type Info struct{ Desktop string `json:"desktop"`; Supported bool `json:"supported"` }
  var ErrNoSession = errors.New("sysproxy: no graphical session")
  var ErrDesktopUnsupported = errors.New("sysproxy: this desktop's proxy settings are not supported")
  type Backend interface {
  	Snapshot() (Snapshot, error)
  	Existing(Snapshot) (server, pac string, has bool)
  	Apply(addr string) error
  	IsOurs(addr string) (bool, error)
  	RestoreIfOurs(addr string, s Snapshot) (bool, error)
  	Watch(onChange func()) (stop func(), err error)
  	Info() Info
  }
  func NewWinINET(api API, watch func(func()) (func(), error)) Backend // Info{Desktop:"Windows",Supported:true}
  type DefaultBypass []string // localhost, 127.0.0.0/8, ::1, 10/8, 172.16/12, 192.168/16, <local>
  func BypassWinINET() string // == today's Bypass constant, byte for byte
  func BypassGNOME() string   // GVariant array text, e.g. "['localhost', '127.0.0.0/8', '::1', '10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16']"
  func BypassKDE() string     // "localhost,127.0.0.0/8,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
  ```
  `State.SysProxy.Snapshot *model.ProxySnapshot`; `CleanState().Version == 5`; loading a v4 file whose `sysproxy.snapshot` has `flags` migrates it into `{backend:"windows", windows:{…}}`. `app.SysProxy = sysproxy.Backend`. `platform.Deps.SysProxy sysproxy.Backend`; `WatchSysProxy` removed (core uses `SysProxy.Watch`). `watchdog.Deps.RestoreSysProxy func(ours string, s model.ProxySnapshot) (bool, error)`.

- [ ] **Step 1: Failing tests.**
  ```go
  func TestLoad_V4ProxySnapshotMigrates(t *testing.T) // v4 {"sysproxy":{"set":true,"ours":"127.0.0.1:8080","snapshot":{"flags":1,"server":"","bypass":"","autoconfigUrl":"http://pac"}}} → Snapshot.Backend "windows", Windows.AutoconfigURL "http://pac", Version 5
  func TestWrite_V5(t *testing.T)                     // written file has "version":5 and "backend":"windows"
  func TestBypass_WinINETUnchanged(t *testing.T)      // BypassWinINET() == the old constant (copy it into the test)
  func TestBypass_GNOMEAndKDE(t *testing.T)           // exact strings above
  ```
  Existing `sysproxy_test.go`, `proxyphase_test.go`, `watchdog/proxy_test.go`, `platform/recovery_test.go` move to the new types without changing what they assert.
- [ ] **Step 2:** FAIL.
- [ ] **Step 3: Implement**; the Windows backend is today's `Manager` with the snapshot wrapped/unwrapped at its edges.
- [ ] **Step 4:** `go test ./...`; `GOOS=windows go test -c -o /dev/null ./internal/sysproxy/ ./internal/app/`; `GOOS=windows go vet ./...`.
- [ ] **Step 5: Commit** `refactor(sysproxy): a backend-neutral proxy snapshot (state.json v5); WinINET becomes one backend`

### Task 8: `internal/session` — users, the agent protocol and the queue

**Files:**
- Create: `internal/session/session.go` (types, `Serve`, `Task`), `internal/session/run_linux.go` (spawn as user), `internal/session/logind.go` (logic over `logindAPI`), `internal/session/logind_linux.go` (godbus), `internal/session/queue.go`, tests `session_test.go` (with `TestMain` agent mode), `logind_test.go`, `queue_test.go`
- Modify: `cmd/ghostlined/args.go`, `cmd/ghostlined/main.go` (+ `args_test.go`), `internal/platform/platform.go` (`Sessions session.Sessions`, nil on Windows), `platform_linux.go`

**Interfaces:**
- Produces:
  ```go
  type User struct{ UID, GID int; Groups []int; Name, Home, Desktop string }
  type Task func(args json.RawMessage) (any, error)
  type StreamTask func(args json.RawMessage, emit func(any)) error // runs until stdin closes
  func Serve(in io.Reader, out io.Writer, tasks map[string]Task, streams map[string]StreamTask) int // agent side; unknown task → error reply, exit 2
  type Sessions interface {
  	Active() (User, bool)
  	ByUID(uid int) (User, bool) // a live session of uid (any seat), for restores
  	Run(u User, task string, in, out any) error
  	Stream(u User, task string, in any, onLine func(json.RawMessage)) (stop func(), err error)
  	WatchNew(onNew func(User)) (stop func(), err error)
  }
  type Queue struct{ /* path */ }
  func NewQueue(path string) *Queue
  func (q *Queue) Add(uid int, task string, args any) error
  func (q *Queue) Drain(u User, run func(task string, args json.RawMessage) error) error // runs u.UID's items in order; failed items stay
  func NewLinux(exe string, agentTasks []string, log *slog.Logger) (Sessions, error)
  var ErrAgent = errors.New("session: agent failed") // wraps the agent's {code,message}
  ```
- Agent wire: request line `{"task":"…","args":…}`; reply line `{"result":…}` or `{"error":{"code":"…","message":"…"}}`. Spawn: `exec.Command(exe, "--session-agent")`, `SysProcAttr.Credential{Uid,Gid,Groups}` (skipped when uid == current euid, for tests), env exactly `HOME USER LOGNAME PATH=/usr/local/bin:/usr/bin:/bin XDG_RUNTIME_DIR=/run/user/<uid> DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/<uid>/bus XDG_CURRENT_DESKTOP=<Desktop>`, timeout 10 s for `Run`.
- `Active`: logind seat0 `ActiveSession` → session `User` (uid), `Name`, `Type` ∈ {x11, wayland}, `Remote` false, `Class` "user", `Desktop`; home from `os/user.LookupId`. `WatchNew`: `SessionNew` signal from `org.freedesktop.login1` (match sender + owner check as in L3), debounced 1 s, resolving the session → `User`.
- `cmd/ghostlined --session-agent` calls `session.Serve(os.Stdin, os.Stdout, agentTasks(), agentStreams())`, where the maps are assembled in `cmd/ghostlined/agent.go` from Tasks 10 and 13 (empty in this task).

- [ ] **Step 1: Failing tests.**
  ```go
  func TestServe_RunsKnownTask(t *testing.T)        // in `{"task":"echo","args":{"x":1}}` → out `{"result":{"x":1}}`
  func TestServe_UnknownTaskRefused(t *testing.T)   // exit 2, error code "UNKNOWN_TASK"
  func TestRun_SpawnsAgentAndReturns(t *testing.T)  // Run(selfUser, "echo", …) via the test binary as agent (TestMain: GHOSTLINE_TEST_AGENT=1 → Serve)
  func TestRun_AgentErrorIsErrAgent(t *testing.T)
  func TestStream_LinesUntilStop(t *testing.T)      // "tick" stream emits 3 lines; stop ends the process
  func TestActive_PicksGraphicalLocalSession(t *testing.T) // fake logind: tty session → none; wayland local → user with Desktop "KDE"
  func TestQueue_DrainOnlyThatUIDAndKeepsFailures(t *testing.T)
  func TestArgs_SessionAgent(t *testing.T)          // cmd/ghostlined: "--session-agent" → Command "session-agent"
  ```
- [ ] **Step 2:** FAIL.
- [ ] **Step 3: Implement.** Queue file written via a temp file + rename, mode 0600.
- [ ] **Step 4:** `go test ./internal/session/ ./cmd/ghostlined/`; `GOOS=windows go vet ./...` (session has no Windows code; platform sets `Sessions` nil there).
- [ ] **Step 5: Commit** `feat(session): the daemon runs fixed tasks in a user's graphical session and queues them until the user logs in`

### Task 9: Dependency and size gate

- [ ] **Step 1:** `go run ./tools/depcheck`; build the Windows exe and run `tools/sizecheck` against the v0.5.1 baseline as in `release.yml`. Expected: depcheck ok; growth ≤ 2%.
- [ ] **Step 2:** If a rule is missing (nftables, session), add it with a test in `tools/depcheck`; commit `chore(depcheck): keep nftables and the session agent out of the Windows build`. If nothing changed, record "no change" in the ledger and skip the commit.

### Task 10: GNOME and KDE proxy, agent side

**Files:**
- Create: `internal/sysproxy/desktop.go` (agent tasks, `desktopFor`), `internal/sysproxy/gnome.go`, `internal/sysproxy/kde.go`, `internal/sysproxy/kde_linux.go` (D-Bus reparse signal), tests `gnome_test.go`, `kde_test.go`
- Modify: `cmd/ghostlined/agent.go`

**Interfaces:**
- Produces:
  ```go
  type runner func(name string, args ...string) ([]byte, error) // fake in tests
  type desktop interface {
  	snapshot() (model.ProxySnapshot, error)
  	apply(addr string) error
  	isOurs(addr string) (bool, error)
  	restore(s model.ProxySnapshot) error
  	watch(emit func()) (stop func(), err error)
  }
  func desktopFor(name string, run runner, home string) (desktop, bool) // "KDE" → kde; GNOME, ubuntu:GNOME, X-Cinnamon, Cinnamon, Budgie:GNOME, Unity → gnome; else false
  func AgentTasks() map[string]session.Task         // "proxy.snapshot","proxy.apply","proxy.isOurs","proxy.restore"
  func AgentStreams() map[string]session.StreamTask // "proxy.watch"
  ```
- GNOME keys (snapshot/restore all of them): `org.gnome.system.proxy` {`mode`,`autoconfig-url`,`ignore-hosts`,`use-same-proxy`}, `org.gnome.system.proxy.http` {`host`,`port`,`enabled`}, `.https` {`host`,`port`}, `.socks` {`host`,`port`}, `.ftp` {`host`,`port`}. Apply: `mode 'manual'`, http/https host `'127.0.0.1'` + port, `ignore-hosts BypassGNOME()`. `isOurs`: mode manual and http host/port equal. Watch: `gsettings monitor org.gnome.system.proxy` and `…proxy.http` (two processes), each output line → emit.
- KDE: snapshot parses `~/.config/kioslaverc` group `[Proxy Settings]` keys `ProxyType`, `httpProxy`, `httpsProxy`, `ftpProxy`, `socksProxy`, `NoProxyFor`, `Proxy Config Script`, `ReversedException` (records `Present`). Apply via `kwriteconfig6` (fallback `kwriteconfig5`) `--file kioslaverc --group "Proxy Settings" --key K V`: `ProxyType 1`, `httpProxy "http://127.0.0.1 8080"`, `httpsProxy` same, `NoProxyFor BypassKDE()`, `ReversedException false`; then the reparse signal `org.kde.KIO.Scheduler.reparseSlaveConfiguration("")` on path `/KIO/Scheduler`. Restore: keys in `Present` set back, others `--delete`. `isOurs`: ProxyType 1 and httpProxy equals. Watch: inotify on `~/.config/kioslaverc` (directory watch, debounced 1 s).

- [ ] **Step 1: Failing tests** (fake runner records commands; temp HOME):
  ```go
  func TestGNOME_ApplySetsManualAndHosts(t *testing.T)
  func TestGNOME_RoundTripRestoresEveryKey(t *testing.T) // snapshot → apply → restore → `gsettings set` calls reproduce the snapshot's raw values
  func TestKDE_ApplyWritesKeysAndSignals(t *testing.T)   // httpProxy "http://127.0.0.1 8080"; reparse called once
  func TestKDE_RestoreAbsentDeletesKeys(t *testing.T)   // no kioslaverc → restore issues --delete for every key it wrote (Review Focus 5)
  func TestDesktopFor(t *testing.T)                     // table of desktop names
  ```
- [ ] **Step 2:** FAIL. **Step 3:** implement; register `AgentTasks/AgentStreams` in `cmd/ghostlined/agent.go`. **Step 4:** PASS + Windows vet.
- [ ] **Step 5: Commit** `feat(sysproxy): GNOME and KDE proxy settings, set and restored inside the user's session`

### Task 11: Linux proxy backend, deferred apply and restore

**Files:**
- Create: `internal/sysproxy/linux.go` (daemon side), `internal/sysproxy/linux_test.go`
- Modify: `internal/app/proxyphase.go` (+ test), `internal/app/errors.go`, `internal/core/core.go` (WatchNew → drain + reapply), `internal/platform/platform_linux.go`, `internal/app/service.go` (`SysProxyInfo`), generated proxy, bindings, frontend `pages/Proxy.tsx` (+ test), i18n

**Interfaces:**
- Consumes: Task 8 `Sessions`, `Queue`; Task 10 task names; Task 7 `Backend`.
- Produces:
  ```go
  func NewLinux(s session.Sessions, q *session.Queue) Backend
  func DrainQueued(s session.Sessions, q *session.Queue, u session.User) error // runs queued proxy.restore items
  // internal/app/errors.go
  const CodeSessionPending = "SESSION_PENDING"
  const CodeProxyDesktopUnsupported = "PROXY_DESKTOP_UNSUPPORTED"
  func (s *Service) SysProxyInfo() sysproxy.Info
  ```
- `Snapshot`: no active user → `ErrNoSession`; desktop unsupported → error wrapping `ErrDesktopUnsupported`; else `Run(u,"proxy.snapshot")` with `UID = u.UID`, `Backend` = "gnome"/"kde".
- `RestoreIfOurs(addr, s)`: user = `ByUID(s.UID)`; none → `Queue.Add(s.UID,"proxy.restore",{addr,snapshot})` and return `(true, nil)`; else `Run(u,"proxy.restore")` only when `proxy.isOurs` says so.
- `Watch`: `Stream(Active(),"proxy.watch")`, restarted on `WatchNew`; no active user → nothing until one appears.
- App: in `startProxyPhase`, `errors.Is(err, sysproxy.ErrNoSession)` → skip the system proxy for this run, add warning `SESSION_PENDING`, no failure; `ErrDesktopUnsupported` → `PROXY_DESKTOP_UNSUPPORTED` (warning, proxy still runs). Core: `Sessions.WatchNew(u)` → `DrainQueued` for `u` then `Orch.ReapplyProxy` if `SESSION_PENDING` is set; at start, drain for `Active()` if any.

- [ ] **Step 1: Failing tests** (fake Sessions):
  ```go
  func TestLinuxProxy_SnapshotNoSession(t *testing.T)        // ErrNoSession
  func TestLinuxProxy_RestoreUsesRecordedUID(t *testing.T)   // snapshot UID 1000, active is 1001 → Run called with uid 1000 (Review Focus 2)
  func TestLinuxProxy_RestoreQueuesWhenUserAway(t *testing.T) // ByUID false → queue has proxy.restore for 1000; returns true
  func TestProxyPhase_NoSessionWarnsAndConnects(t *testing.T) // app: Connect succeeds, warning SESSION_PENDING, no SYSPROXY_FAILED
  func TestCore_NewSessionDrainsAndReapplies(t *testing.T)
  ```
  Frontend: `proxy-linux.test.tsx` — `SysProxyInfo {desktop:"", supported:false}` shows the manual-setup hint; `{desktop:"KDE"}` shows "KDE".
- [ ] **Step 2:** FAIL. **Step 3:** implement; `go generate ./...`; bindings; mocks; vi/en strings for both codes. **Step 4:** PASS (Go + vitest + Windows vet).
- [ ] **Step 5: Commit** `feat(linux): the system proxy follows the graphical session, waits for login and is restored for the user it was set for`

### Task 12: Linux certificate store — system anchors and Firefox policy

**Files:**
- Create: `internal/certstore/linux.go` (composite, `Target`, `ErrPartial`), `internal/certstore/anchors.go`, `internal/certstore/firefox.go`, tests `linux_test.go`, `anchors_test.go`, `firefox_test.go`
- Modify: `internal/core/certwire.go` (ErrPartial → warning), `internal/app/errors.go`, `internal/platform/platform_linux.go`, i18n

**Interfaces:**
- Produces:
  ```go
  type Target interface {
  	Name() string
  	Install(der []byte) error
  	Remove(thumbprint string) error
  	List(prefix string) ([]Cert, error)
  }
  type ErrPartial struct{ Targets []string; Err error } // Error(), Unwrap()
  func NewLinux(required Target, optional ...Target) Store // Install: required error → error; optional errors → *ErrPartial; List: union by thumbprint
  func NewAnchors(dir, tool string, run func(string, ...string) ([]byte, error)) Target // file ghostline-<thumb>.crt (PEM, 0644), then run(tool)
  func DetectAnchors(run …) (Target, error) // update-ca-certificates → /usr/local/share/ca-certificates; update-ca-trust + /etc/pki/ca-trust/source/anchors → that; update-ca-trust → /etc/ca-certificates/trust-source/anchors
  func NewFirefox(policyPath, certDir string) Target // certDir: where CA files the policy points to live = the anchors dir
  func FirefoxInstalled() bool // /usr/lib/firefox, /usr/lib64/firefox, /usr/lib/firefox-esr, /snap/firefox
  const CodeCertPartial = "CERT_PARTIAL"
  ```
- Firefox: read `policies.json` (missing → `{"policies":{}}` and remember "we created it"); add the CA path to `policies.Certificates.Install` (array, no duplicates); keep every other key verbatim (`map[string]json.RawMessage` at each level touched); write temp + rename (0644, dir 0755). Remove: drop our path; if Ghostline created the file (recorded in the sidecar `<DataDir>/firefox-policy.json` when it did) and only an empty `policies` object remains → delete the file and the sidecar. Malformed JSON → error, file untouched.
- certwire: `InstallSession` returning `*ErrPartial` → success plus warning `CERT_PARTIAL` with param `target`.

- [ ] **Step 1: Failing tests.**
  ```go
  func TestAnchors_InstallListRemove(t *testing.T)     // temp dir; file name; PEM; tool run after each change; List parses CN
  func TestAnchors_IgnoresOtherFiles(t *testing.T)
  func TestFirefox_CreatesAndDeletesOwnFile(t *testing.T)
  func TestFirefox_KeepsOtherEntries(t *testing.T)     // existing DisableTelemetry + admin's own Install path kept through install+remove (Review Focus 3)
  func TestFirefox_MalformedPolicyUntouched(t *testing.T) // "{bad" → error, bytes unchanged (Review Focus 3)
  func TestLinuxStore_OptionalFailureIsPartial(t *testing.T)
  func TestLinuxStore_RequiredFailureFails(t *testing.T)
  func TestCertWire_PartialWarns(t *testing.T)
  ```
- [ ] **Step 2:** FAIL. **Step 3:** implement; platform: `Certs: certstore.NewLinux(anchors, firefox-if-installed, nss)` (nss added in Task 13). **Step 4:** PASS + Windows vet.
- [ ] **Step 5: Commit** `feat(certstore): Fake SNI roots go into the system anchors and Firefox's policy on Linux`

### Task 13: User NSS target via the session agent

**Files:**
- Create: `internal/certstore/nss.go` (daemon side), `internal/certstore/nssagent.go` (agent tasks), tests `nss_test.go`
- Modify: `cmd/ghostlined/agent.go`, `internal/core/core.go` (WatchNew also drains `nss.remove`), `internal/platform/platform_linux.go`, `internal/app/errors.go`, i18n

**Interfaces:**
- Produces:
  ```go
  func NewNSS(s session.Sessions, q *session.Queue, usersPath string, p11kit func() bool) Target
  func P11KitTrust() bool // libnssckbi.so resolves to p11-kit-trust.so in /usr/lib*/{,x86_64-linux-gnu/}{,nss/}
  func NSSAgentTasks() map[string]session.Task // "nss.install","nss.remove","nss.list": certutil -d sql:$HOME/.pki/nssdb -A -t "C,," -n "<SessionPrefix> <thumb>" -i <tmpfile>; -D -n …; -L
  var ErrNSSToolMissing = errors.New("certstore: certutil not found")
  const CodeCertNSSToolMissing = "CERT_NSS_TOOL_MISSING"
  ```
- Install: `p11kit()` → no-op; no active user or no `~/.pki/nssdb` → no-op; else `Run(u,"nss.install")`, record uid → thumbprints in `nss-users.json`. Remove: for each recorded uid, `ByUID` → `Run(nss.remove)` or `Queue.Add(uid,"nss.remove",thumb)`. Agent returns code `CERT_NSS_TOOL_MISSING` when `certutil` is absent → daemon maps to `ErrNSSToolMissing` inside `ErrPartial`.

- [ ] **Step 1: Failing tests.**
  ```go
  func TestNSS_SkippedWithP11Kit(t *testing.T)
  func TestNSS_InstallRecordsUID(t *testing.T)
  func TestNSS_RemoveQueuesWhenAway(t *testing.T)
  func TestNSSAgent_CertutilArgs(t *testing.T)     // fake runner: exact -A/-D argv
  func TestNSSAgent_MissingTool(t *testing.T)      // code CERT_NSS_TOOL_MISSING
  func TestCertWire_NSSMissingWarns(t *testing.T)  // warning CERT_NSS_TOOL_MISSING
  ```
- [ ] **Step 2:** FAIL. **Step 3:** implement; register agent tasks; strings. **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat(certstore): Chrome's per-user NSS database gets the Fake SNI root where the system store does not reach it`

### Task 14: Linux firewall — firewalld, ufw, none

**Files:**
- Create: `internal/firewall/linux.go` (sidecar + dispatch), `internal/firewall/firewalld.go` (logic over `firewalldAPI`), `internal/firewall/firewalld_linux.go` (godbus), `internal/firewall/ufw.go`, `internal/firewall/nftpolicy_linux.go` (input-drop check), tests `linux_test.go`, `firewalld_test.go`, `ufw_test.go`
- Modify: `internal/platform/platform_linux.go`, `internal/app/proxyphase.go`/`dnsphase.go` (warning), `internal/app/errors.go`, i18n

**Interfaces:**
- Produces:
  ```go
  type linuxRule struct{ Zone, Proto string; Ports []int; Owned bool } // persisted by name in firewall-rules.json
  func DetectLinux(dataDir string) Manager // firewalld → ufw → none
  func newLinux(path string, b backend) Manager
  type backend interface {
  	open(zone, proto string, port int) (owned bool, err error) // owned=false: already open before Ghostline
  	close(zone, proto string, port int) error
  	zone() (string, error) // firewalld: zone of the default-route interface (empty → default zone); ufw/none: ""
  	public() (bool, error)
  }
  func InputDropElsewhere() (bool, error) // an nft chain hooked at input with policy drop in a table other than ghostline
  const CodeFirewallUnknown = "FIREWALL_UNKNOWN"
  ```
- `Add(port)` = `AddNamed(Rule{Name: ProxyRule, Protocol: "TCP", Ports: {port}})`; `Delete()` = `DeleteNamed(ProxyRule)`. `AddNamed`: write the sidecar entry first (temp + rename), then open each port. `DeleteNamed`: close only `Owned` ports, then drop the entry; unknown name → nil. `Block` rules → nil. `IsPublicNetwork` → `backend.public()`.
- firewalld (D-Bus `org.fedoraproject.FirewallD1`, path `/org/fedoraproject/FirewallD1`): `zone.queryPort` (already open → owned=false), `zone.addPort(zone, port, proto, 0)`, `zone.removePort`, `zone.getZoneOfInterface(iface)`; public zones `public`,`external`,`block`,`drop`.
- ufw (exec `ufw`): `ufw show added` contains `ufw allow <port>/<proto>` → owned=false; else `ufw allow <port>/<proto> comment 'ghostline: <name>'`; close `ufw delete allow <port>/<proto>`. Detected when `ufw status` first line is `Status: active`.
- none: open/close no-op (owned=false); app shows `FIREWALL_UNKNOWN` when `InputDropElsewhere()` is true and LAN sharing is on.

- [ ] **Step 1: Failing tests.**
  ```go
  func TestLinuxFirewall_SidecarSurvivesRestart(t *testing.T) // AddNamed then a new Manager on the same path → DeleteNamed closes the same ports
  func TestUfw_AddDelete(t *testing.T)                          // exact argv
  func TestUfw_PreexistingRuleNotRemoved(t *testing.T)          // (Review Focus 4)
  func TestFirewalld_ZoneAndPublic(t *testing.T)                // fake API: iface zone "public" → public true; addPort(zone, "8080","tcp",0)
  func TestLinuxFirewall_BlockRuleIsNoop(t *testing.T)
  ```
- [ ] **Step 2:** FAIL. **Step 3:** implement; wire `Firewall: firewall.DetectLinux(paths.DataDir)`; strings. **Step 4:** PASS + Windows vet.
- [ ] **Step 5: Commit** `feat(firewall): LAN sharing opens ports through firewalld or ufw on Linux and closes exactly what it opened`

### Task 15: Linux network identity and SSID

**Files:**
- Create: `internal/netid/procnet.go` (pure), `internal/netid/linux.go` (logic over `nmAPI`), `internal/netid/nm_linux.go` (godbus), tests `procnet_test.go`, `linux_test.go`
- Modify: `internal/platform/platform_linux.go`, `docs/platforms.md`

**Interfaces:**
- Produces:
  ```go
  func defaultRoute(procRoute []byte) (iface string, gw netip.Addr, ok bool) // lowest metric "00000000" destination
  func arpMAC(procArp []byte, ip netip.Addr) (string, bool)                 // flags 0x2 (complete), lower-case "aa:bb:…"
  func NewLinux(nm nmAPI) Source // nm nil → SSID functions return ErrUnsupported
  type nmAPI interface{ ActiveSSID() (string, error); SavedSSIDs() ([]string, error) }
  ```
- `NetworkKey`: route → gateway; ARP lookup; missing → send one UDP datagram to `gw:9`, wait 200 ms, look again; still missing → the interface's own MAC; `scanner.NetworkKey(gw.String(), mac)`; no default route → `scanner.NetworkKey("none","none")` (as `Unsupported`). `LiveAdapters`: one entry per default-route interface, `Gateway` set, `DNS` nil (ISP resolvers come from the DNS snapshot on Linux; verify `core.ispResolvers` falls back to `st.DNS.Servers()` and add that fallback if it does not).
- NM: devices with `DeviceType == 2` and `State == 100` → `Wireless.ActiveAccessPoint` → `Ssid` (bytes → string); saved: `Settings.ListConnections` → `GetSettings()["802-11-wireless"]["ssid"]`.

- [ ] **Step 1: Failing tests.** `TestDefaultRoute` (fixture with two defaults, metric picks), `TestArpMAC` (incomplete entry ignored), `TestNetworkKey_FallsBackToOwnMAC`, `TestSSID_FromFakeNM`, `TestSSID_NoNM`, `TestISPResolvers_LinuxFallback` (core).
- [ ] **Step 2:** FAIL. **Step 3:** implement; wire `NetID: netid.NewLinux(nm)`. **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat(netid): network key from the default route and ARP, SSID and saved Wi-Fi from NetworkManager on Linux`

### Task 16: Platform in the snapshot and Linux string overrides

**Files:**
- Create: `frontend/src/i18n/en.linux.json`, `frontend/src/i18n/vi.linux.json`, `frontend/src/i18n/platform.test.ts`
- Modify: `internal/platform/platform.go` (`Name string`), `platform_windows.go` ("windows"), `platform_linux.go` ("linux"), `internal/app/status.go` (`Snapshot.Platform`), `internal/app/orchestrator.go` or `ServiceDeps` (set once), `internal/core/core.go`, `frontend/src/i18n/index.ts` (`applyPlatform`), the app root that first receives the snapshot, `frontend/src/i18n/parity.test.ts`

**Interfaces:**
- Produces: `Snapshot.Platform string \`json:"platform"\``; `export function applyPlatform(p: string): void` — for `"linux"`, `i18n.addResourceBundle(lang, "translation", linuxBundle[lang], true, true)` for both languages, once.
- Overrides: every key in `en.json` whose English text mentions Windows, Defender, WinDivert, svchost, Hotspot, WSL, "Public network", "Windows certificate store", "Windows proxy" or "Run as administrator" (find with `grep -n -iE 'windows|windivert|defender|svchost|hotspot|wsl|public network|administrator' frontend/src/i18n/en.json`), rewritten for Linux in both languages.

- [ ] **Step 1: Failing tests.**
  ```ts
  test("linux overrides replace Windows wording")         // applyPlatform("linux") → t(<a known key>) has no "Windows"
  test("windows keeps base strings")                      // no call / "windows" → unchanged
  test("linux bundles have the same keys in vi and en")   // parity
  test("every linux key exists in the base bundle")
  test("no linux override mentions Windows")              // scan values
  ```
  Go: `TestSnapshot_PlatformFromDeps`.
- [ ] **Step 2:** FAIL. **Step 3:** implement; call `applyPlatform(snapshot.platform)` where the first snapshot arrives. **Step 4:** Go + vitest PASS.
- [ ] **Step 5: Commit** `feat(ui): Linux wording replaces Windows-specific strings, chosen by the platform the backend reports`

### Task 17: Recovery wiring, root integration, docs, manual check

**Files:**
- Create: `internal/platform/recovery_linux_test.go`, `<scratchpad>/root/l4run.sh` (not committed)
- Modify: `internal/platform/recovery.go` (proxy restore via `SysProxy.RestoreIfOurs`, already; DPI cleanup via `NewDPIManager(...).Stop`, already), `internal/headless/headless.go` if needed, `.github/workflows/ci.yml` (root step runs `./internal/dpi/` too, and `modprobe nfnetlink_queue nft_queue` first), `docs/platforms.md` (rows: DPI runner and filter, DPI engine files, System proxy, Cert store, Firewall, Network key and Wi-Fi name, Session agent), spec (rulings)

- [ ] **Step 1: Failing test** `TestRecovery_LinuxCleansEverything` (fakes for interceptor, proxy backend, cert store, firewall): an orphaned state with DPI running, sysproxy set, a session cert and a firewall rule → after `watchdog.RunRestore`: interceptor `Cleanup` called, proxy `RestoreIfOurs` called with the recorded snapshot, cert removed, firewall rule deleted, state clean — in that order before DNS.
- [ ] **Step 2:** FAIL (or PASS if wiring already covers it; then keep the test as a guard and note it in the ledger). **Step 3:** fix wiring. **Step 4:** PASS.
- [ ] **Step 5:** CI step: `sudo modprobe nfnetlink_queue nft_queue || true` then `sudo -E env "PATH=$PATH" go test -tags integration_root ./internal/sysdns/ ./internal/dpi/ ./cmd/ghostlined/`. `actionlint` clean.
- [ ] **Step 6:** Write `l4run.sh` (root, run by the user): records before-state (`nft list tables`, `~harry/.config/kioslaverc` `[Proxy Settings]`, anchors dir listing, `/etc/firefox/policies/policies.json`, `ufw status numbered`, NM SSID); runs the root tests; starts `ghostlined --daemon --data-dir <tmp>`; as the user: connect with zapret2 on, system proxy on, Fake SNI on, LAN sharing on; checks table present, a blocked-site fetch, KDE proxy keys, anchor file, policy entry, ufw rule, SSID in status; after disconnect, asks the user to open Firefox's certificate manager and say whether a "Ghostline Fake SNI" authority is still listed (spec §13 risk), and records the answer in the spec; `kill -9 nfqws2` and fetches again; disconnects; `--restore` in a trap; diff before/after (must be empty). Ask the user to run it; fix anything it finds with a failing test first.
- [ ] **Step 7: Commit** `test(linux): L4 recovery guard and root integration for nftables; docs and spec rulings`
