<div align="center">

<img src="build/appicon.png" width="96" alt="Ghostline logo">

# Ghostline

**One-click encrypted DNS and DPI bypass for Windows and Linux.**

[![CI](https://github.com/hashcott/ghostline/actions/workflows/ci.yml/badge.svg)](https://github.com/hashcott/ghostline/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/hashcott/ghostline?include_prereleases)](https://github.com/hashcott/ghostline/releases)
[![Downloads](https://img.shields.io/github/downloads/hashcott/ghostline/total)](https://github.com/hashcott/ghostline/releases)
[![License: GPL v3](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64%20%7C%20Linux%20x86__64-blue)

English · [Tiếng Việt](README.vi.md)

<a href="https://github.com/hashcott/ghostline/releases/latest/download/ghostline-amd64-installer.exe"><img src="https://img.shields.io/badge/%E2%AC%87%20Download-Windows%2010%2F11%20x64-2ea44f?style=for-the-badge" alt="Download Ghostline for Windows"></a>

</div>

---

Ghostline runs a local DNS server on `127.0.0.1` / `::1`, points every network adapter at it, and forwards your queries over **DoH, DoT, DoQ or DNSCrypt** to the fastest healthy resolver. When your network interferes with encrypted connections by inspecting packets (DPI), it can also run a DPI bypass engine: **zapret2** (recommended) or **GoodbyeDPI**. Above all, it is built to **always give your original DNS back**, even if the app crashes or the machine loses power.

<p align="center">
  <img src="docs/screenshots/simple-en.png" height="360" alt="Simple interface">
  &nbsp;
  <img src="docs/screenshots/overview-en.png" height="360" alt="Full interface">
</p>

## Table of contents

- [Features](#features)
- [Video](#video)
- [Screenshots](#screenshots)
- [Install](#install)
- [Usage](#usage)
- [How it works](#how-it-works)
- [Privacy](#privacy)
- [Known limitations](#known-limitations)
- [Building from source](#building-from-source)
- [Project layout](#project-layout)
- [Contributing](#contributing)
- [Security](#security)
- [Disclaimer](#disclaimer)
- [License and credits](#license-and-credits)
- [Donate](#donate)

## Features

- **Encrypted DNS for the whole system:** DoH, DoT, DoQ and DNSCrypt upstreams, powered by [AdGuard dnsproxy](https://github.com/AdguardTeam/dnsproxy).
- **Automatic server choice:** scans resolvers in parallel, rejects poisoned answers, and remembers the best servers per network.
- **Never lose the internet:** each adapter's original DNS is snapshotted before any change, with five recovery layers: clean disconnect, a watchdog process, restore on next launch, a logon recovery task, and a network guard that works even if an antivirus quarantines `ghostline.exe`.
- **Leak verification:** after connecting, Ghostline checks that queries really go through it.
- **DPI bypass with two engines:** bundled, hash-pinned [zapret2](https://github.com/bol-van/zapret2) v1.0.5.2 (fake packets, more split methods, QUIC for YouTube/Google) and GoodbyeDPI 0.2.3rc3. zapret2 strategies come from a signed list refreshed daily, with auto-tune, a site blacklist, automatic detection of blocked sites, and DoH request fragmentation. If antivirus blocks zapret2, Ghostline falls back to GoodbyeDPI and offers to retry.
- **Local proxy (HTTP / HTTPS / SOCKS4/5):** runs with Connect, can become the Windows system proxy, and can be shared with phones and other devices on your Wi-Fi (QR code included). Names are always resolved through Ghostline's encrypted DNS.
- **Web fragmentation without a driver:** traffic through the proxy gets its TLS ClientHello split automatically when a site is blocked by SNI, and the fix is remembered per network.
- **Rules and community lists:** block, allow, fake DNS, fragment or route through an upstream proxy by domain, keyword, regexp or CIDR. Import hosts, AdBlock/AdGuard, dnsmasq, Unbound, RPZ, Clash, v2ray, sing-box or CIDR lists straight from a GitHub link, updated on a schedule.
- **DNS server for your home network:** encrypted DNS for phones, TVs, consoles and routers on your Wi-Fi: plain DNS on port 53 (no certificate needed) or DNS-over-HTTPS, with a QR-code setup page and an iOS profile.
- **Fake SNI (advanced, off by default):** for sites behind CDNs that allow domain fronting, the proxy sends a different, allowed domain name to the network. It decrypts HTTPS only for domains you choose, with a certificate that can sign only those domains and is removed on disconnect.
- **Diagnostic tools:** DNS lookup that compares sources and spots DNS poisoning, an advanced scanner that grades servers on latency, loss, DNSSEC, ad filtering and poisoning, a Cloudflare clean-IP finder that turns results into `ip=` rules, and a DNS stamp reader and builder.
- **Backup and restore:** export settings, rules, lists and your servers to one file and import them on another PC. Private and machine-bound values are never exported, and an import never turns on Fake SNI or sharing on the LAN.
- **Signed server list:** updated daily and verified with ed25519; the DNSCrypt list is checked with minisign.
- **Simple and Full interfaces**, a tray icon, Vietnamese and English UI, and a neon-terminal look.
- **Installer or portable:** the portable build keeps all data in a `data\` folder next to the exe.
- **Update notifications only:** Ghostline tells you about a new version and never updates itself silently.

## Video

A narrated walkthrough of a little over 3 minutes: one-click connect, protection levels, servers, DPI bypass, the proxy, rules, the DNS server with the iPhone setup, Fake SNI, the diagnostic tools and backup. The preview below plays sped up and silent; click it for the full video with sound. A [Vietnamese version](docs/videos/guide-vi.mp4) is also available.

<p align="center">
  <a href="docs/videos/guide-en.mp4"><img src="docs/videos/guide-en.webp" width="720" alt="Ghostline video guide (sped up); click for the full video"></a>
</p>

**Short narrated clips, one per feature:** [Connect](docs/videos/clips/connect-en.mp4) · [Protection levels](docs/videos/clips/levels-en.mp4) · [Servers](docs/videos/clips/servers-en.mp4) · [DPI bypass](docs/videos/clips/dpi-en.mp4) · [Proxy](docs/videos/clips/proxy-en.mp4) · [Rules](docs/videos/clips/rules-en.mp4) · [DNS server and iPhone](docs/videos/clips/dnsserver-en.mp4) · [Fake SNI](docs/videos/clips/fakesni-en.mp4) · [Tools](docs/videos/clips/tools-en.mp4) · [Test domain and backup](docs/videos/clips/backup-en.mp4) · [Disconnect](docs/videos/clips/disconnect-en.mp4). The [user guide](docs/user-guide.md) shows each one next to its step-by-step instructions.

## Screenshots

| Servers | DPI bypass |
| --- | --- |
| ![Servers](docs/screenshots/servers-en.png) | ![DPI bypass](docs/screenshots/dpi-en.png) |
| **Proxy** | **Rules and lists** |
| ![Proxy](docs/screenshots/proxy-en.png) | ![Rules](docs/screenshots/rules-en.png) |
| **Logs** | **Settings** |
| ![Logs](docs/screenshots/logs-en.png) | ![Settings](docs/screenshots/settings-en.png) |
| **DNS server** | **Fake SNI** |
| ![DNS server](docs/screenshots/dnsserver-en.png) | ![Fake SNI](docs/screenshots/fakesni-en.png) |
| **Lookup** | **Cloudflare IP** |
| ![Lookup](docs/screenshots/lookup-en.png) | ![Cloudflare IP](docs/screenshots/cfscan-en.png) |

## Install

**[⬇ Download the latest release](https://github.com/hashcott/ghostline/releases/latest)** — or pick a file below. Older versions are on the [Releases](https://github.com/hashcott/ghostline/releases) page.

| File | What it is | Download |
| --- | --- | --- |
| `ghostline-amd64-installer.exe` | Installer (installs WebView2 if missing) | **[Direct download ⬇](https://github.com/hashcott/ghostline/releases/latest/download/ghostline-amd64-installer.exe)** |
| `Ghostline-<version>-portable.zip` | Portable: unzip and run | [From latest release](https://github.com/hashcott/ghostline/releases/latest) |
| `SHA256SUMS` | Checksums for both | [Direct download](https://github.com/hashcott/ghostline/releases/latest/download/SHA256SUMS) |

Verify the download:

```powershell
Get-FileHash .\Ghostline-0.1.0-portable.zip -Algorithm SHA256
```

**Requirements:** Windows 10/11 x64 and administrator rights. Changing adapter DNS and loading the WinDivert driver both need admin. When Ghostline starts with Windows it runs through Task Scheduler, so there is no UAC prompt.

> [!NOTE]
> Releases are not code-signed yet, so SmartScreen shows "Windows protected your PC". After checking the SHA-256, choose **More info → Run anyway**.
> Some antivirus products flag the WinDivert driver used by zapret2 and GoodbyeDPI. Ghostline verifies the engine's hash before every start; if your antivirus blocks zapret2, Ghostline runs GoodbyeDPI for now and the DPI page shows the `bin\zapret2` folder to add to the exclusions.

### Linux (x86_64)

| Distribution | File | Install |
| --- | --- | --- |
| Ubuntu 24.04+, Debian 13+ | `ghostline_<version>_amd64.deb` | `sudo apt install ./ghostline_<version>_amd64.deb` |
| Fedora 41+ | `ghostline-<version>-1.x86_64.rpm` | `sudo dnf install ./ghostline-<version>-1.x86_64.rpm` |
| Arch, CachyOS, Manjaro | `ghostline-<version>-linux-amd64.tar.gz` | Unpack it and run `sudo ./install.sh`, or build `ghostline-bin` from the `PKGBUILD` attached to the release (`makepkg -si`) |
| Other distributions | `Ghostline-<version>-x86_64.AppImage` | `chmod +x` and run it; it offers to install the background service |

Verify the downloads: `sha256sum -c SHA256SUMS --ignore-missing`.

Ghostline on Linux is two parts: a background service (`ghostline.service`, root) that changes DNS, runs zapret2 and keeps protecting with the window closed and after a reboot, and the window, which runs as your user. Members of `wheel`, `sudo`, `admin` or the `ghostline` group can control it; to allow another account: `sudo usermod -aG ghostline <user>`, then log in again. The deb and rpm start the service right away; on Arch start it once with `sudo systemctl enable --now ghostline` (or the button in the window).

**Requirements:** systemd, GTK 4 and WebKitGTK 6.0 (the packages pull them in; the AppImage and the tar.gz use the system's).

**Uninstall:** `sudo apt remove ghostline` (`apt purge` also deletes the settings), `sudo dnf remove ghostline`, `sudo pacman -R ghostline-bin`; tar.gz and AppImage: `sudo ./uninstall.sh [--purge]` or `sudo /var/lib/ghostline/bin/ghostlined --uninstall-system [--purge]`. DNS, the system proxy, certificates, firewall rules and the nftables table are restored first. rpm and Arch keep the settings: `sudo rm -rf /var/lib/ghostline /var/log/ghostline` removes them.

## Usage

> 📖 A detailed user guide covering every screen, unblocking sites and troubleshooting: **[docs/user-guide.md](docs/user-guide.md)** ([Tiếng Việt](docs/huong-dan-su-dung.md))

1. Start Ghostline and press **Connect**. It picks a server, redirects DNS and verifies there is no leak.
2. If some sites are still blocked, either turn on the **proxy** (Full → Proxy → enable proxy + use for this PC) so browsers get automatic fragmentation, or open **Full → DPI bypass**, pick an engine (**zapret2** is recommended), turn it on and press **auto-tune**.
   To share with other devices, turn on **share on LAN** and scan the QR code on your phone (the network must be *Private*).
3. Press **Disconnect** (or quit from the tray) to restore your original DNS.

If DNS ever looks wrong, **Settings → Restore DNS now** puts every adapter back to its saved state. From a terminal you can also run:

```powershell
ghostline.exe --restore
```

## How it works

```
apps ──► Windows DNS client ──► 127.0.0.1:53 (Ghostline / dnsproxy) ──► DoH · DoT · DoQ · DNSCrypt
                                         │
            zapret2 / GoodbyeDPI (optional) rewrites outgoing TLS/HTTP/QUIC to dodge SNI filtering
```

**Safety net.** Before touching an adapter, Ghostline writes a snapshot (`state.json`) of its DNS. Five layers make sure that snapshot gets restored:

1. **Clean disconnect:** the normal path.
2. **Watchdog:** a separate `--watchdog` process restores DNS within seconds if the app dies.
3. **Next launch:** a leftover snapshot is restored at startup.
4. **Logon task:** the `Ghostline Recovery` scheduled task runs `--restore` after a crash or power loss.
5. **Network guard:** while connected, the `Ghostline Network Guard` task runs a PowerShell script as SYSTEM every minute, at boot and right after Microsoft Defender acts on a threat. If Ghostline is gone but DNS still points at 127.0.0.1, it restores DNS and the system proxy from `state.json`. It does not need `ghostline.exe`, so it still works when an antivirus kills and quarantines the app along with the layers above.

Design details live in [`docs/superpowers/specs`](docs/superpowers/specs).

## Privacy

- No telemetry, no accounts, no analytics.
- Visited domains are never written to disk. The optional query log lives in RAM only.
- Network access is limited to your chosen DNS resolvers, bootstrap resolution of their hostnames, the signed server list, the DNSCrypt resolver list, the GitHub release check, and the community lists you add yourself.
- The proxy never logs destinations to disk; its live connection view is RAM-only like the query log.

## Known limitations

- If you approve UAC with a **different administrator account**, `%APPDATA%` and the system proxy belong to that account, so "use for this PC" does not affect the signed-in user.
- Web fragmentation only helps apps that go through the proxy. Apps that ignore the Windows proxy (some games, Firefox with its own proxy settings) need the DPI engine instead.
- LAN sharing works only on networks marked **Private** in Windows; Ghostline never changes the network profile itself.

- **Fake SNI** works only for browsers on this PC going through the proxy, only for domains with an `sni=` rule, and breaks apps that pin certificates. Firefox may need `security.enterprise_roots.enabled`.
- **Devices using this PC's DNS lose the internet when it is off or disconnected.** iOS has no fallback: with the DoH profile, the iPhone has no internet on your home Wi-Fi until you choose *Automatic* in *Settings › General › VPN & Device Management › DNS* (mobile data is not affected). Disconnect asks first while LAN devices use the DNS server; if this PC is often off, set the iPhone's DNS manually instead of using the profile. Android's Private DNS cannot use Ghostline; set a static DNS for your Wi-Fi instead.

### Linux

- GNOME has no tray by default: closing the window quits it, and protection keeps running in the background service.
- The system proxy is set automatically only on GNOME and KDE; on other desktops set `127.0.0.1:<port>` by hand.
- Fake SNI in Chrome and Chromium on Ubuntu and Debian needs `libnss3-tools` (`certutil`).
- A kernel without NFQUEUE (`nfnetlink_queue`, `nft_queue`) has no DPI bypass; the proxy's fragmentation still works.
- No GoodbyeDPI (it is Windows-only); zapret2 is the engine.
- Packages are not GPG-signed yet: check `SHA256SUMS`.
- SteamOS (Steam Deck) is not verified yet.

## Building from source

**Prerequisites:** Go 1.26+, Node.js 24, [Wails v3](https://v3.wails.io) `v3.0.0-beta.27`, and [NSIS](https://nsis.sourceforge.io) for the installer.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27

wails3 dev                     # live-reload dev build (connecting for real needs admin)
wails3 build                   # bin/ghostline.exe
wails3 package                 # NSIS installer
wails3 task windows:portable   # portable zip
```

On Linux (GTK 4 and WebKitGTK 6.0 development packages, plus `squashfs-tools` for the AppImage):

```bash
wails3 task linux:build                     # bin/linux/ghostline and bin/linux/ghostlined
wails3 task linux:package VERSION=0.6.0     # deb, rpm, AppImage and tar.gz in bin/
```

**Tests:**

```bash
go test ./...
cd frontend && npm test
```

Integration tests change real system settings, so run them in an **admin** terminal:

```bash
go test -tags integration ./internal/sysdns/... ./internal/startup/...
```

Before a release, go through [`docs/release-checklist.md`](docs/release-checklist.md). Pushing a `v*` tag builds and publishes the release through GitHub Actions.

## Project layout

| Path | Purpose |
| --- | --- |
| `internal/app` | Orchestrator: connect, disconnect, health checks, DPI, recovery |
| `internal/engine` | Local DNS server built on dnsproxy |
| `internal/sysdns` | Read, apply and restore adapter DNS (Win32 + netsh fallback) |
| `internal/watchdog`, `internal/startup` | Watchdog process and scheduled tasks |
| `internal/dpi` | DPI engines (zapret2, GoodbyeDPI), signed strategy list, WinDivert service handling |
| `internal/scanner`, `internal/probe` | Server latency scan and blocked-site probes |
| `internal/servers`, `internal/upstreams` | Signed server list and DNSCrypt list |
| `internal/shell` | Window, tray and OS events (Wails) |
| `frontend/` | React + TypeScript UI |
| `lists/servers.json` | Signed server list, regenerated weekly by CI |

## Contributing

Bug reports, translations and pull requests are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) first.

## Security

Please do **not** open a public issue for vulnerabilities. See [SECURITY.md](SECURITY.md).

## Disclaimer

Ghostline is provided for research and educational purposes, to study encrypted DNS, network filtering and DPI. Its main goals are privacy (keeping DNS queries from being read or logged), protection against DNS spoofing and hijacking, and network diagnostics. You are solely responsible for how you use it and for complying with the laws and regulations of your country and the terms of your network provider. Do not use Ghostline for any unlawful purpose, including:

- reaching websites, services or content that a competent authority has ordered to be blocked under the law of your country;
- online gambling, copyright infringement, fraud, or spreading content that is prohibited by law;
- attacking, disrupting or getting unauthorized access to any network or system.

Ghostline does not ship, recommend or maintain lists of sites blocked by authorities. Lists and rules you add yourself are your own responsibility.

The software is provided "as is", without warranty of any kind. The authors are not liable for any damage, data loss, service disruption or legal consequences arising from its use. See [LICENSE](LICENSE) for the full terms.

## License and credits

Ghostline is free software, released under the [GNU General Public License v3.0 only](LICENSE). You may use, study, share and modify it; if you distribute a modified version, you must release its source code under the same license. Releases v0.1.0 and v0.1.1 were published under the MIT License.

It stands on the shoulders of [dnsproxy](https://github.com/AdguardTeam/dnsproxy), [zapret2](https://github.com/bol-van/zapret2), [GoodbyeDPI](https://github.com/ValdikSS/GoodbyeDPI), [WinDivert](https://github.com/basil00/WinDivert) and [Wails](https://wails.io), and was inspired by [DNSveil / SecureDNSClient](https://github.com/msasanmh/SecureDNSClient). Third-party licenses are listed in [NOTICE](NOTICE).

## Donate

Ghostline is free and always will be. If you can, please support the projects it is built on first; they do the heavy lifting:

- [zapret2](https://github.com/bol-van/zapret2) by bol-van: the DPI bypass engine
- [GoodbyeDPI](https://github.com/ValdikSS/GoodbyeDPI) by ValdikSS
- [WinDivert](https://github.com/basil00/WinDivert) by basil00
- [dnsproxy](https://github.com/AdguardTeam/dnsproxy) by AdGuard
- [Wails](https://wails.io)

If you would also like to support Ghostline itself, you can send a tip through [PayPal](https://paypal.me/hashcott), stablecoins, or MoMo / VietQR. Thank you!

<p align="center">
  <a href="https://paypal.me/hashcott"><img src="https://img.shields.io/badge/PayPal-hashcott-00457C?logo=paypal&logoColor=white" alt="Donate with PayPal"></a>
</p>

**Stablecoins (USDT or USDC):**

```
0x3C0E297cC77416DA2Ac108F09360d7Bf7C4E2c8e
```

> [!WARNING]
> Send only through **BNB Smart Chain (BEP20)** or **Arc**. Coins sent through any other network, such as Ethereum (ERC20) or Tron (TRC20), will be lost.

<details>
<summary><b>MoMo / VietQR</b> (click to show the QR code)</summary>
<br>
Scan with MoMo or any Vietnamese banking app (VietQR / Napas 247).
<p align="center"><img src="docs/donate-momo.png" width="240" alt="MoMo / VietQR donation QR code"></p>
</details>
