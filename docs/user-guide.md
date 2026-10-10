# Ghostline user guide

This guide is for anyone running Windows 10/11 or Linux; no technical background is needed. Sections 1–3 are enough to get started. The rest covers fine-tuning and troubleshooting.

[Tiếng Việt](huong-dan-su-dung.md)

## Contents

1. [What Ghostline does](#1-what-ghostline-does)
2. [Installing](#2-installing)
3. [Quick start: one button](#3-quick-start-one-button)
4. [Full interface](#4-full-interface)
   - [Overview](#41-overview)
   - [Servers](#42-servers)
   - [DPI bypass](#43-dpi-bypass)
   - [Logs](#44-logs)
   - [Settings](#45-settings)
   - [Proxy](#46-proxy)
   - [Rules and lists](#47-rules-and-lists)
   - [DNS server](#48-dns-server)
   - [Fake SNI](#49-fake-sni)
   - [Tools](#410-tools)
   - [Backup and moving to another PC](#411-backup-and-moving-to-another-pc)
5. [The tray icon](#5-the-tray-icon)
6. [When a site is still blocked](#6-when-a-site-is-still-blocked)
7. [Troubleshooting](#7-troubleshooting)
8. [Uninstalling](#8-uninstalling)
9. [FAQ](#9-faq)

---

## 1. What Ghostline does

Every time you open a website, your computer asks **DNS**: "what is the IP address of this site?" Normally that question travels **unencrypted**, so your ISP can read it, log it, or answer with a wrong address to block the site.

Ghostline runs a small DNS server on your own machine (`127.0.0.1`), points the system's DNS at it (every network adapter on Windows; NetworkManager, systemd-resolved or `/etc/resolv.conf` on Linux), and sends your DNS questions over an **encrypted** channel (DoH, DoT, DoQ or DNSCrypt) to the fastest server available. If your ISP also blocks sites by inspecting packets (DPI), Ghostline can run a DPI bypass engine (**zapret2**, or **GoodbyeDPI** on Windows) to get around it.

Most importantly, **Ghostline always gives your original DNS back** when you disconnect, even if the app crashes or the machine loses power.

## 2. Installing

Download from the [download page](https://hashcott.github.io/ghostline/) (it picks the file for your computer) or from [Releases](https://github.com/hashcott/ghostline/releases).

### Windows

There are two options:

| Build | When to use it |
| --- | --- |
| `ghostline-amd64-installer.exe` | For everyday use on your own PC. Adds a Start Menu shortcut and can be removed from Settings → Apps. |
| `Ghostline-<version>-portable.zip` | When you don't want to install anything, or to run from a USB drive. Unzip it into a folder and run `ghostline.exe`. All data stays in the `data\` folder next to it. |

**Verify the download (recommended).** Open PowerShell in the folder with the downloaded file:

```powershell
Get-FileHash .\ghostline-amd64-installer.exe -Algorithm SHA256
```

Compare the result with the matching line in `SHA256SUMS` on the Releases page. If they differ, **do not run the file**.

**SmartScreen warning.** Releases are not code-signed yet, so Windows shows "Windows protected your PC". After checking the SHA-256, click **More info → Run anyway**.

**Administrator rights.** Ghostline needs admin rights to change DNS and run GoodbyeDPI, so Windows shows a UAC prompt each time you open it. Choose **Yes**. If you turn on *start with windows*, the app is launched through Task Scheduler and no longer asks.

**Antivirus.** zapret2 and GoodbyeDPI use the **WinDivert** driver, which antivirus products often flag by mistake. Ghostline checks the engine's hash before every start. If zapret2 is blocked, Ghostline runs GoodbyeDPI for now and the DPI page shows the `bin\zapret2` folder to add to Windows Defender's exclusions.

**Updating.** You never need to uninstall first. When a new version is out, Ghostline shows **update to vX** on the main screen, in Settings and in the tray. With the installer build, press it and confirm: Ghostline downloads the installer, checks it against the release's signed `SHA256SUMS`, closes, installs the new version and opens again, connecting again if it was connected. If the download or the check fails, the button opens the release page instead. The portable build always opens the release page: unzip the new version over the old folder, `data\` is kept. Your settings, rules and server choices survive every update.

### Linux

Pick the file for your distribution on the Releases page (see the table in the README): a `.deb` for Ubuntu and Debian, an `.rpm` for Fedora, the `tar.gz` (or the attached `PKGBUILD`) for Arch, the AppImage for anything else. Check it with `sha256sum -c SHA256SUMS --ignore-missing`.

- **Two parts.** The background service `ghostline.service` runs as root: it changes DNS, runs zapret2 and the proxy, and keeps protecting with the window closed, after a reboot and while you are logged out. The window runs as your user and only talks to the service. It never needs `sudo`.
- **Who may control it.** Members of `wheel`, `sudo`, `admin` or `ghostline`. For another account: `sudo usermod -aG ghostline <user>`, then log in again. Others see "your account may not control Ghostline".
- **The service is not running.** The window says so and offers **start the service** (deb, rpm, Arch) or **install the service** (AppImage, tar.gz); both ask for your password through the system's dialog. In a terminal: `sudo systemctl enable --now ghostline`.
- **Start with the system.** In Settings, *start with the system* connects at boot (with *connect automatically*) and opens Ghostline in the tray when you log in.
- **Updating.** Install the new `.deb` or `.rpm`, or rebuild `ghostline-bin` from the new `PKGBUILD`. If you installed the service from the AppImage, open the new AppImage: it finds the older service and updates it once you enter your password, and reconnects if you were protected. If you cancel, the banner at the top keeps an **update the service** button. From the tar.gz, unpack the new one and run `sudo ./install.sh` once: it replaces the service and restarts it.

### Steam Deck (SteamOS) — not verified yet

Ghostline has not been tested on a Steam Deck. The intended way, in Desktop Mode:

1. Open Konsole and set a password for the `deck` user once: `passwd`. The service install asks for it.
2. Download the AppImage, make it executable (*Properties → Permissions*) and open it.
3. Press **install the service** and enter the password. Connect.
4. Switch to Game Mode: the service keeps protecting; the window is not needed.

If the AppImage says WebKitGTK 6.0 is missing, this SteamOS image cannot run the window yet; please open an issue with your SteamOS version.

## 3. Quick start: one button

<p align="center"><img src="screenshots/simple-en.png" width="320" alt="Simple interface"></p>
<a href="videos/clips/connect-en.mp4"><img src="videos/clips/connect-en.webp" width="640" alt="Connecting in the Simple interface"></a>

*The preview plays sped up and silent; click it for the narrated video.*

Ghostline opens in the **Simple** interface.

1. Click the **round power button** in the middle.
2. Ghostline goes through: checking the system → choosing servers → starting the engine → saving original DNS → arming safety net → setting DNS → verifying no leaks. To choose servers, Ghostline keeps a ranking of every server for each network. The first time on a network it scans the whole list (about 900 servers, 30–60 seconds; the step shows how far it is, for example *choosing servers 120/906*) and takes the fastest that do not filter content, pinned servers first. After that it connects in a few seconds from the ranking. When the ranking is more than a day old, Ghostline checks its best servers before using them and rebuilds the ranking in the background once connected.
3. When the ring glows green and shows **[ PROTECTED ]**, you're done: every DNS query on the machine is encrypted.

To cancel while connecting, click the power button again. To turn protection off, click the power button while protected; your DNS goes back to what it was.

<a href="videos/clips/disconnect-en.mp4"><img src="videos/clips/disconnect-en.webp" width="640" alt="Disconnecting"></a>

*The preview plays sped up and silent; click it for the narrated video.*

**First connect: auto-tune.** The first time you connect after installing, Ghostline adds one more step to the list: once DPI bypass (if on) is running, it checks the test sites and, if some are still blocked, auto-tunes DPI bypass on them, showing which strategy it is trying (for example *auto-tuning DPI bypass: trying 2/4*). When it finds a working strategy, DPI bypass turns on and the level becomes **DNS + DPI bypass**; if nothing is blocked, the level stays as it is. It runs once; if you disconnect before it finishes, it runs again on the next connect. The levels wait while auto-tune runs.

**Protection level.** Under the status, pick how much Ghostline does. You can change it any time, also while connected:

| Level | What it turns on | When |
| --- | --- | --- |
| **DNS only** | Encrypted DNS | Your ISP blocks with DNS alone; the lightest |
| **DNS + DPI bypass** *(recommended)* | Adds DPI bypass for every app | Sites are still blocked after the DNS change |
| **Maximum** | Adds the proxy for this PC (it sets the system proxy); browsers get fragmented automatically on blocked sites | DPI bypass is not enough for some sites |
| **Custom** | Your own combination from the Full interface | Lit when what you set there matches no level |

Choosing a level also looks for the best servers: Ghostline scans the whole list again (the line under the levels shows *Finding the best servers 120/906…*) and, when connected, switches to the fastest without disconnecting. Click the current level to look again.

<a href="videos/clips/levels-en.mp4"><img src="videos/clips/levels-en.webp" width="640" alt="Choosing a protection level"></a>

*The preview plays sped up and silent; click it for the narrated video.*

A level only changes these switches; your engine, strategy, servers, rules and other settings stay as they are. When you leave **Custom** for a level, Ghostline remembers your combination, and clicking **Custom** brings it back. Fake SNI needs the proxy: a level without the proxy asks before turning it off, and **Custom** turns it back on.

**The info panel below:**

| Row | Meaning |
| --- | --- |
| server | The DNS server in use; `+4` means four more servers run alongside it as backup |
| latency | Average time to get a DNS answer |
| dpi bypass | The DPI bypass preset in use, or *off* |
| uptime | How long you've been protected |

**Statuses you may see:**

| Status | Meaning |
| --- | --- |
| UNPROTECTED | Your machine is using its original DNS |
| CONNECTING | Running the steps above |
| PROTECTED | Everything is working |
| DEGRADED | Servers are slow or not answering; Ghostline is finding new ones by itself. Browsing still works |
| ERROR | Connecting failed. **Your DNS was not changed.** Read the message below it for what to do (see [section 7](#7-troubleshooting)) |

## 4. Full interface

Click **FULL** at the top left for every page and setting, and **SIMPLE** to go back. The left sidebar always shows the current status and a **⏻ CONNECT / DISCONNECT** button.

The sidebar groups the pages: **basic** (overview, servers, DPI bypass), **advanced** (proxy, rules, DNS server) and **diagnostics** (tools), then settings. **Fake SNI** is a tab of the **proxy** page, and **logs** is the first tab of the **tools** page.

### 4.1. Overview

![Overview](screenshots/overview-en.png)

- **Top panel:** status, uptime, and the DNS route: `127.0.0.1 → the servers in use`.
- **latency · 60 seconds:** a latency chart for the last minute. Lower is better.
- **queries:** the number of DNS queries answered since you connected.
- **servers in use:** the servers Ghostline queries in parallel; the fastest answer wins.

### 4.2. Servers

![Servers](screenshots/servers-en.png)

<a href="videos/clips/servers-en.mp4"><img src="videos/clips/servers-en.webp" width="640" alt="Scanning, searching and pinning servers"></a>

*The preview plays sped up and silent; click it for the narrated video.*

**Step by step: use the servers you like**

1. Open **Servers** and click **⟳ scan all**. Wait until the scan finishes.
2. Type a name, provider or protocol in the search box, for example `cloudflare`.
3. Click the **☆** star on the servers you want, or **★ pin N results** to pin every result.
4. If you are connected, click **reconnect to apply**.
5. To use nothing else, turn on **use pinned servers only**.

Every encrypted DNS server Ghostline knows about (several hundred), refreshed daily from a signed list.

- **⟳ scan all:** re-measures every server's latency, drops servers that return wrong (poisoned) answers and, when connected, switches to the fastest without disconnecting. Ghostline also does this by itself when the ranking is more than a day old or you join another network. Servers that filter content (`adblock`, `family`) are measured too but marked *not picked*: Ghostline never chooses them by itself; pin one to use it.
- **What "pass" means:** the server answered the test domain correctly twice within 3 seconds, with a public IP. A pass does not catch a server that blocks only some sites, an ISP that returns fake public IPs, or DNSSEC problems; use **Tools › Scanner** for a closer look. The test domain is set in **Settings**.
- **filter:** pick protocols (`doh`, `dot`, `doq`, `dnscrypt`) or server types:
  - `no-filter`: blocks nothing.
  - `adblock`: blocks ads and trackers.
  - `family`: blocks adult content, good for children's computers.
  - **only ok:** show only servers that are currently working.
- **Click a column header** (name, latency…) to sort.
- **☆ Pin:** click the star (or double-click the row) to pin servers you like. Right-click a row for more: use only this server, check again, copy address/IP, remove. Pinned servers stay at the top of the table and are **preferred when connecting**: every pinned server that passes the check is used first, and the remaining slots go to the fastest others. Turn on **use pinned servers only** (next to the search box) to use nothing else.
- **Search and bulk pin:** type in the search box (name, provider, protocol, address, IP or tag; several words must all match), then **★ pin all results**. The **★ pinned (N)** chip shows only pinned servers; **unpin all** clears them. If you change pins while connected, press **reconnect to apply**.
- **+ add:** add your own servers. Paste URLs (`https://…`, `tls://…`, `quic://…`) or `sdns://…` stamps, one per line, or import them from a file. Servers you added have an **✕** button to remove them.
- **state:** *in use* (receiving queries), *ok* (working), *not checked*.

### 4.3. DPI bypass

![DPI bypass](screenshots/dpi-en.png)

<a href="videos/clips/dpi-en.mp4"><img src="videos/clips/dpi-en.webp" width="640" alt="The DPI bypass page"></a>

*The preview plays sped up and silent; click it for the narrated video.*

**Step by step: get a blocked site to open**

1. Click **Connect** first; the engine only runs while you are connected.
2. On **DPI bypass**, turn the switch on and keep the **zapret2** engine.
3. Add the site to **Test sites** (one per line) and click **⟳ test again**.
4. A **✕ TLS** result usually means DPI: click **⚡ auto-tune** and wait. Ghostline keeps the lightest strategy that opens every test site.
5. If only a few sites are blocked, set **scope** to **blacklist** and list just those sites.

Use this when DNS is encrypted but connections to a site are **still interfered with**: equipment on the path reads the site name inside your traffic (SNI) and resets the connection.

> ⚖️ You are responsible for complying with the law and your network provider's terms. Do not use these features to reach content that is prohibited by law. See the [Disclaimer](../README.md#disclaimer).

**DPI bypass for every app**

- **Switch:** turns it on or off. The engine only runs while Ghostline is **connected**:
  - `● zapret2 started (preset …)`: working.
  - `○ starting…`: waiting a few seconds for packet capture to start (the WinDivert driver on Windows, the nftables queue on Linux).
  - `○ Enabled — starts when connected`: switched on, but you're not connected yet.
- **engine:**
  - **zapret2 (recommended):** stronger, with fake packets, more split methods and QUIC support (YouTube, Google). Ghostline refreshes its signed strategy list daily, no new release needed.
  - **GoodbyeDPI** (Windows): the previous engine. Installs from before zapret2 keep GoodbyeDPI until you switch.
  - On Windows, if antivirus blocks zapret2, Ghostline runs GoodbyeDPI instead, shows *degraded* and offers **retry zapret2**.
- **preset:** how aggressively packets are modified.
  - **Light → Medium → High → Extreme:** higher levels get past more blocks but may slow down or break some sites. Start with **Light**.
  - **Mode 1–6** (GoodbyeDPI only): GoodbyeDPI's built-in modes; try them when the levels above don't help.
  - **Custom:** enter your own arguments. zapret2 only accepts `--lua-desync=…` calls to the built-in functions; Ghostline rejects dangerous flags.
- **⚡ auto-tune:** Ghostline tries each preset from lightest to strongest and keeps the lightest one that opens every *test site*. You must **connect first**. Click again to cancel.
- **scope:**
  - **all connections:** applies to every site.
  - **blacklist:** applies only to domains on the list. Click **edit ›**, enter one domain per line, then **save**. This affects other sites the least.
- **detect blocked sites automatically** (zapret2, blacklist scope): zapret2 notices blocked sites and adds them to a separate list shown below; you can remove any of them.
- **command line:** shows exactly what the engine will run.

**DNS fragment**

Splits the packets sent to DoH servers into pieces so the ISP has a harder time recognising them. You only need it when **no servers can be found** (your ISP blocks encrypted DNS itself). It is redundant while a DPI engine (zapret2 or GoodbyeDPI) is on.

- **chunks:** how many pieces (2–20).
- **delay (ms):** the pause between pieces.

**Test sites**

The sites used to check connectivity (default: youtube.com, discord.com, x.com). Click **⟳ test again** to check; each site shows:

| Result | Meaning |
| --- | --- |
| ✓ | Opens fine |
| ✕ DNS | The name could not be resolved |
| ✕ TCP | Could not connect to the site's server |
| ✕ TLS | Blocked during the encrypted handshake, usually DPI → turn on DPI bypass or auto-tune |
| ✕ HTTP | Connected, but the site returned an error |

You can edit the list in the box below, one site per line.

### 4.4. Logs

Open it from **tools › logs** (the first tab).

![Logs](screenshots/logs-en.png)

Records events: connecting, switching servers, DPI bypass on/off, errors.

- **Filters:** all, engine, dpi, system.
- **pause / resume:** stop scrolling so you can read.
- **copy / save file:** copy the log or save it as `ghostline-log.txt` to attach to a bug report.
- **show queries:** watch DNS queries live. Kept in RAM only, at most 500 lines, **never written to disk**.

### 4.5. Settings

![Settings](screenshots/settings-en.png)

| Setting | Meaning |
| --- | --- |
| language | VI or EN (also switchable with the VI/EN button at the top) |
| start with windows / start with the system | Windows: open Ghostline when you sign in, without a UAC prompt. Linux: connect at boot (with *connect on launch*) and open Ghostline in the tray when you log in |
| connect on launch | Connect as soon as the app opens |
| close → minimise to tray | Clicking ✕ hides the window to the tray instead of quitting. Ghostline keeps protecting you in the background. On a Linux desktop without a tray (GNOME without an extension), open Ghostline again from the app menu to bring the window back |
| adapters | Windows: **auto** protects every adapter in use (recommended), **manual** only the adapters you pick. Linux shows the DNS system Ghostline drives instead (NetworkManager, systemd-resolved or `/etc/resolv.conf`) |
| test domain | The domains the server scan asks for, one per line (default `www.google.com`); a server must answer every one. Use 1–2 sites that always work: each extra domain makes every scan slower (at most 5). A new domain is checked when you save it: one without an IPv4 address (for example `steam.com`; use `store.steampowered.com`) is refused. If a domain later fails on most servers, scans ignore it and a warning asks you to fix it. Applies from the next scan |
| bootstrap | Plain DNS servers used only to look up the addresses of DoH servers at startup (default `1.1.1.1:53`, `8.8.8.8:53`). This is the only unencrypted DNS traffic, and it is only used to look up DoH server names |
| max servers | How many servers to use in parallel (default 5). More is steadier but uses slightly more bandwidth |
| update server list | Download a fresh server list daily (signature-checked) |
| notify about new versions | Show a notice when a new version is out. Ghostline **never updates without asking**: installing takes your click (see *Updating* in section 2) |
| receive beta versions | Also offer pre-releases (for example `v0.8.0-beta.1`): new features earlier, less tested. Off by default, and always on while you run a beta, so a beta build hears of the next beta. Turning it off hides a beta notice |
| ⚠ RESTORE DNS NOW | Put the system's DNS back to its saved state. Use it if DNS ever looks wrong |

### 4.6. Proxy

![Proxy](screenshots/proxy-en.png)

<a href="videos/clips/proxy-en.mp4"><img src="videos/clips/proxy-en.webp" width="640" alt="Turning the proxy on and sharing it"></a>

*The preview plays sped up and silent; click it for the narrated video.*

**Step by step: use the proxy on this PC and on a phone**

1. Turn on **enable proxy**. It runs while you are connected.
2. For browsers on this PC, turn on **use for this PC**.
3. For a phone, turn on **share on LAN**. On the phone, open *Wi-Fi › this network › Proxy › Manual* and enter the address shown, or scan the QR code.
4. On Windows, if Ghostline says the network is *Public*, switch it to *Private* in Windows Settings › Network.

Ghostline can run a local proxy on one port (default `8080`) that speaks **HTTP, HTTPS (CONNECT) and SOCKS4/4a/5**. It starts and stops with **Connect**, and it always resolves names through Ghostline's encrypted DNS, so it never leaks plain DNS.

| Setting | Meaning |
| --- | --- |
| enable proxy | Run the proxy while connected |
| use for this PC | Point the system proxy at Ghostline (Windows; GNOME and KDE on Linux, set in your own session). The old setting is saved first and put back on Disconnect, crash or power loss. If another app (a VPN, a company proxy) already set one, Ghostline asks before replacing it, and never fights an app that changes it later |
| share on LAN | Let phones and other devices on the same Wi-Fi use the proxy. Only private addresses are accepted. On Windows the firewall rule `Ghostline Proxy` is limited to *Private* networks; on Linux Ghostline opens the port in firewalld or ufw and closes it again on Disconnect |
| port | 1024–65535 |

**On a phone:** turn on *share on LAN*, then on the phone open Wi-Fi → this network → Proxy → Manual, and enter the address shown (or scan the QR code). On Windows, if the page says the network is *Public*, switch it to *Private* in Windows Settings → Network.

**Web fragmentation** splits the TLS ClientHello so DPI cannot read the site name:

- **auto when blocked** (default): connect normally; if the connection is reset or stalls before the server answers, retry once with fragmentation and remember the site for this network (7 days). The first visit to a blocked site can take up to 3 seconds longer.
- **always** / **off**.
- **method:** TCP (split around the SNI), TLS record (split into several TLS records), or combined (default).
- The **remembered domains** list shows what was learned on this network; remove entries if a site starts working without help.

If the statistics show connections *blocked even fragmented*, that network needs DPI bypass.

**Upstream proxies** (SOCKS5 or HTTP, with optional user/password) let rules send some sites through another proxy such as Tor. Passwords are encrypted (Windows DPAPI; on Linux a key only root can read). Use **test** to check one.

### 4.7. Rules and lists

![Rules and lists](screenshots/rules-en.png)

<a href="videos/clips/rules-en.mp4"><img src="videos/clips/rules-en.webp" width="640" alt="Adding a rule and testing a domain"></a>

*The preview plays sped up and silent; click it for the narrated video.*

**Step by step: block a domain**

1. On **Rules**, type a pattern such as `ads.example.com` in the pattern box.
2. Pick the action **block**, then click **+ add rule**.
3. Type the domain in **test a domain** and click **test**: Ghostline names the rule or list that decides it.
4. To remove the rule, click its **✕**.

Rules decide what happens to a domain, both for DNS and for the proxy. The first matching rule wins; if none matches, lists are checked in order.

| Pattern | Matches |
| --- | --- |
| `example.com` | example.com and every subdomain |
| `=example.com` | only example.com |
| `*.example.com` | only subdomains |
| `~ads` | any name containing "ads" |
| `/^ad[0-9]+\./` | a regular expression (RE2) |
| `10.0.0.0/8` | an IP range (proxy only) |

| Action | Effect |
| --- | --- |
| `block` | DNS answers 0.0.0.0 (or NXDOMAIN, see *DNS block answer*); the proxy refuses |
| `allow` | go direct and skip the lists, to fix a false positive |
| `ip=1.2.3.4` | fake DNS answer (repeat for IPv6) |
| `fragment=auto\|on\|off` | override web fragmentation for this site |
| `upstream=<id>` | send through an upstream proxy |

Edit rules in the **table** or switch to **text** (one rule per line, `#` comments, `#!` for a disabled rule). Nothing is saved until every line is valid; bad lines are marked with their number.

**Lists:** paste any GitHub link (blob, raw, gist or jsDelivr) or a local file path, choose the action, and press **+ add list**. Ghostline detects the format (hosts, plain domains, AdBlock/AdGuard, dnsmasq, Unbound, RPZ, Clash/Surge, v2ray domain-list-community, sing-box JSON, CIDR), shows how many entries it read and which lines it skipped, and updates the list every 24 hours. **Quick add** offers well-known lists with their license and repository. If GitHub is blocked, Ghostline falls back to jsDelivr.

**Test a domain** tells you which rule or list decides a name, for example *block — list HaGeZi Light, line 120*.

### 4.8. DNS server

![DNS server](screenshots/dnsserver-en.png)

<a href="videos/clips/dnsserver-en.mp4"><img src="videos/clips/dnsserver-en.webp" width="640" alt="Sharing the DNS server and opening the phone setup page"></a>

*The preview plays sped up and silent; click it for the narrated video.*

Share Ghostline's encrypted DNS with this PC's browsers and with other devices on your home network. It runs while you are connected.

- **Local DoH:** browsers on this PC can use `https://127.0.0.1/dns-query` as their custom secure DNS.
- **Share on the LAN:** also answers DNS on port 53 and DoH on this PC's LAN addresses. Only devices on your local network can use it; requests from elsewhere are refused. On Windows the network must be marked **Private**; on Linux Ghostline opens the ports in firewalld or ufw.
- **DoH port:** 443 by default; change it if another program uses that port.

**Use on other devices** (no certificate needed for port 53):

| Device | How |
| --- | --- |
| Router, TV, console | set the DNS server to this PC's IP |
| Android | turn off *Private DNS*, then set a static DNS for your home Wi-Fi |
| Steam Deck | set the DNS manually for your home Wi-Fi only (not for every network) |
| iPhone / iPad | *Settings › Wi-Fi › (i) › Configure DNS › Manual* with this PC's IP — or install the DoH profile below |

**Step by step: iPhone DoH profile**

1. Click **Connect**, then on **DNS server** turn on **local DoH** and **share on the LAN** (on Windows the network must be *Private*).
2. Pick or type your **home Wi-Fi name**, exactly as on the phone. Ghostline lists the networks this PC knows; a PC on Ethernet may list none, and the phone can also type it on the page.
3. Click **open the phone setup page** and scan the QR code with the iPhone. The page stays open for 10 minutes.
4. On the phone, compare the fingerprint with the one in Ghostline, then tap **Download the profile** and install it in *Settings › Profile Downloaded*.
5. **Required:** open *Settings › General › About › Certificate Trust Settings* and turn on **Ghostline LAN CA** (Full Trust). Without this step the iPhone has no internet on your home Wi-Fi.
6. Check *Settings › General › VPN & Device Management › DNS*: **Ghostline DNS** is selected.

The encrypted DNS is used only on your home Wi-Fi; on mobile data and other networks the iPhone uses its usual DNS. **save files…** writes the certificate and profile to disk instead.

<p align="center"><img src="screenshots/setuppage-en.png" width="300" alt="The phone setup page"></p>

**When this PC is off or disconnected,** every device that uses it for DNS loses the internet on your home network. iOS does not fall back to another DNS server. To get the iPhone back online, open *Settings › General › VPN & Device Management › DNS* and choose **Automatic** (or remove the profile). If this PC is often off, use the manual DNS setting above instead of the profile: it needs no certificate and is quick to switch back. When LAN devices have used the DNS server in the last 10 minutes, **Disconnect** (in the app and in the tray) asks first; shutting the PC down and **Quit** do not ask.

**LAN CA:** the certificate other devices trust. It can only sign private addresses and `*.ghostline.lan`, so it cannot be used to impersonate websites. **recreate** makes a new one (devices must install it again); **remove** deletes it and turns the DNS server off.

### 4.9. Fake SNI

![Fake SNI](screenshots/fakesni-en.png)

<a href="videos/clips/fakesni-en.mp4"><img src="videos/clips/fakesni-en.webp" width="640" alt="Turning Fake SNI on"></a>

*The preview plays sped up and silent; click it for the narrated video.*

**Step by step: turn Fake SNI on**

1. Click **Connect**. On **Proxy**, turn on **enable proxy** and **use for this PC**.
2. Open **proxy › fake sni**, read the warning and scroll it to the end, tick **I understand**, then click **continue**.
3. Turn on **turn Fake SNI on**, and turn on a **preset group** or write `sni=` rules on the **Rules** page.
4. Open the site in your browser. The counters show how many connections were decrypted or fell back to fragmentation.
5. Click **turn Fake SNI off** in the violet banner when you are done.

An advanced feature for sites behind CDNs that allow *domain fronting*. The proxy decrypts the browser's HTTPS for the domains you choose and connects to the server with a different, allowed name, so the network sees that name instead of the real site.

- The first time, read the warning to the end and confirm.
- It needs the **proxy** with **use for this PC** (it applies only to browsers on this PC).
- Turn on a **preset group** or write rules such as `youtube.com sni=www.google.com connect=www.google.com`. `sni=none` sends no name. `connect=` chooses which host's address to connect to.
- While it runs, a violet banner on every page says how many domains are decrypted. **turn Fake SNI off** stops it at once.
- If a server refuses the fake name, Ghostline silently falls back to fragmentation; the counters on the page show this.
- The certificate it installs (in Windows' certificate store; on Linux in the system trust store, and for Firefox and Chrome where they keep their own) exists only while you are connected, can sign only the domains in your rules, and is removed on disconnect, on a crash (by the watchdog) and on uninstall.
- Do not use it for banking or important accounts; apps that pin certificates will fail for these domains. In Firefox you may need `security.enterprise_roots.enabled` in `about:config`.

Lists from other sources can carry `sni=` rules only after you mark them **trust for Fake SNI**; Ghostline's own presets are signed.


### 4.10. Tools

The tools page has the logs (first tab, [section 4.4](#44-logs)) and four diagnostic tools.

<a href="videos/clips/tools-en.mp4"><img src="videos/clips/tools-en.webp" width="640" alt="Lookup, Scanner, Cloudflare IP and Stamp"></a>

*The preview plays sped up and silent; click it for the narrated video.*

| Lookup | Cloudflare IP |
| --- | --- |
| ![Lookup](screenshots/lookup-en.png) | ![Cloudflare IP](screenshots/cfscan-en.png) |
| **Scanner** | **Stamp** |
| ![Scanner](screenshots/scanner-en.png) | ![Stamp](screenshots/stamp-en.png) |

**Lookup** asks one domain through several sources at once and says whether the answers agree.

1. Type a domain (a link pasted from the browser works too) and pick the record type. For **PTR**, type an IP.
2. Tick the sources. By default you get Ghostline (when connected) and the fastest servers from the last scan.
3. Click **look up**. The card on top reads:
   - **DNS is poisoned:** a source answered a private IP, or said the domain does not exist while others found it. That is how ISPs block with DNS.
   - **Answers differ:** different addresses. CDNs answer by location, so this alone does not mean blocking.
   - **Answers match:** same addresses, or the same CDN.
4. **details** shows each answer the way `dig` prints it, with TTLs and flags.

The **ISP DNS** source is the only place Ghostline ever sends an unencrypted query: your ISP sees the domain you look up. It is never ticked for you; tick it only to compare. When the PC gets DNS from the router, Ghostline offers the router's address.

**Scanner** grades many servers at once: latency over several rounds (median, p90, jitter), packet loss, whether the server validates DNSSEC, whether it filters ads, and whether it answers the test sites listed on the **DPI bypass** page with fake addresses. Scan the server list with filters, or paste addresses. One scan grades up to 500 servers by default (50–2000 in **options**); when a filter matches more, the most useful are scanned first: pinned, good last time, then built in. More servers take longer: 2000 take about 6–8 minutes. From the results you can pin servers, use only one, add pasted ones to your list, or export a CSV that opens in Excel.

**Cloudflare IP** looks for Cloudflare addresses that work and are fast on your network.

1. Click **scan**. Ghostline tries one address in each block of Cloudflare's network, over port 443 only, at most 200 new connections a second, straight out (not through Ghostline's proxy). It stops once 50 addresses work.
2. The 10 fastest also get a download speed test.
3. Select a few addresses, then **copy** them, or **create rule**: type the domains (for example `example.com` and `*.example.com`) and Ghostline adds an `ip=` rule on the **Rules** page.

An `ip=` rule only affects apps that use Ghostline's DNS or proxy, and only works for domains that really are behind Cloudflare. Results are kept per network; **check again** retests the selected addresses. A clean address today may be blocked tomorrow: scan again when a site stops loading.

**Stamp** reads and builds `sdns://` stamps. Paste stamps to see what is inside, or fill the form (or **fill from URL**) to build one, then **add to servers**. Relay and ODoH stamps can be read but Ghostline does not use them.

### 4.11. Backup and moving to another PC

In **Settings › Backup and move to another PC**:

<a href="videos/clips/backup-en.mp4"><img src="videos/clips/backup-en.webp" width="640" alt="The test domain and exporting settings"></a>

*The preview plays sped up and silent; click it for the narrated video.*

- **export settings…** saves a `.ghostline.json` file. Choose what goes in: settings, rules and lists, your servers, the DPI blacklist, the zapret2 learned list. It never contains your home Wi-Fi name, network adapters, proxy passwords, logs or certificates. Lists that point at a file on this PC are left out.
- **import settings…** works only while disconnected. Ghostline shows what will change first, and asks before importing rules that redirect decrypted traffic (`sni=`, `connect=`). An import always turns off Fake SNI, the DNS server, sharing on the LAN and start with Windows (start with the system), and lists from other sources are no longer trusted for Fake SNI: turn them on again on this PC if you need them. Proxy passwords must be typed again.
- If writing fails halfway, every file is put back as it was. The files an import replaced are kept next to them as `*.bak-import`.

`Ghostline.exe --export <file>` writes the same backup from the command line, for sending to someone who helps you. Run it from Command Prompt or PowerShell to see the result; like Ghostline itself, it asks for administrator permission. On Linux: `sudo /usr/lib/ghostline/ghostlined --export <file>` (`/var/lib/ghostline/bin/ghostlined` when the service came from the AppImage or the tar.gz).

## 5. The tray icon

Ghostline puts a ring icon in the system tray (on Windows, bottom right next to the clock; on Linux, in the panel's tray: KDE has one, GNOME needs a tray extension). Its colour shows the current status. **Right-click** it for the menu:

- **Connect / Disconnect** (asks first while devices on your network use this PC's DNS)
- **DPI bypass:** quickly turn DPI bypass on or off
- **Proxy: on/off:** turn the local proxy on or off
- **Open Ghostline:** show the window again
- **Quit:** on Windows, disconnect, restore your DNS, then close the app. On Linux it closes only the window: the background service keeps protecting until you **Disconnect**

## 6. When a site is still blocked

> ⚖️ You are responsible for complying with the law and your network provider's terms. Do not use these features to reach content that is prohibited by law. See the [Disclaimer](../README.md#disclaimer).

Work through these in order and stop as soon as the site opens:

1. **Connect Ghostline.** Many sites are blocked only through DNS, so connecting is enough.
2. **Clear your browser cache**, or try a private window (the browser may still remember old DNS answers).
3. Open **DPI bypass** and turn it on with the **zapret2** engine and the **Light** preset.
4. Click **⚡ auto-tune** to let Ghostline find a preset that works. Add the site you need to **Test sites** first so auto-tune checks that exact site.
5. Still blocked: try a stronger preset (on Windows with GoodbyeDPI, **Mode 1–6**).
6. If only a few sites are blocked, switch **scope** to **blacklist** and add just those sites, so DPI bypass doesn't affect anything else.

> **Browser note:** Chrome, Edge and Firefox have their own *Secure DNS / DNS over HTTPS* option. When it's on, the browser bypasses Ghostline. Turn it off, or set it to use the system's DNS.

## 7. Troubleshooting

| Message | Cause and fix |
| --- | --- |
| **Ghostline needs administrator rights to change DNS** | Windows: the app was opened without admin rights. Close it, then right-click → **Run as administrator** |
| **… seems to be intercepting this computer's DNS** | An ad blocker, antivirus or VPN takes the DNS queries before they reach Ghostline (AdGuard, Avast, AVG, YogaDNS, Portmaster…). The message names the program and says which setting to turn off; turn off its DNS filtering (or quit it), then retry |
| **Port 53 is held by …** | Another program is running DNS on 127.0.0.1 (usually WSL, Hyper-V or another DNS tool; Mobile Hotspot no longer gets in the way). Close it, or use the **Stop service …** button Ghostline offers. Ghostline always asks before stopping any service |
| **No working servers found** | Your network is down, or your ISP blocks encrypted DNS too. Check your connection, then try turning on **DNS fragment** |
| **DNS queries are not going through Ghostline** | A VPN or another tool owns DNS. If the error lists **adapters with their own DNS**, those are Windows adapters still asking DNS another way (a VPN, PPPoE, a USB 4G stick…): turn them off, or pick only the right ones in **Settings → adapters → manual**, then connect again |
| **Could not set DNS on …** | That adapter doesn't allow DNS changes (often a virtual adapter from a VPN or VM). Go to **Settings → adapters → manual** and leave it out |
| **Could not restore the original DNS on …** | Click **⚠ RESTORE DNS NOW**. The message stays until the restore succeeds |
| **GoodbyeDPI failed to start** | Try another preset. The details in brackets say more |
| **Another program is using the WinDivert driver** | Windows: a GoodbyeDPI or zapret running outside Ghostline holds the driver. Close it (`goodbyedpi.exe`, `winws.exe`, `winws2.exe`), then try again |
| **GoodbyeDPI was blocked by antivirus** | Add the Ghostline folder to your antivirus exclusions |
| **GoodbyeDPI files were modified** | The GoodbyeDPI files no longer match their original hash (an antivirus may have changed them, or they were tampered with). Reinstall Ghostline |
| **No working DPI bypass configuration found** | Auto-tune found no preset that opens every test site. Try Mode 1–6 or custom arguments |
| **Connect first to auto-tune DPI bypass** | Click Connect, then run auto-tune again |
| **Ghostline's background service is not running** | Linux: press **start the service** (or **install the service**) in the window, or run `sudo systemctl enable --now ghostline` |
| **Your account may not control Ghostline** | Linux: run `sudo usermod -aG ghostline $USER`, then log out and in again |
| **This Linux kernel cannot pass packets to the DPI engine** | Linux: the kernel lacks `nfnetlink_queue` / `nft_queue`. Since 0.6.1 the service loads them itself; on an older install run `sudo modprobe -a nfnetlink_queue nft_queue` or update. Without them, the proxy's fragmentation still helps |
| **Chrome and Chromium do not trust the Fake SNI certificate yet** | Linux (Ubuntu, Debian): install `libnss3-tools`, then turn Fake SNI off and on |
| **Ghostline cannot change this desktop's proxy settings** | Linux: only GNOME and KDE are set automatically. Set an HTTP proxy to the address on the Proxy page by hand |

**Lost internet after using Ghostline?** This is very unlikely because there are several recovery layers. On Windows, even if an antivirus kills Ghostline and deletes `ghostline.exe`, the `Ghostline Network Guard` task puts your DNS back within about a minute, and again at the next boot. On Linux, systemd restores DNS whenever the service stops or crashes, and the service restores at boot. If it still happens on Windows:

1. Open Ghostline → **Settings → ⚠ RESTORE DNS NOW**.
2. Or open PowerShell as administrator and run:
   ```powershell
   & "C:\Program Files\Ghostline\Ghostline\ghostline.exe" --restore
   ```
   (for the portable build, use the path to your own `ghostline.exe`).
3. Last resort: **Settings → Network & internet → your adapter → DNS server assignment → Edit → Automatic (DHCP)**.

On Linux: open Ghostline → **Settings → ⚠ RESTORE DNS NOW**, or run `sudo systemctl stop ghostline` (stopping the service restores DNS), then `sudo systemctl start ghostline`.

**Reporting a bug:** go to **Logs → save file**, then open an issue on [GitHub](https://github.com/hashcott/ghostline/issues) with that file attached. Logs never contain the sites you visited.

**Removing Ghostline certificates by hand:** **Settings → certificates → remove all Ghostline certificates** does it from the app. Without the app on Windows, run `certlm.msc`, open *Trusted Root Certification Authorities → Certificates* and delete entries starting with `Ghostline`. On Linux, `sudo /usr/lib/ghostline/ghostlined --remove-certs` removes them from the system trust store and the browsers (uninstalling does this too).

## 8. Uninstalling

- **Installer build:** Settings → Apps → Ghostline → Uninstall. The uninstaller restores your DNS and removes the startup tasks, the WinDivert driver and every Ghostline certificate.
- **Portable build:** in the app click **Disconnect**, turn off **start with windows**, quit from the tray, then delete the folder.
- **Linux:** `sudo apt remove ghostline` (`apt purge` also deletes the settings), `sudo dnf remove ghostline`, `sudo pacman -R ghostline-bin`; tar.gz: `sudo ./uninstall.sh [--purge]` from the unpacked folder; AppImage: `sudo /var/lib/ghostline/bin/ghostlined --uninstall-system [--purge]`, then delete the AppImage file. DNS, the system proxy, certificates (also in Firefox profiles), firewall rules and the nftables table come back first. rpm and Arch keep `/var/lib/ghostline` and `/var/log/ghostline`; delete them to remove everything.

## 9. FAQ

**Is Ghostline a VPN?**
No. Ghostline encrypts only **DNS** (the "where is this site?" question). It doesn't change your IP address or encrypt the content you browse. If you need to hide your IP, use a VPN; note that a VPN and Ghostline usually can't run at the same time.

**Does Ghostline slow down my internet?**
Usually not. Ghostline queries several servers at once, uses the fastest answer, and caches results. GoodbyeDPI on a high preset may make some sites slightly slower.

**Does Ghostline collect my data?**
No. No telemetry, no accounts, and visited sites are never written to disk. The code is open source, so you can check for yourself.

**What if I shut down while connected?**
That's fine. On Windows, Ghostline restores DNS before Windows shuts down; if the power is cut, the *Ghostline Recovery* task restores DNS at your next sign-in, even if you don't open Ghostline. On Linux, the service restores DNS when it stops at shutdown, and again at boot if the power was cut (then reconnects if *start with the system* is on).

**Can I use it with Mobile Hotspot?** (Windows)
Yes. Mobile Hotspot listens on port 53 of every address, but Windows still lets Ghostline take 127.0.0.1:53, so you can connect with the hotspot on.
