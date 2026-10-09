# Contributing to Ghostline

Thanks for helping! Issues and pull requests are welcome in English or Vietnamese.

## Reporting bugs

Please include:

- Your operating system (Windows version, or Linux distribution and desktop) and Ghostline version.
- What you did, what you expected, and what happened.
- The log: **Advanced → Logs → save file**. Logs never contain visited domains, but check them before posting.
- If DNS was left wrong, the output of `Get-DnsClientServerAddress` on Windows, or `resolvectl status` (or `cat /etc/resolv.conf`) on Linux. On Linux, the service's log helps too: `journalctl -u ghostline -b`.

## Development setup

See [Building from source](README.md#building-from-source). In short:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27
cd frontend && npm ci && cd ..
wails3 dev
```

On Linux the window talks to a separate root service. For development, run the service from your checkout and point the window at its socket:

```bash
go build -o bin/ghostlined ./cmd/ghostlined && go build -o bin/ghostline .
sudo bin/ghostlined --daemon --data-dir /var/tmp/ghostline-dev        # terminal 1
GHOSTLINE_SOCKET=/var/tmp/ghostline-dev/run/ctl.sock bin/ghostline     # terminal 2
```

Stop the service with Ctrl+C: it disconnects and puts the system back. After a hard kill, `sudo bin/ghostlined --restore --data-dir /var/tmp/ghostline-dev` does the same.

## Pull requests

- **Tests first.** Every behavior change or bug fix comes with a test that fails without it.
- **Keep it green:** `go test ./...`, `cd frontend && npm test`, `npx tsc --noEmit`, and `golangci-lint run` for both platforms (`GOOS=windows golangci-lint run` too). CI runs all of them on Windows and Linux, plus `go run ./tools/depcheck` and `go generate ./...` with no diff.
- **System changes need extra care.** Code that touches system DNS, services, scheduled tasks, the firewall, certificates or nftables must keep the safety net intact: snapshot first, and always be able to restore. Run the integration tests in an admin terminal on Windows, and the `integration_root` tests as root on Linux.
- **One place per OS difference.** OS-specific code lives in `_windows.go` / `_linux.go` files behind an interface; shared code never checks `runtime.GOOS`. `internal/platform` is the one place that picks each implementation (see [docs/platforms.md](docs/platforms.md)).
- **Translations:** add every new UI string to both `frontend/src/i18n/vi.json` and `en.json`; a parity test checks this.
- Use [Conventional Commits](https://www.conventionalcommits.org) (`fix(dpi): …`, `feat(ui): …`) and keep each PR to one topic.

## Things we won't merge

- Telemetry, or anything that writes visited domains to disk.
- Code that stops or kills third-party processes or services without the user's explicit consent.
- Changes to the bundled zapret2 or GoodbyeDPI binaries without updating their pinned hashes and NOTICE.
- Lists, presets, default test sites or docs that target websites or services blocked by a competent authority (for example gambling, piracy, or platforms blocked by government order). Ghostline's purpose is privacy and protection against DNS spoofing, not reaching content that is prohibited by law.

## Issues about specific blocked sites

Please don't post issues asking how to reach a specific site or service that is blocked by government order, or share lists of such sites. Those issues will be edited or closed. Describe the network symptom instead (for example "TLS handshake resets on network X") so it can be fixed in a general way.

By contributing, you agree that your contributions are licensed under the [GNU GPL v3.0 only](LICENSE).
