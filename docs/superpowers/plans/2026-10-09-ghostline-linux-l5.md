# Ghostline Linux L5 (packaging + release) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ghostline installs and uninstalls cleanly on Linux from `.deb`, `.rpm`, AppImage, `tar.gz` and a PKGBUILD, runs as a hardened systemd service from first boot, and one tag push publishes Windows and Linux files with one `SHA256SUMS`.

**Architecture:** A new `internal/sysinstall` package owns the unit template (one source for packages and self-install), `--install-system`/`--uninstall-system`, and the GUI's install-kind detection; system calls sit behind a small interface. The GUI's service buttons are `app.Service` methods skipped by `genrpc` and written by hand in `internal/rpc/client`, so they work while the daemon is down. Packaging lives in `build/linux/` (Taskfile + nfpm + mksquashfs with a vendored AppImage runtime + scripts); CI builds, installs and removes the packages on the runner.

**Tech Stack:** Go 1.27, Wails v3 beta.27, go-task (via `wails3 task`), nfpm v2 (`go run`, pinned), squashfs-tools + AppImage type2 runtime (vendored), systemd, polkit `pkexec`, GitHub Actions, React + vitest.

**Spec:** `docs/superpowers/specs/2026-10-09-ghostline-linux-l5-design.md` (with the master Linux spec `docs/superpowers/specs/2026-10-07-ghostline-linux-design.md` §5.6, §14, §16–§17, §19.3).

## Global Constraints

- OS specifics only in `_linux.go`/`_windows.go` (or `//go:build` files); no `runtime.GOOS` in shared code.
- Windows behaviour and package contents unchanged; Windows exe ≤ 2% larger than v0.5.1 (size guard in release).
- `ghostlined` stays `CGO_ENABLED=0` and free of Wails (`tools/depcheck` must stay green).
- Every new UI string in `vi.json` and `en.json` (Linux-only wording in `vi.linux.json`/`en.linux.json`).
- Commits: no `Co-Authored-By` or "Generated with" lines.
- Paths: package daemon `/usr/lib/ghostline/ghostlined`; self-installed daemon `/var/lib/ghostline/bin/ghostlined`; package unit `/usr/lib/systemd/system/ghostline.service`; self-install unit `/etc/systemd/system/ghostline.service`; tar.gz GUI `/usr/local/bin/ghostline`, daemon copy for the GUI `/usr/local/lib/ghostline/ghostlined`.
- Version: `-ldflags "-s -w -X github.com/hashcott/ghostline/internal/brand.Version=<ver>"`, `-trimpath`.
- Dependencies: deb `libgtk-4-1, libwebkitgtk-6.0-4`, recommends `libnss3-tools`; rpm `gtk4, webkitgtk6.0`, recommends `nss-tools`; Arch `gtk4 webkitgtk-6.0`, optdepends `nss`.
- AppStream id `io.github.hashcott.ghostline`; group `ghostline`.
- Root-only checks are scripts the maintainer runs (`sudo bash …`); never run sudo from the session.

## Review Focus

1. Package **upgrade** while connected: prerm on upgrade (deb `upgrade`, rpm `$1 = 1`, Arch `pre_upgrade`) must not run `--remove-certs`; postinst `try-restart`s. → scripts test in Task 7.
2. `--install-system` run by the already installed copy (`/var/lib/ghostline/bin/ghostlined --install-system`): source = destination must not truncate the binary. → test in Task 1.
3. `--uninstall-system` when nothing is installed, or the service is already stopped: succeeds, idempotent. → test in Task 1.
4. AppImage or GUI path with spaces in the autostart `Exec=` line: quoted per the Desktop Entry spec. → test in Task 3.
5. `pkexec` dismissed or without a polkit agent (exit 126/127): GUI shows `PKEXEC_FAILED` with the terminal command; stays usable. → test in Task 4.

---

### Task 1: `internal/sysinstall` — unit template, install, uninstall

**Files:**
- Create: `internal/sysinstall/ghostline.service.tmpl` (from `build/linux/ghostline.service`, `@DAEMON@` placeholders, hardening per spec §4)
- Create: `internal/sysinstall/sysinstall.go`, `internal/sysinstall/sysinstall_test.go`
- Delete: `build/linux/ghostline.service` (update its references: `grep -rn 'build/linux/ghostline.service'`)

**Interfaces:**
- Produces:
  - `func Unit(daemon string) []byte` — template with every `@DAEMON@` replaced.
  - `type System interface { Systemctl(args ...string) error; IsActive(unit string) bool; GroupExists(name string) bool; AddGroup(name string) error }`
  - `type Installer struct { Root string; Self string; Sys System }` (`Root` "/" in production; `Self` = running binary)
  - `func (i Installer) Install() error`
  - `func (i Installer) Uninstall(purge bool, removeAll func() error) error`
  - `var ErrPackaged = errors.New("sysinstall: Ghostline is installed by a package; update it with the package manager")`
  - Constants `PackageDaemon = "/usr/lib/ghostline/ghostlined"`, `SelfDaemon = "/var/lib/ghostline/bin/ghostlined"`, `PackageUnit = "/usr/lib/systemd/system/ghostline.service"`, `SelfUnit = "/etc/systemd/system/ghostline.service"`.

- [ ] **Step 1: Write the failing tests** (fake `System` records calls; `Root = t.TempDir()`; `Self` = a temp file with known bytes)
  - `TestUnit_FillsDaemonPath`: `Unit("/x/ghostlined")` contains `ExecStart=/x/ghostlined --daemon` and `ExecStopPost=/x/ghostlined --restore`, no `@DAEMON@`, contains `NoNewPrivileges=yes` and `CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SETUID CAP_SETGID CAP_KILL CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER`.
  - `TestInstall_Fresh`: copies Self to `<Root>/var/lib/ghostline/bin/ghostlined` (same bytes, mode 0755; `var/lib/ghostline` and `bin` 0755), writes `<Root>/etc/systemd/system/ghostline.service` equal to `Unit(SelfDaemon)`, calls `AddGroup("ghostline")`, then `Systemctl("daemon-reload")`, `Systemctl("enable", "--now", "ghostline.service")`; no `.new` file left.
  - `TestInstall_AgainRestarts`: `IsActive` true, group exists → no `AddGroup`; calls `daemon-reload`, `enable ghostline.service`, `restart ghostline.service`.
  - `TestInstall_FromInstalledCopy` (Review Focus 2): `Self` = `<Root>/var/lib/ghostline/bin/ghostlined`; after `Install()` the file still has its bytes.
  - `TestInstall_RefusesPackaged`: `<Root>/usr/lib/systemd/system/ghostline.service` exists → `ErrorIs(ErrPackaged)`, nothing written, no systemctl call.
  - `TestUninstall_RemovesServiceKeepsData`: after Install, `Uninstall(false, removeAll)` → `removeAll` called first, then `Systemctl("disable", "--now", "ghostline.service")`, unit file gone, `daemon-reload`, `var/lib/ghostline/bin` gone, `var/lib/ghostline/data/x` still there.
  - `TestUninstall_Purge`: `Uninstall(true, …)` → `var/lib/ghostline` and `var/log/ghostline` gone.
  - `TestUninstall_NothingInstalled` (Review Focus 3): fresh root, `Systemctl` returns an error for `disable` → `Uninstall` returns nil.
  - `TestUninstall_RemoveAllErrorStillUninstalls`: `removeAll` errors → unit still removed, the error is returned (joined).

- [ ] **Step 2: Run** `go test ./internal/sysinstall/` — Expected: FAIL (package does not compile).

- [ ] **Step 3: Implement** in `sysinstall.go`: `//go:embed ghostline.service.tmpl`; copy via `<dst>.new` + `os.Rename`, reading Self fully into memory first (covers source = destination), verifying the written SHA-256; unit written via temp + rename; `Uninstall` ignores a `disable` error when no unit file existed (check before) and joins `removeAll`'s error.

- [ ] **Step 4: Run** `go test ./internal/sysinstall/` and `GOOS=windows go vet ./internal/sysinstall/` — Expected: PASS / clean.

- [ ] **Step 5: Commit** `feat(sysinstall): unit template and install/uninstall of the system service`

### Task 2: `ghostlined --install-system` / `--uninstall-system [--purge]`

**Files:**
- Create: `internal/sysinstall/system_linux.go` (exec-based `System`: `systemctl`, `systemctl is-active --quiet`, `user.LookupGroup`, `groupadd --system`)
- Modify: `cmd/ghostlined/args.go`, `cmd/ghostlined/args_test.go`, `cmd/ghostlined/main.go`

**Interfaces:**
- Consumes: Task 1 `Installer`, `ErrPackaged`.
- Produces: `func NewSystem() System` (Linux); commands `install-system`, `uninstall-system`; `Args.Purge bool`.

- [ ] **Step 1: Failing tests** in `args_test.go`: `parseArgs([]string{"--install-system"})` → `Command "install-system"`; `{"--uninstall-system", "--purge"}` → `Command "uninstall-system", Purge true`; `{"--purge"}` alone → error `"--purge needs --uninstall-system"`; usage string mentions `--install-system | --uninstall-system [--purge]`.
- [ ] **Step 2: Run** `go test ./cmd/ghostlined/ -run TestParseArgs` — Expected: FAIL.
- [ ] **Step 3: Implement** flags; in `main.go`: `os.Geteuid() != 0` → stderr `ghostlined: --install-system needs root (sudo)` exit 1; `Installer{Root: "/", Self: os.Executable(), Sys: sysinstall.NewSystem()}`; uninstall's `removeAll` = `headless.Run(cli.Mode{Kind: cli.KindRemoveCerts}, deps)` with `depsFor(a)`, non-zero → `errors.New("restoring the system failed")`; any error → stderr + exit 1.
- [ ] **Step 4: Run** `go test ./cmd/ghostlined/ ./internal/sysinstall/`, `go run ./tools/depcheck`, `CGO_ENABLED=0 go build ./cmd/ghostlined` — Expected: PASS / ok / builds.
- [ ] **Step 5: Commit** `feat(ghostlined): --install-system and --uninstall-system`

### Task 3: Start with the system

**Files:**
- Modify: `internal/startup/startup.go`, `internal/startup/startup_test.go`
- Create: `internal/shell/autostart_linux.go`, `internal/shell/autostart_linux_test.go`
- Modify: `internal/shell/client_linux.go` (call on the `settings` event and once after the first settings load)

**Interfaces:**
- Produces: `func syncAutostart(dir, exe string, on bool) error` (dir = `$XDG_CONFIG_HOME/autostart` or `~/.config/autostart`); `func autostartExe(getenv func(string) string, executable func() (string, error)) (string, error)` (`$APPIMAGE` if set).

- [ ] **Step 1: Failing tests**
  - `TestServiceManaged` changes to `require.NoError(t, m.SetAutostart(true))` (systemd starts the daemon; the GUI writes its own autostart file).
  - `TestSyncAutostart_WritesAndRemoves`: `on` → `dir/ghostline.desktop` exists, contains `Exec="/opt/My Apps/Ghostline.AppImage" --autostart` for exe `/opt/My Apps/Ghostline.AppImage` (Review Focus 4: quoted), `Type=Application`, `Name=Ghostline`, `Icon=ghostline`, `X-GNOME-Autostart-enabled=true`; `off` → file gone; `off` again → nil.
  - `TestSyncAutostart_LeavesUnchangedFile`: same content twice → mtime unchanged.
  - `TestAutostartExe_PrefersAppImage`.
- [ ] **Step 2: Run** `go test ./internal/startup/ ./internal/shell/ -run 'ServiceManaged|Autostart'` — Expected: FAIL.
- [ ] **Step 3: Implement** (`ServiceManaged.SetAutostart` returns nil; quote `Exec` per Desktop Entry spec: double quotes, escape `"`, `` ` ``, `$`, `\`; write via temp + rename; create dir 0755).
- [ ] **Step 4: Run** the tests, `GOOS=windows go vet ./internal/startup/ ./internal/shell/` — Expected: PASS / clean.
- [ ] **Step 5: Commit** `feat(linux): start with the system turns on the GUI's autostart entry`

### Task 4: GUI service actions (`ServiceInstall`, `InstallService`, `StartService`)

**Files:**
- Create: `internal/sysinstall/detect.go`, `internal/sysinstall/detect_test.go`
- Modify: `internal/app/service.go` (+ test), `internal/app/errors.go` (codes)
- Modify: `tools/genrpc/main.go` (`defaultSkip` adds the three), regenerate `internal/rpc/client/service_gen.go`
- Create: `internal/rpc/client/install.go` (shared), `internal/rpc/client/install_linux.go`, `internal/rpc/client/install_other.go` (`//go:build !linux`), `internal/rpc/client/install_test.go`

**Interfaces:**
- Produces:
  - `sysinstall.Info{Kind string; Unit bool; SteamOS bool}`; `func Detect(getenv func(string) string, exists func(string) bool, osRelease []byte) Info` — `Kind` `"appimage"` (`APPIMAGE` set) / `"package"` (`PackageUnit` exists) / `"tarball"`; `Unit` = `PackageUnit` or `SelfUnit` exists; `SteamOS` = `ID=steamos` line.
  - `app.InstallInfo{Kind string \`json:"kind"\`; Unit bool \`json:"unit"\`; SteamOS bool \`json:"steamos"\`}`; `func (s *Service) ServiceInstall() InstallInfo` (returns zero), `InstallService() error`, `StartService() error` (return `AppError{Code: CodeUnsupported}`-style error; reuse the existing unsupported code if one exists, else add `SERVICE_ACTION_UNSUPPORTED`).
  - Codes: `PKEXEC_FAILED` (params `command`), `SERVICE_INSTALL_MANUAL` (tar.gz without `../lib/ghostline/ghostlined`).
  - Client: `func (s *Service) ServiceInstall() app.InstallInfo`, `InstallService() error`, `StartService() error`; Linux helpers take a `runner func(name string, args ...string) error` field for tests; after a successful pkexec they wait for `rpc` dial to succeed (≤ 10 s, 250 ms steps).

- [ ] **Step 1: Failing tests**
  - `TestDetect` table: APPIMAGE set → appimage; package unit → package, Unit true; nothing → tarball, Unit false; `/etc/systemd/system/ghostline.service` → Unit true; os-release `ID=steamos` → SteamOS; `ID_LIKE=steamos` alone → false.
  - `TestService_InstallActionsUnsupportedInDaemon` (app): `ServiceInstall()` zero; `InstallService()` errors.
  - `TestGenrpc_SkipsInstallMethods` (or extend the existing render test): generated file has no `ServiceInstall`.
  - Client (`install_test.go`, linux build tag where needed):
    - appimage: copies `$APPDIR/usr/lib/ghostline/ghostlined` to `<runtime>/ghostline-install/ghostlined` (0755) and runs `pkexec <copy> --install-system`.
    - tarball with `../lib/ghostline/ghostlined` next to the exe → `pkexec <that> --install-system`; without → error code `SERVICE_INSTALL_MANUAL`.
    - `StartService` → `pkexec systemctl enable --now ghostline.service`.
    - Review Focus 5: runner returns exit status 126 → error code `PKEXEC_FAILED` with `command` = the equivalent `sudo …` line.
- [ ] **Step 2: Run** `go test ./internal/sysinstall/ ./internal/app/ ./internal/rpc/... ./tools/genrpc/` — Expected: FAIL.
- [ ] **Step 3: Implement**; `go generate ./...`.
- [ ] **Step 4: Run** the same + `git diff --exit-code internal/rpc/client/service_gen.go` after a second `go generate` + `GOOS=windows go vet ./...` — Expected: PASS / clean.
- [ ] **Step 5: Commit** `feat(gui): install or start the background service from the window`

### Task 5: Frontend — service buttons and strings

**Files:**
- Modify: `frontend/src/components/ConnectError.tsx`, `frontend/src/components/ConnectError.test.tsx`
- Modify: `frontend/src/i18n/vi.json`, `en.json` (via `addi18n.py` or by hand)

**Interfaces:**
- Consumes: `Service.ServiceInstall(): Promise<{kind, unit, steamos}>`, `Service.InstallService()`, `Service.StartService()` (Task 4 bindings).

- [ ] **Step 1: Failing tests** (mock the three methods):
  - unreachable + `{kind:"package", unit:true}` → button "Khởi động dịch vụ"; click calls `StartService`, then reloads on success.
  - unreachable + `{kind:"appimage", unit:false}` → button "Cài dịch vụ" calls `InstallService`; `{kind:"appimage", unit:true}` and `{kind:"tarball", unit:true}` → "Khởi động dịch vụ".
  - protocol mismatch + appimage → "Cập nhật dịch vụ"; + package → no install button, text "Cập nhật Ghostline bằng trình quản lý gói".
  - `steamos:true` → hint "Trên Steam Deck, đặt mật khẩu trước: mở Konsole và chạy passwd".
  - `InstallService` rejects with `PKEXEC_FAILED` `{command:"sudo …"}` → the message with the command is shown; buttons stay.
  - `kind:""` (Windows) → only the existing retry button.
- [ ] **Step 2: Run** `npx vitest run src/components/ConnectError.test.tsx` — Expected: FAIL.
- [ ] **Step 3: Implement**; strings (vi / en):
  - `service.start` "Khởi động dịch vụ" / "Start the service"; `service.install` "Cài dịch vụ" / "Install the service"; `service.update` "Cập nhật dịch vụ" / "Update the service"; `service.packageUpdate` "Cập nhật Ghostline bằng trình quản lý gói" / "Update Ghostline with your package manager"; `service.steamosHint` "Trên Steam Deck, đặt mật khẩu trước: mở Konsole và chạy passwd" / "On a Steam Deck, set a password first: open Konsole and run passwd".
  - `errors.PKEXEC_FAILED.message` "Không chạy được với quyền quản trị" / "Could not run as administrator"; `.action` "Chạy trong terminal: {{command}}" / "Run in a terminal: {{command}}".
  - `errors.SERVICE_INSTALL_MANUAL.message` "Bản này cài dịch vụ bằng install.sh" / "This build installs the service with install.sh"; `.action` "Chạy: sudo ./install.sh trong thư mục đã giải nén" / "Run: sudo ./install.sh in the extracted folder".
- [ ] **Step 4: Run** `npx vitest run` — Expected: all pass.
- [ ] **Step 5: Commit** `feat(ui): start, install or update the background service from the error page`

### Task 6: `build/linux` — build, shared files, `tar.gz`

**Files:**
- Create: `build/linux/Taskfile.yml`, `build/linux/ghostline.desktop`, `build/linux/io.github.hashcott.ghostline.metainfo.xml`, `build/linux/sysusers.d/ghostline.conf` (`g ghostline -`), `build/linux/tarball/install.sh`, `build/linux/tarball/uninstall.sh`
- Modify: `Taskfile.yml` (include `linux: ./build/linux/Taskfile.yml`)

**Interfaces:**
- Produces tasks `linux:build` (→ `bin/linux/ghostline`, `bin/linux/ghostlined`), `linux:package:tarball` (→ `bin/ghostline-<ver>-linux-amd64.tar.gz`), `linux:package` (all formats; extended by Tasks 7–8). `VERSION` var, default `dev`.
- Tarball layout: `ghostline-<ver>-linux-amd64/{ghostline,ghostlined,install.sh,uninstall.sh,ghostline.desktop,ghostline.png,io.github.hashcott.ghostline.metainfo.xml,LICENSE,NOTICE}` (no unit or sysusers file: `--install-system` embeds the unit and runs `groupadd`)`. Icon: `build/appicon.png` (512) installed to `hicolor/512x512/apps/ghostline.png`.

- [ ] **Step 1:** Write the files. `install.sh`: `set -eu`, must be root, installs GUI/desktop/icon/metainfo under `/usr/local`, daemon copy to `/usr/local/lib/ghostline/ghostlined`, then `./ghostlined --install-system`. `uninstall.sh [--purge]`: `/var/lib/ghostline/bin/ghostlined --uninstall-system [--purge]` (if present), removes the `/usr/local` files.
- [ ] **Step 2: Run** `wails3 task linux:package:tarball VERSION=0.0.0-dev`, `bsdtar -tzf bin/ghostline-0.0.0-dev-linux-amd64.tar.gz`, `desktop-file-validate build/linux/ghostline.desktop`, `appstreamcli validate --no-net build/linux/io.github.hashcott.ghostline.metainfo.xml`, `bash -n build/linux/tarball/*.sh` — Expected: tarball lists the layout above; validators pass. `go version -m bin/linux/ghostline | grep -E 'ldflags|vcs'` shows the version ldflag.
- [ ] **Step 3: Commit** `build(linux): build task, desktop files and the tar.gz with install scripts`

### Task 7: `.deb` and `.rpm` (nfpm)

**Files:**
- Create: `build/linux/nfpm/nfpm.yaml`, `build/linux/nfpm/scripts/{postinst.sh,prerm.sh,postrm.sh}`, `build/linux/nfpm/scripts/scripts_test.sh`
- Modify: `build/linux/Taskfile.yml` (`linux:package:nfpm`, deb + rpm; unit rendered with `sed 's|@DAEMON@|/usr/lib/ghostline/ghostlined|g' internal/sysinstall/ghostline.service.tmpl`)

**Interfaces:**
- nfpm via `go run github.com/goreleaser/nfpm/v2/cmd/nfpm@<pin>`; pin = the newest release at implementation time that is ≥ 2 weeks old (record it in the Taskfile and the ledger).
- Scripts receive deb args (`configure`, `remove`, `upgrade`, `purge`) or rpm counts (`1`, `2`, `0`).

- [ ] **Step 1: Failing test** `scripts_test.sh` (PATH with stub `systemctl`, `systemd-sysusers`, `ghostlined` that log calls; ghostlined path overridable by `GHOSTLINED` env in the scripts): 
  - `postinst.sh configure` (fresh, deb: `$2` empty) → `systemd-sysusers`, `daemon-reload`, `enable --now ghostline.service`.
  - `postinst.sh configure 0.6.0` (upgrade) and rpm `postinst.sh 2` → `try-restart`, no `enable --now`.
  - Review Focus 1: `prerm.sh upgrade` and rpm `prerm.sh 1` → no `--remove-certs`, no `disable`.
  - `prerm.sh remove` and rpm `prerm.sh 0` → `ghostlined --remove-certs` then `disable --now ghostline.service`.
  - `postrm.sh purge` → removes `$ROOT/var/lib/ghostline`, `$ROOT/var/log/ghostline` (`ROOT` env for the test, default empty).
- [ ] **Step 2: Run** `bash build/linux/nfpm/scripts/scripts_test.sh` — Expected: FAIL (scripts missing).
- [ ] **Step 3: Implement** scripts and `nfpm.yaml` (contents per spec §7.3, dependencies per Global Constraints, `overrides.deb.recommends`/`overrides.rpm.recommends`).
- [ ] **Step 4: Run** the scripts test; `wails3 task linux:package:nfpm VERSION=0.0.0-dev`; `bsdtar -tf` on both packages lists `usr/bin/ghostline`, `usr/lib/ghostline/ghostlined`, `usr/lib/systemd/system/ghostline.service`, `usr/lib/sysusers.d/ghostline.conf`, desktop, icon, metainfo — Expected: PASS / listed.
- [ ] **Step 5: Commit** `build(linux): deb and rpm packages with service scripts`

### Task 8: AppImage

**Files:**
- Create: `build/linux/appimage/AppRun`, `build/linux/appimage/runtime-x86_64` (vendored AppImage type2 runtime), `build/linux/appimage/RUNTIME.md` (source URL, release, SHA-256, license)
- Modify: `build/linux/Taskfile.yml` (`linux:package:appimage`: AppDir → `mksquashfs AppDir app.squashfs -root-owned -noappend -comp zstd` → `cat runtime-x86_64 app.squashfs > bin/Ghostline-<ver>-x86_64.AppImage`, `chmod +x`; task checks `sha256sum` of the runtime against `RUNTIME.md` before use)

- [ ] **Step 1:** Download the type2 runtime from `github.com/AppImage/type2-runtime/releases` (a tagged release if one exists, else `continuous`), record URL + SHA-256 in `RUNTIME.md`, commit the binary. `AppRun`: `ldconfig -p | grep -q libwebkitgtk-6.0.so.4` else print (and `notify-send` if present) "Ghostline needs WebKitGTK 6.0: sudo apt install libwebkitgtk-6.0-4 | sudo dnf install webkitgtk6.0 | sudo pacman -S webkitgtk-6.0", exit 1; otherwise `exec "$APPDIR/usr/bin/ghostline" "$@"`.
- [ ] **Step 2: Run** (needs `squashfs-tools`; in CI, or locally after the maintainer installs it) `wails3 task linux:package:appimage VERSION=0.0.0-dev`; `./bin/Ghostline-0.0.0-dev-x86_64.AppImage --appimage-extract` lists `AppRun usr/bin/ghostline usr/lib/ghostline/ghostlined ghostline.desktop ghostline.png` — Expected: listed. `bash -n build/linux/appimage/AppRun`.
- [ ] **Step 3: Commit** `build(linux): AppImage on the system's GTK4 and WebKitGTK`

### Task 9: PKGBUILD

**Files:**
- Create: `build/linux/aur/PKGBUILD` (`@VERSION@`, `@SHA256@`), `build/linux/aur/ghostline.install` (`post_install`, `post_upgrade` → `try-restart`, `pre_remove` → `--remove-certs` + `disable --now`; no `pre_upgrade` action)
- Create: `build/linux/aur/render.sh <version> <tarball>` (prints PKGBUILD with values; `--local` switches `source` to the file path for CI)

- [ ] **Step 1:** Write the files; `package()` installs into the paths of spec §7.3 (unit rendered with `PackageDaemon`), plus `/usr/share/licenses/ghostline-bin/`.
- [ ] **Step 2: Run** locally (Arch host): `wails3 task linux:package:tarball VERSION=0.0.0-dev`, `build/linux/aur/render.sh 0.0.0-dev bin/ghostline-0.0.0-dev-linux-amd64.tar.gz --local > $SP/aur/PKGBUILD`, copy `ghostline.install` and the tarball, `cd $SP/aur && makepkg --nodeps` — Expected: `ghostline-bin-0.0.0_dev-1-x86_64.pkg.tar.zst` built; `bsdtar -tf` lists the paths. (Do not install it.)
- [ ] **Step 3: Commit** `build(linux): PKGBUILD for ghostline-bin`

### Task 10: CI — package, install, remove

**Files:**
- Modify: `.github/workflows/ci.yml` (job `linux`: install `squashfs-tools`, `wails3 task linux:package VERSION=0.0.0-ci`, `bash build/linux/nfpm/scripts/scripts_test.sh`, `bash build/linux/ci/smoke.sh deb`, `bash build/linux/ci/smoke.sh tarball`, upload tarball artifact; new job `pkgbuild`)
- Create: `build/linux/ci/smoke.sh`

**Interfaces:**
- `smoke.sh deb`: `sudo apt-get install -y ./bin/ghostline_*_amd64.deb` → `systemctl is-active ghostline` → `sudo /usr/lib/ghostline/ghostlined status` exits 0 → `sudo apt-get remove -y ghostline` → `systemctl is-active` fails; no `/run/systemd/resolved.conf.d/*ghostline*`, no `nft list table inet ghostline`, no `ghostline*` in `/usr/local/share/ca-certificates`, `/etc/pki/ca-trust/source/anchors` or `/etc/ca-certificates/trust-source/anchors`, no `ghostline` in `ufw status` → `sudo apt-get purge -y ghostline` → `/var/lib/ghostline` and `/var/log/ghostline` absent.
- `smoke.sh tarball`: extract → `sudo ./install.sh` → active + status → `sudo ./uninstall.sh --purge` → same clean checks.
- `pkgbuild` job: `container: archlinux:base-devel`, `needs: linux`, downloads the tarball artifact, `render.sh … --local`, `useradd -m builder`, `makepkg --nodeps` as builder.

- [ ] **Step 1:** Write `smoke.sh` (`set -eu`, each check prints what it checks and fails loudly) and the workflow changes.
- [ ] **Step 2: Run** locally what can run without root: `bash -n build/linux/ci/smoke.sh`; `actionlint` if available (else `python3 -c 'import yaml,sys; yaml.safe_load(open(".github/workflows/ci.yml"))'`) — Expected: clean. Real run happens on push (Task 13 reports it).
- [ ] **Step 3: Commit** `ci(linux): build packages, install and remove them on the runner, makepkg in Arch`

### Task 11: Release workflow

**Files:**
- Modify: `.github/workflows/release.yml`

**Interfaces:**
- Jobs: `ci` (unchanged) → `windows` (former `release`; keeps build, package, size guard; drops checksums and `gh release create`; `actions/upload-artifact` of `*installer.exe`, `*portable.zip`) ‖ `linux` (ubuntu-24.04; deps as CI; `wails3 task linux:package VERSION=<tag without v>`; uploads `bin/*.deb bin/*.rpm bin/*.AppImage bin/*.tar.gz`) → `publish` (ubuntu; downloads all; `sha256sum * > SHA256SUMS`; `build/linux/aur/render.sh <ver> ghostline-<ver>-linux-amd64.tar.gz > PKGBUILD`; copies `ghostline.install`; `gh release create <tag> <files> PKGBUILD ghostline.install SHA256SUMS --generate-notes`).

- [ ] **Step 1:** Edit the workflow.
- [ ] **Step 2: Run** YAML check as in Task 10; `git diff` shows Windows build/package/size-guard steps byte-identical except the removed checksum/release steps and the added upload — Expected: clean.
- [ ] **Step 3: Commit** `ci(release): Linux packages and one SHA256SUMS for both platforms`

### Task 12: Documentation and release checklist

**Files:**
- Modify: `README.md`, `README.vi.md`, `docs/user-guide.md`, `docs/huong-dan-su-dung.md`, `docs/platforms.md`, `docs/release-checklist.md`

- [ ] **Step 1:** Write per spec §9 (install per distro, `ghostline` group, known limitations incl. "SteamOS chưa kiểm chứng", SteamOS section marked unverified, platforms rows for packaging / system install / start with the system, checklist rows incl. Firefox snap CA purge and the optional Steam Deck spike).
- [ ] **Step 2: Run** `grep -n 'build/linux/ghostline.service' -r docs README*` — Expected: no stale references.
- [ ] **Step 3: Commit** `docs: install Ghostline on Linux, known limits, Linux release checklist`

### Task 13: Hardened root run and CI result

**Files:**
- Create (scratchpad, not committed): `<SP>/l5run/run.sh` and the binaries it needs

- [ ] **Step 1:** Build `tar.gz` (`VERSION=0.0.0-l5`); write `run.sh`: record state → `install.sh` → `systemctl show ghostline -p CapabilityBoundingSet,NoNewPrivileges` → root tests against the installed daemon → `ghostlined connect` as the user with DPI, system proxy, Fake SNI, LAN sharing (settings as in the previous runs) → check queue portid = nfqws2 pid, CA in trust, KDE proxy, ufw rule → `systemctl kill -s KILL ghostline` → DNS back within 5 s → `uninstall.sh --purge` → compare state. Ask the maintainer to run `sudo bash <SP>/l5run/run.sh`.
- [ ] **Step 2:** Read the log. A missing capability → add it to `internal/sysinstall/ghostline.service.tmpl` with a test assertion and a line in the spec §4 naming why; rerun. Push the branch only with the maintainer's consent; read the CI result of Task 10 and fix failures.
- [ ] **Step 3: Commit** (only if the template changed) `fix(sysinstall): <capability> is needed for <feature>`
