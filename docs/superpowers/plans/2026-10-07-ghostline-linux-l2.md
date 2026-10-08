# Ghostline Linux L2 (Daemon + GUI Client) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On Linux, a root daemon `ghostlined` runs Ghostline, and the GTK GUI runs as the user and drives it over a Unix socket. Both the daemon and the Windows GUI run the same Wails-free `internal/core`.

**Architecture:**
- `shell.Run` is split in two:
  - `internal/core` builds the orchestrator, `app.Service`, the Bus and the background loops.
  - `shell` keeps only the window and the tray.
- On Windows, `shell` runs `core` in-process, as today.
- On Linux:
  - `cmd/ghostlined` serves `core`'s `*app.Service` through `internal/rpc` (JSON lines over `/run/ghostline/ctl.sock`).
  - The GUI binds a generated proxy, `internal/rpc/client.Service`. The proxy registers the same Wails binding IDs as `app.Service`, so the frontend is unchanged.
- File dialogs run in the GUI. The daemon reaches them through reverse `ui` calls tied to the calling connection.

**Tech Stack:** Go 1.26 (stdlib `net`, `encoding/json`, `reflect`, `hash/fnv`, `golang.org/x/sys/unix`), Wails v3 beta.27 (`application.RegisterBindingMethodID`), React + Vitest for one frontend change.

**Spec:** `docs/superpowers/specs/2026-10-07-ghostline-linux-l2-design.md` (§ numbers below refer to it). It builds on `docs/superpowers/specs/2026-10-07-ghostline-linux-design.md`.

## Global Constraints

- **Windows behaves exactly as on `feat/linux`:**
  - The startup order is unchanged: log, settings, state, DPI manager, startup restore, picker, bus, engine, orchestrator, service, rules; then the window, the tray and the background loops.
  - Every existing Windows test passes.
- **No new Go module dependencies.**
- **Frontend and bindings:**
  - The TS bindings stay as they are, with one exception: the new `LANDNSClients()` method.
  - The only frontend change is `ConnectError.tsx` plus four i18n keys (Task 12).
- **Packages that must not import Wails, `winutil` or `golang.org/x/sys/windows`:** `internal/core`, `internal/headless`, `internal/rpc` and `cmd/ghostlined`. depcheck enforces this from Task 2 / Task 10.
- **`internal/rpc` never imports `internal/app`.** Only `internal/rpc/client` knows app types.
- **Protocol:**
  - `rpc.Protocol = 1`.
  - `rpc.MaxLine = 16 << 20` (see the ruling below).
  - `rpc.UITimeout = 10 * time.Minute`.
- **Peers allowed to use the socket:** uid 0, or a member of `wheel`, `sudo`, `admin` or `ghostline`. `--allow-uid` exists for dev and tests only.
- **Linux daemon directories** (only `platform.New` and `platform.NewDev` choose them):
  - data: `/var/lib/ghostline/data`
  - logs: `/var/log/ghostline`
  - runtime: `/run/ghostline` (`ctl.sock`, `state.lock`)
- **Error codes:** `DAEMON_UNREACHABLE`, `DAEMON_PROTOCOL_MISMATCH`, `NOT_AUTHORIZED`, `NO_UI`. The same string literals appear in `internal/app/errors.go` and `internal/rpc/proto.go`; Task 9 has a test that pins them equal.
- **Commits:**
  - Style is `type(scope): sentence`, on branch `feat/linux-l2`.
  - **Never** add a `Co-Authored-By` trailer or a "Generated with" line.
- **Verify after every task:**
  - `go test ./...` (Linux, CGO on)
  - `GOOS=windows go vet ./...`
  - `go run ./tools/depcheck`
  - Commands with redirection or pipes go in the plan's scratch workspace; do not wrap test commands in `sh -c`.

**Ruling (spec §3, §5 "4 MiB"):** `MaxLine` is **16 MiB**, not 4 MiB. A backup may be up to `backup.MaxSize` = 8 MiB, and the `openFile`/`saveFile` reverse calls carry it base64-encoded (about 10.7 MiB). Task 13 updates the spec line.

## Review Focus

1. **Windows startup order after the core split.**
   - Expected: a dirty `state.json` from a crashed run is restored before the orchestrator exists, and the restore warning shows in the first snapshot.
   - Test: Task 2, `TestNew_RestoresOrphanedStateFirst`.
2. **A stale `ctl.sock` left by a killed daemon, or a second daemon started by mistake.**
   - Expected: the new daemon replaces a dead socket, and refuses to start while a live daemon answers on it.
   - Tests: Task 10, `TestListen_ReplacesStaleSocket` and `TestListen_RefusesWhenLive`.
3. **The daemon restarts (systemd `Restart=on-failure`) while the GUI is open.**
   - Expected: the next call reconnects, and events flow again.
   - Test: Task 9, `TestService_ReconnectsAfterDaemonRestart`.
4. **Importing a backup close to 8 MiB, or a huge non-backup file.**
   - Expected: an 8 MiB file crosses the socket intact; a larger file is refused in the GUI before it is sent.
   - Tests: Task 6, `TestLargeMessageWithinMaxLine`, and Task 11, `TestOpenFileHandler_RejectsOversize`.
5. **An event whose JSON does not decode into the type the frontend expects.**
   - Expected: every one of the 16 typed events re-emitted by the GUI has the Go type registered with Wails.
   - Test: Task 11, `TestDecodeEvent_CoversEveryRegisteredEvent`.

---

### Task 1: `app`: dialog methods take `ctx` and get file contents; `LANDNSClients`

**Files:**
- Modify:
  - `internal/app/service.go`: the `ServiceDeps.OpenFile`, `ServiceDeps.SaveFile` and new `ServiceDeps.LANDNSClients` fields, and the new `LANDNSClients` method.
  - `internal/app/backupservice.go:107-150`: `ExportSettings`, `PreviewImport`.
  - `internal/app/advservice.go:239`: `ExportAdvancedCSV`.
  - `internal/app/dnsservice.go:104`: `SaveDeviceFiles`.
  - The tests that set `OpenFile`/`SaveFile`.
  - `internal/shell/shell.go`: the Windows dialog wiring and `LANDNSClients`.

**Interfaces:**
- Produces:
  ```go
  // ServiceDeps
  OpenFile      func(ctx context.Context, title string) (name string, data []byte, err error) // name "" = cancelled
  SaveFile      func(ctx context.Context, name string, data []byte) error
  LANDNSClients func() int // LAN devices that used the DNS server in the last 10 minutes; 0 while it is off
  // Service methods (Wails omits a leading ctx in the TS bindings)
  func (s *Service) ExportSettings(ctx context.Context, sections []string) error
  func (s *Service) PreviewImport(ctx context.Context) (ImportPreview, error)
  func (s *Service) ExportAdvancedCSV(ctx context.Context) error
  func (s *Service) SaveDeviceFiles(ctx context.Context) error
  func (s *Service) LANDNSClients() int // 0 when the dep is nil
  ```
- `PreviewImport` validates `data` exactly as it validates the file today: size `> backup.MaxSize` gives `IMPORT_INVALID`, and parsing is unchanged. It never opens a path itself.
- Windows wiring in `shell.go`:
  - `OpenFile`: the existing dialog with the same title and filter. It reads the chosen path through `io.LimitReader(f, backup.MaxSize+1)` and returns `filepath.Base(path), data`.
  - `SaveFile`: the existing dialog, then writes the file with mode `0o644`.
  - `LANDNSClients`: the current `ui.lanDNSClients` closure body.

- [ ] **Step 1: Write the failing tests** in `internal/app/backupservice_test.go`:
  ```go
  func TestPreviewImport_ReadsContentFromOpenFile(t *testing.T) // OpenFile returns ("x.ghostline.json", <valid export bytes>) → preview has the same sections as before this change
  func TestPreviewImport_CancelledIsNoError(t *testing.T)        // OpenFile returns ("", nil, nil) → ImportPreview{} and nil
  func TestPreviewImport_TooLargeIsInvalid(t *testing.T)         // data of backup.MaxSize+1 bytes → error whose message starts with "IMPORT_INVALID"
  func TestLANDNSClients_NilDepIsZero(t *testing.T)              // NewService with ServiceDeps{} → LANDNSClients() == 0
  ```
  Update every existing caller in the tests to the new signatures, passing `context.Background()`.
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/app/ -run 'TestPreviewImport|TestLANDNSClients'` → build failure (new signatures).
- [ ] **Step 3: Implement** the signatures above and the Windows wiring.
- [ ] **Step 4: Run and verify.**
  - `go test ./internal/app/ ./internal/shell/` → PASS.
  - `GOOS=windows go vet ./...` → no output.
  - Run `wails3 generate bindings -clean=true -ts -i`, then `git diff --stat -- frontend/bindings` (the directory is ignored; compare against a copy saved before the step). The only change should be the new `LANDNSClients` function; the four ctx methods keep their TS parameter lists.
- [ ] **Step 5: Commit.** `refactor(app): file dialogs take ctx and hand over contents, not paths; LANDNSClients for the tray`

### Task 2: `internal/core`: the Wails-free half of `shell.Run`

**Files:**
- Create: `internal/core/core.go`, `internal/core/log.go`, `internal/core/core_test.go`
- Move with `git mv` from `internal/shell` to `internal/core`, changing the package clause and nothing else unless something must become exported:
  - `catalog.go`, `catalog_test.go`
  - `strategies.go`, `strategies_test.go`
  - `proxywire.go`
  - `certwire.go`, `certwire_test.go`
  - `dnswire.go`, `dnswire_test.go`
  - `toolswire.go`, `toolswire_test.go`
  - `updates.go`, `updates_test.go`
  - `checkupdate.go`, `checkupdate_test.go`
  - `system.go`, `safety.go`
- Move into `internal/core/core.go`: `restoreNow` from `shell.go`, and `builderFunc` plus its helper types from `ui.go`.
- Modify: `internal/shell/shell.go`, which now calls `core`; `stopdpi.go`, `headless.go`; and `tools/depcheck/rules.go`, which gains two rules.

**Interfaces:**
- Produces:
  ```go
  package core
  type Options struct {
      Platform platform.Deps
      Log      *slog.Logger
      Emitter  app.Emitter // receives every UI event (Wails on Windows, the RPC server on Linux)
      // GUI hooks; nil in the daemon.
      SetMode           func(mode string)
      OnSettingsChanged func(old, n store.Settings) // called after core's own handling (autostart, bootstrap)
      OnUpdate          func(tag, url string)
      OpenFile          func(ctx context.Context, title string) (name string, data []byte, err error)
      SaveFile          func(ctx context.Context, name string, data []byte) error
  }
  type Core struct {
      Orch     *app.Orchestrator
      Svc      *app.Service
      Bus      *app.Bus
      Settings *app.SettingsBox
      // unexported: platform, paths, log, eng, pw, picker, cat, strats, checker
  }
  func OpenLog(paths store.Paths) (*slog.Logger, io.Closer, error) // MkdirAll DataDir + logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3)
  func New(o Options) (*Core, error)  // everything shell.Run did up to and including app.LoadRules, in the same order
  func (c *Core) Run(ctx context.Context) // proxy and network watches, the 3 one-second tickers, runLists, runUpdates; returns when ctx is done, after stopping the watches
  func (c *Core) Shutdown(ctx context.Context) error // c.Orch.Disconnect(ctx)
  func NewDPIManager(paths store.Paths, p platform.Deps, list func() strategies.List) *dpi.Manager // moved from shell
  ```
- `shell.Run` (Windows path):
  1. `preflight`, then `core.OpenLog`.
  2. `core.New` with these hooks:
     - `SetMode: ui.setMode`
     - `OnSettingsChanged`: the language and proxy relabelling
     - `OnUpdate: ui.onUpdate`
     - the Task 1 dialogs, using `wapp` once it is set
  3. Build the Wails options with `application.NewService(c.Svc)` (keep this exact call; the binding generator keys the TS paths on it), then `applyPlatformOptions`.
  4. `go c.Run(ctx)`, then the autostart connect, then `wapp.Run()`.
  5. `OnShutdown` calls `c.Shutdown` with a 10 s timeout.
- New depcheck rules: for both `windows` and `linux`, `./internal/core` must not import `github.com/wailsapp/wails`. Why: "core is shared by the Windows GUI and the Linux daemon".

- [ ] **Step 1: Write the failing tests** in `internal/core/core_test.go`, with a helper `testDeps(t) platform.Deps`:
  - Linux-style `Unsupported` stubs for every interface (they exist on both OSes since L1).
  - `Paths: store.ResolvePaths(filepath.Join(dir, "ghostline"), dir)`.
  - An in-memory `Lock`.
  - `DPIEngines` returns nil; `WatchNetwork`/`WatchSysProxy` return a no-op stop.
  ```go
  func TestNew_ServesSettingsAndSnapshot(t *testing.T) // c.Svc.GetSettings().Language == store.Defaults().Language; c.Svc.GetSnapshot().Status == app.StatusDisconnected
  func TestNew_RestoresOrphanedStateFirst(t *testing.T) // Review Focus 1: state.json {version 3, phase "dns_set", pid 999999999} → after New the state file is clean, and the first snapshot carries the startup-restore warning (app.StartupWarnings)
  func TestRun_ReturnsWhenCancelled(t *testing.T)       // ctx cancelled → Run returns within 2 s
  func TestNew_CallsSettingsHookAfterSave(t *testing.T) // OnSettingsChanged sees old/new language after c.Svc.SaveSettings
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/core/` → FAIL (package missing).
- [ ] **Step 3: Implement.** Move the files, write `core.go`/`log.go`, and slim `shell.go` to the window, tray and hooks. Update `stopdpi.go` to use `core.NewDPIManager`. Add the two depcheck rules.
- [ ] **Step 4: Run and verify.**
  - `go test ./internal/core/ ./internal/shell/ ./...` → PASS. The moved tests pass unchanged apart from the package clause.
  - `GOOS=windows go vet ./...` → no output.
  - `go run ./tools/depcheck` → `depcheck: 6 rules ok`.
  - `CGO_ENABLED=0 go build ./internal/core/` succeeds.
- [ ] **Step 5: Commit.** `refactor(core): the Wails-free half of shell.Run becomes internal/core, shared by the Windows GUI and the Linux daemon`

### Task 3: `internal/headless`: recovery and export modes outside `main`

**Files:**
- Create: `internal/headless/headless.go` (the body of today's `headless.go` and `stopdpi.go`), `internal/headless/headless_test.go`
- Delete: `headless.go`, `stopdpi.go` (root)
- Modify: `main.go`, `tools/depcheck/rules.go`

**Interfaces:**
- Produces: `func Run(mode cli.Mode, p platform.Deps) int`. Exit codes, logging and behaviour are identical to `runHeadless`. `modeName` moves with it.
- New depcheck rules: for both OSes, `./internal/headless` must not import `github.com/wailsapp/wails`.

- [ ] **Step 1: Write the failing tests.**
  - `TestRun_ExportWritesFile`: `cli.Mode{Kind: cli.KindExport, ExportPath: <tmp>/out.json}` with `testDeps`-style deps gives exit code 0, and the file exists and parses with `backup.Parse`.
  - `TestRun_RestoreOnCleanStateIsNoop`: `cli.KindRestore` gives 0, and `state.json` is still clean.
  - Copy the `testDeps` helper locally rather than exporting it.
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/headless/` → FAIL.
- [ ] **Step 3: Implement.** Move the code; `main.go` calls `headless.Run(mode, p)`.
- [ ] **Step 4: Run and verify.**
  - `go test ./internal/headless/ ./...` → PASS.
  - `GOOS=windows go vet ./...` → no output.
  - `go run ./tools/depcheck` → `depcheck: 8 rules ok`.
- [ ] **Step 5: Commit.** `refactor(headless): recovery and export modes move to internal/headless so the daemon can share them`

### Task 4: Tray drives `Service` methods only

**Files:**
- Create: `internal/shell/tray.go`, `internal/shell/tray_test.go`
- Modify: `internal/shell/ui.go`, `internal/shell/shell.go`

**Interfaces:**
- Produces, in `tray.go`:
  ```go
  type trayBackend interface {
      GetSnapshot() app.Snapshot
      GetSettings() store.Settings
      SaveSettings(store.Settings) error
      Connect() error
      Disconnect() error
      SetDPIEnabled(on bool) error
      SetProxyEnabled(on bool) error
      CheckUpdateNow() (app.UpdateCheck, error)
      LANDNSClients() int
  }
  // trayToggle connects, or disconnects after confirm when LAN devices use this PC's DNS.
  func trayToggle(b trayBackend, confirm func(clients int) bool) error
  ```
- `ui` loses the `orch`, `box`, `checker`, `svc` and `lanDNSClients` fields and gains `b trayBackend`. Each call site maps one-to-one:
  - `orch.Snapshot` → `b.GetSnapshot`
  - `box.Get`/`box.Save` → `b.GetSettings`/`b.SaveSettings`
  - `orch.SetDPIEnabled` → `b.SetDPIEnabled`
  - `checker.checkNow` → `b.CheckUpdateNow` (the tray only uses `Newer` and `err`)
  - `lanDNSClients` → `b.LANDNSClients`
- Windows passes `c.Svc`.

- [ ] **Step 1: Write the failing tests** with a recording `fakeTray`:
  ```go
  func TestTrayToggle_ConnectsWhenDisconnected(t *testing.T)            // status disconnected → Connect called, confirm not called
  func TestTrayToggle_AsksBeforeDisconnectWithLANClients(t *testing.T)  // protected + LANDNSClients()==3 → confirm(3); false → no Disconnect; true → Disconnect
  func TestTrayToggle_DisconnectsWithoutAskingWhenNoClients(t *testing.T) // degraded + 0 clients → Disconnect, confirm not called
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/shell/ -run TestTrayToggle` → FAIL.
- [ ] **Step 3: Implement.** Write `tray.go` and rewire `ui.go`.
- [ ] **Step 4: Run and verify.** `go test ./internal/shell/ ./...` → PASS; `GOOS=windows go vet ./...` → no output.
- [ ] **Step 5: Commit.** `refactor(shell): the tray drives Service methods only, so a remote Service can stand in`

### Task 5: Linux `procs`, and daemon paths in `platform`

**Files:**
- Create: `internal/procs/procs_linux.go`, `internal/procs/procs_linux_test.go`
- Modify:
  - `internal/platform/platform.go`: add a `Socket string` field. Windows sets `""`; on Linux it is the daemon socket.
  - `internal/platform/platform_linux.go`, `internal/platform/platform_test.go`
  - `internal/store/paths.go`: add `PathsIn`.

**Interfaces:**
- Produces:
  ```go
  // procs
  func NewLinux() Inspector // IsAdmin: os.Geteuid()==0; StartTime: /proc/<pid>/stat field 22 (clock ticks, 100/s) + btime from /proc/stat;
                            // Alive: process exists and |StartTime-start| < time.Second; WaitForExit: unix.PidfdOpen + unix.Poll until readable;
                            // PortOwners and StopService still return errors.ErrUnsupported (L3)
  // store
  func PathsIn(dataDir, logDir string) Paths // every file under dataDir, LogDir = logDir, MachineDir = dataDir; ResolvePaths now builds on it
  // platform (Linux)
  func New(exe string) (Deps, error)    // daemon: data /var/lib/ghostline/data, logs /var/log/ghostline, Socket /run/ghostline/ctl.sock, Lock /run/ghostline/state.lock, Procs NewLinux(), UsesDaemon true
  func NewDev(dir string) (Deps, error) // same layout under dir: dir/data, dir/log, dir/run/ctl.sock, dir/run/state.lock
  func ClientSocket() string            // $GHOSTLINE_SOCKET if set, else /run/ghostline/ctl.sock (the GUI's view)
  ```

- [ ] **Step 1: Write the failing tests.**
  ```go
  func TestLinux_SelfIsAliveWithItsStartTime(t *testing.T) // st, err := NewLinux().StartTime(uint32(os.Getpid())); no error; Alive(pid, st) true; Alive(pid, st.Add(time.Hour)) false; Alive(999999999, st) false
  func TestLinux_WaitForExitReturnsWhenChildExits(t *testing.T) // exec "true", Start; WaitForExit(pid) returns nil within 2 s (before cmd.Wait)
  func TestLinux_IsAdminMatchesEuid(t *testing.T)
  func TestNewDev_PutsEverythingUnderDir(t *testing.T)   // DataDir, LogDir, Socket and the lock file all have the prefix dir; UsesDaemon true
  func TestPathsIn_MatchesResolvePathsLayout(t *testing.T) // in store: same file names as ResolvePaths for the same DataDir
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/procs/ ./internal/platform/ ./internal/store/` → FAIL.
- [ ] **Step 3: Implement** the functions above.
- [ ] **Step 4: Run and verify.** `go test ./...` → PASS; `GOOS=windows go vet ./...` → no output.
- [ ] **Step 5: Commit.** `feat(linux): process inspection from /proc and the daemon's directories in platform`

### Task 6: `internal/rpc`: protocol, server and client

**Files:**
- Create: `internal/rpc/proto.go`, `internal/rpc/conn.go`, `internal/rpc/server.go`, `internal/rpc/client.go`, `internal/rpc/rpc_test.go`

**Interfaces:**
- Produces:
  ```go
  const Protocol = 1
  const MaxLine = 16 << 20
  const (CodeUnreachable = "DAEMON_UNREACHABLE"; CodeProtocol = "DAEMON_PROTOCOL_MISMATCH"; CodeNotAuthorized = "NOT_AUTHORIZED"; CodeNoUI = "NO_UI")
  type Hello struct{ Version string `json:"version"`; Protocol int `json:"protocol"` }
  type Error struct{ Message string `json:"message"`; Detail json.RawMessage `json:"detail,omitempty"` }
  type Msg struct { // one line; exactly one of hello/call/result/error/event/ui is meaningful
      Hello *Hello `json:"hello,omitempty"`; ID uint64 `json:"id,omitempty"`; Call string `json:"call,omitempty"`
      Args []json.RawMessage `json:"args,omitempty"`; Result json.RawMessage `json:"result,omitempty"`; Error *Error `json:"error,omitempty"`
      Event string `json:"event,omitempty"`; Data json.RawMessage `json:"data,omitempty"`; UID uint64 `json:"uid,omitempty"`; UI string `json:"ui,omitempty"`
  }
  type Handler func(ctx context.Context, method string, args []json.RawMessage) (json.RawMessage, error)
  func NewServer(h Handler, version string, authorize func(net.Conn) error, log *slog.Logger) *Server
  func (s *Server) Serve(l net.Listener) error
  func (s *Server) Emit(name string, data any) // app.Emitter: to every connected client
  func (s *Server) Close() error
  func Dial(ctx context.Context, socket, version string) (*Client, error) // dial + hello; mismatch → error message starting with CodeProtocol
  func (c *Client) Call(ctx context.Context, method string, result any, args ...any) error
  func (c *Client) OnEvent(fn func(name string, data json.RawMessage))
  func (c *Client) OnUI(fn func(ctx context.Context, kind string, args []json.RawMessage) (any, error))
  func (c *Client) Done() <-chan struct{}
  func (c *Client) Close() error
  // RemoteError is what a call returns for a daemon-side error: Error() == Message, MarshalJSON == Detail (or {}),
  // so Wails marshals it exactly as it would the original error (defaultMarshalError: json.Marshal(&err)).
  type RemoteError struct{ Message string; Detail json.RawMessage }
  ```
- Server behaviour:
  - It calls `authorize` before reading anything. On failure it writes `{"error":{"message":"NOT_AUTHORIZED"}}` and closes.
  - The hello is the first line each way.
  - Each call runs in its own goroutine; writes on a connection are serialised.
  - A call's error becomes `Error{Message: err.Error(), Detail: json.Marshal(&err)}`.
  - A line longer than `MaxLine` closes the connection.

- [ ] **Step 1: Write the failing tests** (unix sockets in `t.TempDir()`, `authorize` = accept all unless stated):
  ```go
  func TestCallRoundTrip(t *testing.T)                 // handler echoes args → Call result equals
  func TestConcurrentCalls(t *testing.T)               // 20 goroutines × Call, each gets its own answer
  func TestEventsReachEveryClient(t *testing.T)        // two clients; server.Emit("state", {"a":1}) → both OnEvent receive it
  func TestErrorKeepsMessageAndJSON(t *testing.T)      // handler returns &codeErr{Code:"X", Params:{"n":1}} (Error() "X: boom") →
                                                       // err.Error()=="X: boom"; json.Marshal(&err) == json.Marshal(&origErr as error)
  func TestHelloProtocolMismatch(t *testing.T)         // server speaks protocol 2 → Dial error starts with "DAEMON_PROTOCOL_MISMATCH"
  func TestAuthorizeRejects(t *testing.T)              // authorize returns error → Dial error starts with "NOT_AUTHORIZED"
  func TestLineTooLongClosesConnection(t *testing.T)   // raw conn writes MaxLine+1 bytes without newline → server closes it
  func TestLargeMessageWithinMaxLine(t *testing.T)     // Review Focus 4: an 8 MiB []byte argument round-trips intact
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/rpc/` → FAIL.
- [ ] **Step 3: Implement** the four files.
- [ ] **Step 4: Run and verify.**
  - `go test -race ./internal/rpc/` → PASS.
  - `CGO_ENABLED=0 GOOS=windows go vet ./internal/rpc/` → no output (the package is portable; only peer checks are OS-specific).
- [ ] **Step 5: Commit.** `feat(rpc): line-delimited JSON protocol over a Unix socket: calls, events, hello and errors that marshal like Wails'`

### Task 7: `rpc.ServiceHandler` and reverse `ui` calls

**Files:**
- Create: `internal/rpc/handler.go`, `internal/rpc/ui.go`, `internal/rpc/handler_test.go`, `internal/rpc/ui_test.go`

**Interfaces:**
- Produces:
  ```go
  // ServiceHandler dispatches calls to svc's exported methods by name: a leading context.Context
  // parameter gets the call's ctx; args decode into the parameter types; a trailing error is the error;
  // 0 other results → null, 1 → that value, >1 → JSON array. Unknown method → error "rpc: unknown method <m>".
  func ServiceHandler(svc any) Handler
  const UITimeout = 10 * time.Minute
  var ErrNoUI = errors.New(CodeNoUI)
  // CallUI asks the client whose call ctx belongs to; outside a call, after that client is gone,
  // or after UITimeout → ErrNoUI (the timeout wraps context.DeadlineExceeded too).
  func CallUI(ctx context.Context, kind string, result any, args ...any) error
  ```

- [ ] **Step 1: Write the failing tests** against a test type `calc`, with the methods `Add(a, b int) int`, `Div(a, b int) (int, error)`, `Pair() (int, string)`, `WithCtx(ctx context.Context, s string) string` and `Ask(ctx context.Context) (string, error)` (Ask calls `CallUI(ctx, "pick", &out, "q")`):
  ```go
  func TestServiceHandler_DecodesArgsAndResults(t *testing.T) // Add→3, Pair→[1,"x"], WithCtx gets a non-nil ctx
  func TestServiceHandler_ErrorAndUnknownMethod(t *testing.T) // Div by 0 → error; "Nope" → "rpc: unknown method Nope"
  func TestCallUI_GoesToCallingClient(t *testing.T)           // clients A and B registered OnUI; A calls Ask → only A's OnUI runs, result returned
  func TestCallUI_ClientGoneIsNoUI(t *testing.T)              // A's OnUI closes A → Ask returns ErrNoUI
  func TestCallUI_OutsideCallIsNoUI(t *testing.T)             // CallUI(context.Background(), …) → ErrNoUI
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/rpc/` → FAIL.
- [ ] **Step 3: Implement** both files.
- [ ] **Step 4: Run and verify.** `go test -race ./internal/rpc/` → PASS.
- [ ] **Step 5: Commit.** `feat(rpc): reflection dispatch to a service and reverse UI calls to the calling client`

### Task 8: Peer authorisation (`SO_PEERCRED`)

**Files:**
- Create: `internal/rpc/peer.go`, `internal/rpc/peer_linux.go`, `internal/rpc/peer_other.go`, `internal/rpc/peer_test.go`, `internal/rpc/peer_linux_test.go`

**Interfaces:**
- Produces:
  ```go
  var DefaultGroups = []string{"wheel", "sudo", "admin", "ghostline"}
  // allowed is the pure decision: uid 0, a uid in extra, or any group name in allowedGroups.
  func allowed(uid uint32, groupNames, allowedGroups []string, extra []int) bool
  // AllowGroups authorises a Unix-socket peer by SO_PEERCRED (Linux); elsewhere it rejects every peer.
  func AllowGroups(groups []string, extraUIDs ...int) func(net.Conn) error
  ```
- Errors start with `CodeNotAuthorized`.
- The Linux implementation reads the peer with `unix.GetsockoptUcred` and resolves groups with `user.LookupId` → `GroupIds` → `LookupGroupId`.

- [ ] **Step 1: Write the failing tests.**
  - `TestAllowed` (table):
    - root → true
    - uid 1000 in `[users wheel]` → true
    - in `[users]` → false
    - extra `[1000]` → true
    - empty groups and no extra → false
  - `TestAllowGroups_SelfViaExtraUID` (Linux): a real socket pair; `AllowGroups(nil, os.Getuid())` accepts.
  - `TestAllowGroups_RejectsWhenNotListed` (Linux): `AllowGroups([]string{"no-such-group-xyz"})` rejects with `NOT_AUTHORIZED`. Skip if `os.Getuid()==0`.
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/rpc/ -run 'TestAllow'` → FAIL.
- [ ] **Step 3: Implement** the files.
- [ ] **Step 4: Run and verify.** `go test ./internal/rpc/` → PASS; `GOOS=windows go vet ./internal/rpc/` → no output.
- [ ] **Step 5: Commit.** `feat(rpc): authorise socket peers by uid and group (SO_PEERCRED)`

### Task 9: `tools/genrpc` and the generated `client.Service`

**Files:**
- Create:
  - `tools/genrpc/main.go`, `tools/genrpc/main_test.go`
  - `internal/rpc/client/doc.go` (package doc and `//go:generate go run ../../../tools/genrpc -out service_gen.go -skip SetMode,GetSnapshot`)
  - `internal/rpc/client/service.go` (hand-written)
  - `internal/rpc/client/service_gen.go` (generated, committed)
  - `internal/rpc/client/service_test.go`
- Modify: `internal/app/errors.go`, adding `CodeDaemonUnreachable = "DAEMON_UNREACHABLE"`, `CodeDaemonProtocol = "DAEMON_PROTOCOL_MISMATCH"`, `CodeNotAuthorized = "NOT_AUTHORIZED"`, `CodeNoUI = "NO_UI"`.

**Interfaces:**
- Produces, in `genrpc`:
  - `func bindingID(method string) uint32`: FNV-32a of `"github.com/hashcott/ghostline/internal/app.Service." + method`, the same as Wails' `hash.Fnv`.
  - `func render(methods []reflect.Method, skip map[string]bool) ([]byte, error)`: gofmt'd source. Type expressions come from `reflect.Type` (`PkgPath`/`Name`, recursing through slices, maps and pointers), and the import set is collected from them.
  - Generated code:
    - Each method has the identical signature.
    - The body is `err := s.call(<ctx or context.Background()>, "<Name>", <&r or nil>, <args...>)`.
    - Methods with no error result log the error with `s.log.Warn` and return the zero value.
    - An `init()` with `application.RegisterBindingMethodID((*Service).<Name>, <id>)` for **every** method, skipped ones included.
- Produces, in `client`:
  ```go
  func New(socket string, log *slog.Logger, onMode func(mode string)) *Service // dials lazily on first call; redials after the connection drops
  func (s *Service) OnEvent(fn func(name string, data json.RawMessage))      // also fires a synthetic "state" (unreachable snapshot) when the connection drops
  func (s *Service) OnUI(fn func(ctx context.Context, kind string, args []json.RawMessage) (any, error))
  func (s *Service) Close() error
  func (s *Service) SetMode(mode string) error // RPC, then onMode(mode) on success
  func (s *Service) GetSnapshot() app.Snapshot  // RPC; on a transport error: app.Snapshot{Status: app.StatusError,
                                                //   Error: &app.AppError{Code: <CodeDaemonUnreachable | CodeNotAuthorized | CodeDaemonProtocol>},
                                                //   Warnings: []app.AppError{}, Servers: []string{}, BlockedSites: []string{}}
  ```

- [ ] **Step 1: Write the failing tests.**
  ```go
  // tools/genrpc
  func TestBindingID_IsFNV32aOfAppFQN(t *testing.T) // bindingID("Connect") == fnv32a("github.com/hashcott/ghostline/internal/app.Service.Connect")
  func TestRender_IsUpToDate(t *testing.T)          // render(app methods, skip) == contents of internal/rpc/client/service_gen.go
  // internal/rpc/client
  func TestService_HasEveryAppMethod(t *testing.T)  // for every method of *app.Service: *client.Service has it with an identical func type (receiver aside)
  func TestService_RegistersEveryBindingID(t *testing.T) // service_gen.go contains one RegisterBindingMethodID line per app method, with the right id
  func TestCodesMatchApp(t *testing.T)              // rpc.CodeUnreachable == app.CodeDaemonUnreachable, and the other three
  func TestGetSnapshot_DaemonDownIsUnreachable(t *testing.T) // socket path does not exist → Status error, Error.Code DAEMON_UNREACHABLE
  func TestService_ReconnectsAfterDaemonRestart(t *testing.T) // Review Focus 3: rpc server over ServiceHandler(fake); GetSettings works;
                                                              // server closed + restarted on the same socket → next GetSettings works; an Emit after restart reaches OnEvent
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./tools/genrpc/ ./internal/rpc/client/` → FAIL.
- [ ] **Step 3: Implement.** Write genrpc and `service.go`, then run `go generate ./internal/rpc/client/`.
- [ ] **Step 4: Run and verify.**
  - `go test ./tools/genrpc/ ./internal/rpc/client/ ./...` → PASS.
  - Run `go generate ./...`, then `git status --porcelain internal/rpc/client` → empty.
- [ ] **Step 5: Commit.** `feat(rpc): generated client Service with Wails binding IDs identical to app.Service`

### Task 10: `cmd/ghostlined`

**Files:**
- Create: `cmd/ghostlined/main.go`, `cmd/ghostlined/args.go`, `cmd/ghostlined/args_test.go`, `cmd/ghostlined/daemon.go`, `cmd/ghostlined/listen.go`, `cmd/ghostlined/listen_test.go`, `cmd/ghostlined/daemon_linux_test.go`
- Modify: `tools/depcheck/rules.go`, adding a linux rule for `./cmd/ghostlined` that forbids `github.com/wailsapp/wails`, `golang.org/x/sys/windows`, `…/internal/winutil` and `…/assets/goodbyedpi`.

**Interfaces:**
- Produces, in `args.go`:
  ```go
  type Args struct {
      Command   string // "daemon" | "restore" | "remove-certs" | "export" | "status" | "connect" | "disconnect"
      Socket    string // --socket (default platform socket)
      DataDir   string // --data-dir: dev/test layout via platform.NewDev
      AllowUID  int    // --allow-uid, -1 when unset
      ExportPath string
  }
  func parseArgs(argv []string) (Args, error) // "--daemon", "--restore", "--remove-certs", "--export <path>", "status", "connect", "disconnect"; unknown → error
  ```
- Produces, in `listen.go`: `func listen(socket string) (net.Listener, error)`.
  - If a daemon answers on `socket`, return an error.
  - Otherwise remove a stale file, `net.Listen("unix", socket)`, then `os.Chmod(socket, 0o666)`.
- Produces, in `daemon.go`: `func runDaemon(ctx context.Context, a Args) error`.
  1. `platform.New` (or `NewDev(a.DataDir)`), then `core.OpenLog`.
  2. `srv := rpc.NewServer(…, rpc.AllowGroups(rpc.DefaultGroups, a.AllowUID…))`.
  3. `core.New` with these options:
     - `Emitter: srv`
     - `OnSettingsChanged`: `srv.Emit("settings", n)`
     - `OpenFile`: `rpc.CallUI(ctx, "openFile", …)`, decoding `{name, data}`
     - `SaveFile`: `rpc.CallUI(ctx, "saveFile", nil, name, data)`
     - `SetMode`: nil
  4. If `StartWithWindows && AutoConnect`, `go Connect`.
  5. `go c.Run(ctx)`, `srv.Serve(listener)`.
  6. On ctx done (SIGTERM/SIGINT in `main`): `c.Shutdown` with 10 s, `srv.Close`, remove the socket.
- Produces, in `main.go`:
  - `restore`, `remove-certs` and `export` → `headless.Run`, mapping to the `cli.Kind` values.
  - `status` → `rpc.Dial`, then `GetSnapshot`; print `status` and `error.code`.
  - `connect`/`disconnect` → the call; print the error and exit 1 on failure.

- [ ] **Step 1: Write the failing tests.**
  ```go
  func TestParseArgs(t *testing.T) // table: each command; --socket/--data-dir/--allow-uid values; "--export" without a path → error; unknown flag → error
  func TestListen_ReplacesStaleSocket(t *testing.T) // Review Focus 2: plain file at socket path → listen succeeds and the path is a socket with mode 0666
  func TestListen_RefusesWhenLive(t *testing.T)     // a listener already accepting on the path → listen returns error
  // daemon_linux_test.go (no root needed):
  func TestDaemon_EndToEnd(t *testing.T) {
      // runDaemon(ctx, Args{Command:"daemon", DataDir:t.TempDir(), Socket:<tmp>/ctl.sock, AllowUID:os.Getuid()}) in a goroutine;
      // c := client.New(socket, log, nil): GetSettings().Language == "vi";
      // SaveSettings(language "en") → an OnEvent "settings" arrives within 2 s;
      // GetSnapshot().Status == app.StatusDisconnected; Connect() returns an error and the system is untouched;
      // with OnUI recording, ExportSettings(ctx, nil) → OnUI got kind "saveFile" whose first arg is a name ending ".ghostline.json";
      // cancel ctx → runDaemon returns nil within 12 s and the socket file is gone.
  }
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./cmd/ghostlined/` → FAIL.
- [ ] **Step 3: Implement** the four source files and the depcheck rule.
- [ ] **Step 4: Run and verify.**
  - `go test -race ./cmd/ghostlined/` → PASS.
  - `CGO_ENABLED=0 go build -o /dev/null ./cmd/ghostlined` → succeeds.
  - `go run ./tools/depcheck` → `depcheck: 9 rules ok`.
- [ ] **Step 5: Commit.** `feat(ghostlined): the Linux daemon serves core over the control socket, with recovery, export and status commands`

### Task 11: The Linux GUI as a client

**Files:**
- Create:
  - `internal/shell/client_linux.go`, `internal/shell/client_windows.go` (`runClient` returns `errors.New("shell: no daemon client on Windows")`)
  - `internal/shell/eventdecode.go`, `internal/shell/eventdecode_test.go`
  - `internal/shell/uifiles.go`, `internal/shell/uifiles_test.go`
- Modify: `internal/shell/shell.go`. `Run` starts with `if o.Platform.UsesDaemon { return runClient(o) }`; the in-process path is unchanged.

**Interfaces:**
- Produces:
  ```go
  // eventdecode.go: one entry per typed event registered in events.go
  func decodeEvent(name string, data json.RawMessage) (any, bool) // value of the registered type (e.g. app.Snapshot for "state"); false for unknown names
  // uifiles.go: GUI-side halves of the reverse calls; dialogs injected for tests
  func saveFileHandler(pickPath func(name string) (string, error)) func(args []json.RawMessage) (any, error) // args: name, data(base64 []byte); writes 0o644; "" path = cancelled → nil
  func openFileHandler(pickPath func(title string) (string, error)) func(args []json.RawMessage) (any, error) // returns {"name": base, "data": bytes}; > backup.MaxSize → error "IMPORT_INVALID: file too large"
  func runClient(o Options) error // client.New(platform.ClientSocket(), log, ui.setMode); application.NewService(proxy);
                                  // OnEvent: "settings" → ui.onLanguage, "update" → ui.onUpdate, typed events → wapp.Event.Emit(name, decoded);
                                  // OnUI: "saveFile"/"openFile" via the handlers with wapp dialogs; tray backend = proxy;
                                  // log to ~/.config/ghostline/logs; autostart mode = tray only
  ```

- [ ] **Step 1: Write the failing tests.**
  ```go
  // Review Focus 5: parse events.go for `RegisterEvent[T](app.EventX)`; for each, decodeEvent(name of EventX, `{}`) returns ok
  // and fmt.Sprintf("%T", v) == the registered T (e.g. "app.Snapshot", "[]certstore.Cert"); and no extra names in the table.
  func TestDecodeEvent_CoversEveryRegisteredEvent(t *testing.T)
  func TestSaveFileHandler_WritesAsCaller(t *testing.T)    // pickPath → <tmp>/a.json; file has the bytes; mode 0644
  func TestSaveFileHandler_CancelledWritesNothing(t *testing.T)
  func TestOpenFileHandler_RejectsOversize(t *testing.T)   // Review Focus 4: file of backup.MaxSize+1 bytes → error starting "IMPORT_INVALID"
  func TestOpenFileHandler_ReturnsNameAndData(t *testing.T)
  ```
- [ ] **Step 2: Run the tests to verify they fail.** `go test ./internal/shell/ -run 'TestDecodeEvent|Handler'` → FAIL.
- [ ] **Step 3: Implement** the files.
- [ ] **Step 4: Run and verify.**
  - `go test ./...` → PASS.
  - `GOOS=windows go vet ./...` → no output.
  - `go run ./tools/depcheck` → `depcheck: 9 rules ok` (Windows `.` still has no `internal/rpc`).
  - Manual check: `sudo ./bin/ghostlined --daemon` in one terminal, then `go build -o bin/ghostline . && ./bin/ghostline`.
    - The window shows the real settings.
    - Pressing Connect no longer shows NOT_ADMIN.
    - Closing the window leaves the daemon running.
    - Settings → export shows a save dialog, and the file is owned by the user.
- [ ] **Step 5: Commit.** `feat(shell): on Linux the GUI is a client of ghostlined: generated proxy, re-emitted events, dialogs for the daemon`

### Task 12: Frontend: daemon errors and their retry

**Files:**
- Modify: `frontend/src/components/ConnectError.tsx`, `frontend/src/i18n/en.json`, `frontend/src/i18n/vi.json`
- Create: `frontend/src/components/ConnectError.test.tsx`

**Interfaces:**
- When `snap.error.code` is one of `DAEMON_UNREACHABLE`, `DAEMON_PROTOCOL_MISMATCH` or `NOT_AUTHORIZED`, the only action is Retry. It calls `Service.GetSnapshot().then(useGhost.getState().setSnapshot)`, never `Connect`.
- i18n copy, under `errors.<CODE>.message` / `.action`:

  | Code | en | vi |
  |---|---|---|
  | `DAEMON_UNREACHABLE` | "Ghostline's background service is not running" / "Start it with: sudo systemctl start ghostline" | "Dịch vụ nền của Ghostline chưa chạy" / "Khởi động bằng lệnh: sudo systemctl start ghostline" |
  | `DAEMON_PROTOCOL_MISMATCH` | "The background service is a different version" / "Update Ghostline so the app and the service match" | "Dịch vụ nền khác phiên bản với ứng dụng" / "Cập nhật Ghostline để ứng dụng và dịch vụ cùng phiên bản" |
  | `NOT_AUTHORIZED` | "Your account may not control Ghostline" / "Run: sudo usermod -aG ghostline $USER, then log in again" | "Tài khoản của bạn không có quyền điều khiển Ghostline" / "Chạy: sudo usermod -aG ghostline $USER rồi đăng nhập lại" |
  | `NO_UI` | "This needs the Ghostline window" / "Open Ghostline and try again" | "Thao tác này cần cửa sổ Ghostline" / "Mở Ghostline rồi thử lại" |

- [ ] **Step 1: Write the failing tests** (vitest + Testing Library, mocking `../app/api` as `App.test.tsx` does):
  - `"daemon unreachable shows its message"`: the vi text is visible.
  - `"retry reloads the snapshot instead of connecting"`: clicking Retry calls `GetSnapshot` once and `Connect` zero times.
- [ ] **Step 2: Run the tests to verify they fail.** `cd frontend && npx vitest run src/components/ConnectError.test.tsx` → FAIL.
- [ ] **Step 3: Implement** the component change and the i18n entries.
- [ ] **Step 4: Run and verify.** `cd frontend && npm test` → PASS (`parity.test.ts` included); `npm run build` → ok.
- [ ] **Step 5: Commit.** `feat(ui): daemon connection errors with a retry that reloads state`

### Task 13: systemd unit, CI, docs

**Files:**
- Create: `build/linux/ghostline.service`
- Modify:
  - `.github/workflows/ci.yml` (Linux job)
  - `docs/platforms.md`
  - `docs/superpowers/specs/2026-10-07-ghostline-linux-l2-design.md`:
    - §3 and §5: "4 MiB" → "16 MiB", with the reason.
    - §7: `platform.NewClient()` → `platform.ClientSocket()`. The GUI only needs the socket path; its log and preferences live under `~/.config/ghostline`.

**Interfaces:**
- `build/linux/ghostline.service`, exact content:
  ```ini
  [Unit]
  Description=Ghostline encrypted DNS and DPI bypass
  After=network.target NetworkManager.service systemd-resolved.service

  [Service]
  Type=simple
  ExecStart=/usr/lib/ghostline/ghostlined --daemon
  ExecStopPost=/usr/lib/ghostline/ghostlined --restore
  Restart=on-failure
  RestartSec=2
  StateDirectory=ghostline
  StateDirectoryMode=0755
  LogsDirectory=ghostline
  RuntimeDirectory=ghostline
  RuntimeDirectoryMode=0755
  NoNewPrivileges=yes

  [Install]
  WantedBy=multi-user.target
  ```
- CI Linux job, after "Build":
  - `go generate ./... && git diff --exit-code`, as two steps: run `go generate ./...`, then `git diff --exit-code`.
  - `CGO_ENABLED=0 go build -o /dev/null ./cmd/ghostlined`.
- `docs/platforms.md`:
  - The Watchdog row (Linux): "systemd `ExecStopPost=--restore` (`build/linux/ghostline.service`)".
  - Port owners: Linux is now "`/proc` liveness (`internal/procs/procs_linux.go`); port owners and service stop: stub (L3)".
  - New rows: "Daemon and GUI client" (`cmd/ghostlined`, `internal/rpc`, `internal/rpc/client`, `internal/shell/client_linux.go`) and "Shared core" (`internal/core`).

- [ ] **Step 1: Write the unit file, the CI steps and the doc edits.**
- [ ] **Step 2: Check.**
  - `systemd-analyze verify build/linux/ghostline.service`: the only complaints should be about the missing `/usr/lib/ghostline/ghostlined` binary.
  - `go run github.com/rhysd/actionlint/cmd/actionlint@latest -verbose .github/workflows/ci.yml` → `Found total 0 errors`.
  - Every path in `docs/platforms.md` exists (the same `grep … | xargs ls` check as L1 Task 12).
- [ ] **Step 3: Commit.** `build(linux): systemd unit for ghostlined; CI checks generated code and a CGO-free daemon build`
