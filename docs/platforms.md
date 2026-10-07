# Platforms

How Ghostline does each OS-specific job on each OS, for maintainers. Edit this table in the same commit that changes a row.

- Every interface below is chosen in one place, `internal/platform/platform_<os>.go`.
- Shared logic lives in each package's unsuffixed files.
- OS calls live only in `_windows.go` and `_linux.go` files.
- Each `Unsupported` stub fails anything that would change the system, and treats removing or restoring as a no-op that succeeds.
- The Linux plan is in `docs/superpowers/specs/2026-10-07-ghostline-linux-design.md`. Steps L2–L5 replace the stubs.

| Job | Windows | Linux | Tests |
|---|---|---|---|
| Data paths | `%APPDATA%\Ghostline`, the portable `data\` folder, and `%ProgramData%\Ghostline` for the LAN CA (`internal/store/paths.go`, wired in `internal/platform/platform_windows.go`) | `~/.config/Ghostline` for the dev build (`internal/platform/platform_linux.go`); `/var/lib/ghostline/data` from L2 | `internal/platform/platform_test.go` |
| State lock | Named mutex `Local\Ghostline-State` (`internal/winutil/mutex_windows.go`) | `flock` on `state.lock` (`internal/platform/flock_linux.go`) | `internal/platform/flock_linux_test.go` |
| System DNS | IP Helper and netsh per adapter (`internal/sysdns/api_windows.go`) | stub (L3): NetworkManager, systemd-resolved or `resolv.conf` | `internal/sysdns/manager_test.go`, `internal/sysdns/unsupported_test.go` |
| Network change watch | `NotifyIpInterfaceChange` (`internal/sysdns/watch_windows.go`) | stub (L3): netlink, D-Bus and inotify | — |
| System proxy | WinINET options and a registry watch (`internal/sysproxy/api_windows.go`, `internal/sysproxy/watch_windows.go`) | stub (L4): GNOME and KDE settings, through the session agent | `internal/sysproxy/sysproxy_test.go`, `internal/sysproxy/unsupported_test.go` |
| Cert store | Crypt32 LocalMachine Root (`internal/certstore/api_windows.go`) | stub (L4): system anchors, Firefox policy, NSS | `internal/certstore/store_test.go`, `internal/certstore/unsupported_test.go` |
| Firewall | netsh rules (`internal/firewall/netsh.go`, `internal/firewall/netsh_windows.go`) | stub (L4): firewalld or ufw | `internal/firewall/netsh_test.go`, `internal/watchdog/rules_test.go` |
| DPI runner and filter | Hidden process in a kill-on-close job, plus WinDivert services (`internal/dpi/runner_windows.go`) | stub (L4): `nfqws2` with an nftables table | `internal/dpi/dpi_test.go`, `internal/dpi/unsupported_test.go` |
| DPI engine files | GoodbyeDPI and zapret2 (`assets/goodbyedpi/embed_windows.go`, `assets/zapret2/embed_windows.go`) | none yet (L4: `nfqws2`) | `assets/zapret2/embed_windows_test.go` |
| Autostart and recovery task | Task Scheduler (`internal/startup/tasks_windows.go`) | stub (L2/L3): systemd unit and boot restore | `internal/startup/startup_test.go`, `internal/startup/xml_test.go` |
| Watchdog | Detached `--watchdog` process (`internal/platform/platform_windows.go`) | stub (L2): systemd `ExecStopPost=--restore` | `internal/watchdog/recover_test.go` |
| Recovery wiring | `Deps.Recovery` (`internal/platform/recovery.go`), shared by the GUI and the headless modes | same file | `internal/platform/recovery_test.go` |
| Secrets | DPAPI, user and machine scope (`internal/secrets/dpapi_windows.go`) | stub (L3): AES-GCM with a root-only key file | `internal/secrets/secrets_test.go`, `internal/secrets/dpapi_windows_test.go` |
| Network key and Wi-Fi name | IP Helper, `SendARP` and WLAN API (`internal/netid/netid_windows.go`) | stub (L4): netlink and NetworkManager | `internal/netid/netid_test.go`, `internal/netid/lanip_test.go` |
| Port owners, process liveness, service stop | Win32 and the service control manager (`internal/procs/procs_windows.go`) | stub (L3): `/proc` and systemd | `internal/procs/procs_test.go` |
| GUI preflight and startup errors | WebView2 check and message box (`internal/shell/preflight_windows.go`) | none; errors go to stderr (`internal/shell/preflight_linux.go`) | — |
| Window behaviour | Stays in the tray when closed; handles logoff, shutdown and resume (`internal/shell/winopts_windows.go`) | Closing the window quits the GUI (`internal/shell/winopts_linux.go`) | `internal/shell/wndproc_windows_test.go` |
| Dependency guard | `tools/depcheck/rules.go` checks both OSes | same | `tools/depcheck/depcheck_test.go` |
| Windows exe size guard | `tools/sizecheck/main.go`, run in `.github/workflows/release.yml` | — | `tools/sizecheck/sizecheck_test.go` |
