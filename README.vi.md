<div align="center">

<img src="build/appicon.png" width="96" alt="Logo Ghostline">

# Ghostline

**Mã hoá DNS và vượt DPI cho Windows và Linux, chỉ với một nút bấm.**

[![CI](https://github.com/hashcott/ghostline/actions/workflows/ci.yml/badge.svg)](https://github.com/hashcott/ghostline/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/hashcott/ghostline?include_prereleases)](https://github.com/hashcott/ghostline/releases)
[![Downloads](https://img.shields.io/github/downloads/hashcott/ghostline/total)](https://github.com/hashcott/ghostline/releases)
[![License: GPL v3](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64%20%7C%20Linux%20x86__64-blue)

[English](README.md) · Tiếng Việt

<a href="https://github.com/hashcott/ghostline/releases/latest/download/ghostline-amd64-installer.exe"><img src="https://img.shields.io/badge/%E2%AC%87%20T%E1%BA%A3i%20v%E1%BB%81-Windows%2010%2F11%20x64-2ea44f?style=for-the-badge" alt="Tải Ghostline cho Windows"></a>
<a href="#linux-x86_64"><img src="https://img.shields.io/badge/%E2%AC%87%20T%E1%BA%A3i%20v%E1%BB%81-Linux%20x86__64-2ea44f?style=for-the-badge" alt="Tải Ghostline cho Linux"></a>

</div>

---

Ghostline chạy một DNS server cục bộ trên `127.0.0.1` / `::1`, trỏ DNS của hệ thống về đó (mọi card mạng trên Windows; NetworkManager, systemd-resolved hoặc `/etc/resolv.conf` trên Linux), rồi chuyển tiếp truy vấn qua **DoH, DoT, DoQ hoặc DNSCrypt** tới máy chủ nhanh nhất còn hoạt động. Khi kết nối mã hoá bị can thiệp bằng cách soi gói tin (DPI), Ghostline có thể chạy thêm một engine vượt DPI: **zapret2** (khuyên dùng), hoặc **GoodbyeDPI** trên Windows. Trên hết, Ghostline được thiết kế để **luôn trả lại DNS gốc của bạn**, kể cả khi app bị tắt đột ngột hay máy mất điện.

<p align="center">
  <img src="docs/screenshots/simple-vi.png" height="360" alt="Giao diện Đơn giản">
  &nbsp;
  <img src="docs/screenshots/overview-vi.png" height="360" alt="Giao diện Đầy đủ">
</p>

## Mục lục

- [Tính năng](#tính-năng)
- [Video hướng dẫn](#video-hướng-dẫn)
- [Ảnh chụp màn hình](#ảnh-chụp-màn-hình)
- [Cài đặt](#cài-đặt)
- [Sử dụng](#sử-dụng)
- [Cách hoạt động](#cách-hoạt-động)
- [Quyền riêng tư](#quyền-riêng-tư)
- [Hạn chế đã biết](#hạn-chế-đã-biết)
- [Build từ mã nguồn](#build-từ-mã-nguồn)
- [Đóng góp](#đóng-góp)
- [Bảo mật](#bảo-mật)
- [Tuyên bố miễn trừ trách nhiệm](#tuyên-bố-miễn-trừ-trách-nhiệm)
- [Giấy phép và ghi công](#giấy-phép-và-ghi-công)
- [Ủng hộ](#ủng-hộ)

## Tính năng

- **Mã hoá DNS cho toàn hệ thống:** hỗ trợ DoH, DoT, DoQ, DNSCrypt, chạy trên [AdGuard dnsproxy](https://github.com/AdguardTeam/dnsproxy).
- **Tự chọn máy chủ:** quét song song, loại máy chủ trả kết quả bị đầu độc, và nhớ máy chủ tốt nhất cho từng mạng.
- **Không bao giờ mất mạng:** chụp lại DNS gốc trước khi đổi và luôn trả về. Trên Windows có 5 lớp khôi phục: ngắt kết nối sạch, tiến trình watchdog, khôi phục khi mở lại app, tác vụ chạy lúc đăng nhập, và lớp bảo vệ mạng vẫn chạy được khi antivirus cách ly `ghostline.exe`. Trên Linux, dịch vụ systemd khôi phục khi ngắt kết nối, khi dừng hoặc bị crash, và lúc khởi động máy.
- **Xác minh không rò rỉ:** sau khi kết nối, Ghostline kiểm tra truy vấn thật sự đi qua nó.
- **Vượt DPI:** đi kèm [zapret2](https://github.com/bol-van/zapret2) v1.0.5.2 (gói giả, nhiều kiểu cắt, hỗ trợ QUIC cho YouTube/Google), chạy qua WinDivert trên Windows và hàng đợi nftables trên Linux, cùng GoodbyeDPI 0.2.3rc3 trên Windows, đều được khoá mã băm. Chiến lược zapret2 lấy từ danh sách có chữ ký, cập nhật hằng ngày; có tự dò, danh sách đen, tự phát hiện trang bị chặn, và chia nhỏ (fragment) truy vấn DoH. Trên Windows, nếu antivirus chặn zapret2, Ghostline tạm chạy GoodbyeDPI và cho phép thử lại.
- **Proxy cục bộ (HTTP / HTTPS / SOCKS4/5):** chạy cùng nút Connect, có thể đặt làm proxy hệ thống (Windows, GNOME, KDE), và chia sẻ cho điện thoại hay thiết bị khác cùng Wi-Fi (có mã QR). Tên miền luôn được phân giải qua DNS mã hoá của Ghostline.
- **Fragment web không cần driver:** lưu lượng qua proxy được tự động cắt nhỏ ClientHello khi trang bị chặn theo SNI, và Ghostline ghi nhớ cách vượt cho từng mạng.
- **Rules và danh sách cộng đồng:** chặn, cho phép, DNS giả, fragment hoặc đi qua upstream proxy theo domain, keyword, regexp hay CIDR. Import danh sách hosts, AdBlock/AdGuard, dnsmasq, Unbound, RPZ, Clash, v2ray, sing-box hoặc CIDR thẳng từ link GitHub, tự cập nhật theo lịch.
- **DNS server cho mạng nhà:** DNS mã hoá cho điện thoại, TV, máy chơi game và router trong Wi-Fi: DNS cổng 53 (không cần chứng chỉ) hoặc DNS-over-HTTPS, có trang cài đặt qua mã QR và profile cho iOS.
- **Fake SNI (nâng cao, mặc định tắt):** với trang nằm sau CDN cho phép domain fronting, proxy gửi ra mạng một tên miền khác được phép. Chỉ giải mã HTTPS của những tên miền bạn chọn, bằng chứng chỉ chỉ ký được cho đúng các tên miền đó và bị gỡ khi ngắt kết nối.
- **Công cụ chẩn đoán:** tra DNS so sánh nhiều nguồn và phát hiện DNS bị đầu độc, Scanner nâng cao chấm server theo độ trễ, mất gói, DNSSEC, lọc quảng cáo và đầu độc, công cụ tìm IP Cloudflare sạch rồi tạo rule `ip=`, và công cụ đọc/tạo stamp DNS.
- **Sao lưu và khôi phục:** xuất cài đặt, rule, danh sách và server tự thêm ra một file rồi nhập trên máy khác. Thông tin riêng tư và gắn với máy không bao giờ được xuất, và khi nhập không bao giờ tự bật Fake SNI hay chia sẻ trong LAN.
- **Danh sách máy chủ có chữ ký:** cập nhật mỗi ngày, xác minh bằng ed25519; danh sách DNSCrypt được kiểm tra bằng minisign.
- **Giao diện Đơn giản và Đầy đủ**, icon khay, giao diện tiếng Việt và tiếng Anh, phong cách neon-terminal.
- **Bản cài đặt, portable hoặc gói Linux:** trên Windows có bản cài đặt và bản portable (lưu mọi dữ liệu trong thư mục `data\` cạnh file exe); trên Linux có `.deb`, `.rpm`, AppImage, `tar.gz` và PKGBUILD.
- **Chỉ thông báo khi có bản mới:** không bao giờ tự cập nhật ngầm.

## Video hướng dẫn

Video hơn 3 phút có lồng tiếng: kết nối bằng một nút bấm, mức bảo vệ, máy chủ, vượt DPI, proxy, rules, DNS server kèm cài đặt cho iPhone, Fake SNI, công cụ chẩn đoán và sao lưu. Ảnh động bên dưới được tua nhanh, không có tiếng; bấm vào để xem bản đầy đủ có tiếng. Có cả [bản tiếng Anh](docs/videos/guide-en.mp4).

<p align="center">
  <a href="docs/videos/guide-vi.mp4"><img src="docs/videos/guide-vi.webp" width="720" alt="Video hướng dẫn Ghostline (tua nhanh); bấm để xem bản đầy đủ"></a>
</p>

**Clip ngắn có lồng tiếng cho từng tính năng:** [Kết nối](docs/videos/clips/connect-vi.mp4) · [Mức bảo vệ](docs/videos/clips/levels-vi.mp4) · [Máy chủ](docs/videos/clips/servers-vi.mp4) · [Vượt DPI](docs/videos/clips/dpi-vi.mp4) · [Proxy](docs/videos/clips/proxy-vi.mp4) · [Rules](docs/videos/clips/rules-vi.mp4) · [DNS server và iPhone](docs/videos/clips/dnsserver-vi.mp4) · [Fake SNI](docs/videos/clips/fakesni-vi.mp4) · [Công cụ](docs/videos/clips/tools-vi.mp4) · [Tên miền thử và sao lưu](docs/videos/clips/backup-vi.mp4) · [Ngắt kết nối](docs/videos/clips/disconnect-vi.mp4). [Hướng dẫn sử dụng](docs/huong-dan-su-dung.md) đặt mỗi clip cạnh các bước làm tương ứng.

## Ảnh chụp màn hình

| Máy chủ | Vượt DPI |
| --- | --- |
| ![Máy chủ](docs/screenshots/servers-vi.png) | ![Vượt DPI](docs/screenshots/dpi-vi.png) |
| **Proxy** | **Rules và danh sách** |
| ![Proxy](docs/screenshots/proxy-vi.png) | ![Rules](docs/screenshots/rules-vi.png) |
| **Nhật ký** | **Cài đặt** |
| ![Nhật ký](docs/screenshots/logs-vi.png) | ![Cài đặt](docs/screenshots/settings-vi.png) |
| **DNS server** | **Fake SNI** |
| ![DNS server](docs/screenshots/dnsserver-vi.png) | ![Fake SNI](docs/screenshots/fakesni-vi.png) |
| **Lookup** | **IP Cloudflare** |
| ![Lookup](docs/screenshots/lookup-vi.png) | ![IP Cloudflare](docs/screenshots/cfscan-vi.png) |

## Cài đặt

**[⬇ Tải bản mới nhất](https://github.com/hashcott/ghostline/releases/latest)** — hoặc chọn một file bên dưới. Các phiên bản cũ nằm ở trang [Releases](https://github.com/hashcott/ghostline/releases).

| File | Là gì | Tải về |
| --- | --- | --- |
| `ghostline-amd64-installer.exe` | Bản cài đặt (tự cài WebView2 nếu thiếu) | **[Tải trực tiếp ⬇](https://github.com/hashcott/ghostline/releases/latest/download/ghostline-amd64-installer.exe)** |
| `Ghostline-<phiên bản>-portable.zip` | Bản portable: giải nén rồi chạy | [Từ bản mới nhất](https://github.com/hashcott/ghostline/releases/latest) |
| `SHA256SUMS` | Mã SHA-256 của hai file trên | [Tải trực tiếp](https://github.com/hashcott/ghostline/releases/latest/download/SHA256SUMS) |

### Windows

Kiểm tra file đã tải:

```powershell
Get-FileHash .\Ghostline-0.1.0-portable.zip -Algorithm SHA256
```

**Yêu cầu:** Windows 10/11 x64 và quyền quản trị (admin), vì đổi DNS của card mạng và nạp driver WinDivert đều cần quyền này. Khi khởi động cùng Windows, Ghostline chạy qua Task Scheduler nên không hiện hộp thoại UAC.

> [!NOTE]
> Bản phát hành chưa được ký số, nên SmartScreen sẽ hiện "Windows protected your PC". Sau khi đã kiểm tra SHA-256, chọn **More info → Run anyway**.
> Một số phần mềm diệt virus báo nhầm driver WinDivert mà zapret2 và GoodbyeDPI dùng. Ghostline kiểm tra mã băm của engine trước mỗi lần chạy; nếu zapret2 bị chặn, Ghostline tạm chạy GoodbyeDPI và trang DPI hiện thư mục `bin\zapret2` để bạn thêm vào danh sách loại trừ.
>
> Nếu bạn nâng quyền UAC bằng **một tài khoản admin khác**, dữ liệu của Ghostline sẽ nằm trong `%APPDATA%` của tài khoản admin đó.

### Linux (x86_64)

| Bản phân phối | File | Cài đặt |
| --- | --- | --- |
| Ubuntu 24.04+, Debian 13+ | `ghostline_<phiên bản>_amd64.deb` | `sudo apt install ./ghostline_<phiên bản>_amd64.deb` |
| Fedora 41+ | `ghostline-<phiên bản>-1.x86_64.rpm` | `sudo dnf install ./ghostline-<phiên bản>-1.x86_64.rpm` |
| Arch, CachyOS, Manjaro | `ghostline-<phiên bản>-linux-amd64.tar.gz` | Giải nén rồi chạy `sudo ./install.sh`, hoặc build `ghostline-bin` từ `PKGBUILD` đính kèm bản phát hành (`makepkg -si`) |
| Bản phân phối khác | `Ghostline-<phiên bản>-x86_64.AppImage` | `chmod +x` rồi chạy; app sẽ đề nghị cài dịch vụ nền |

Kiểm tra file tải về: `sha256sum -c SHA256SUMS --ignore-missing`.

Trên Linux, Ghostline gồm hai phần: dịch vụ nền (`ghostline.service`, chạy bằng root) đổi DNS, chạy zapret2 và vẫn bảo vệ khi đã đóng cửa sổ hay sau khi khởi động lại máy; và cửa sổ app, chạy bằng tài khoản của bạn. Thành viên các nhóm `wheel`, `sudo`, `admin` hoặc `ghostline` điều khiển được dịch vụ; muốn cho tài khoản khác dùng: `sudo usermod -aG ghostline <tài khoản>` rồi đăng nhập lại. Gói deb và rpm khởi động dịch vụ ngay; trên Arch hãy bật một lần bằng `sudo systemctl enable --now ghostline` (hoặc nút trong cửa sổ).

**Yêu cầu:** systemd, GTK 4 và WebKitGTK 6.0 (gói cài tự kéo về; AppImage và tar.gz dùng thư viện có sẵn của máy).

**Cập nhật:** cài gói `.deb` hoặc `.rpm` mới, hoặc build lại `ghostline-bin` từ `PKGBUILD` mới. Nếu dịch vụ được cài từ AppImage hoặc tar.gz, giải nén tar.gz mới rồi chạy `sudo ./install.sh` một lần (lệnh này thay dịch vụ và khởi động lại nó); sau đó dùng AppImage mới.

**Gỡ cài đặt:** `sudo apt remove ghostline` (`apt purge` xoá luôn cài đặt), `sudo dnf remove ghostline`, `sudo pacman -R ghostline-bin`; tar.gz: `sudo ./uninstall.sh [--purge]` trong thư mục đã giải nén; AppImage: `sudo /var/lib/ghostline/bin/ghostlined --uninstall-system [--purge]`, rồi xoá file AppImage. DNS, proxy hệ thống, chứng chỉ, luật firewall và bảng nftables được trả lại trước. rpm và Arch giữ lại cài đặt: `sudo rm -rf /var/lib/ghostline /var/log/ghostline` để xoá hẳn.

## Sử dụng

> 📖 Hướng dẫn chi tiết từng màn hình, cách xử lý khi không vào được trang và xử lý sự cố: **[docs/huong-dan-su-dung.md](docs/huong-dan-su-dung.md)**

1. Mở Ghostline và bấm **Kết nối**. App tự chọn máy chủ, chuyển hướng DNS và kiểm tra rò rỉ.
2. Nếu vẫn còn trang bị chặn, hoặc bật **proxy** (Đầy đủ → Proxy → bật proxy + dùng cho máy này) để trình duyệt được fragment tự động, hoặc vào **Đầy đủ → Vượt DPI**, chọn engine (khuyên dùng **zapret2**), bật lên rồi bấm **tự dò**.
   Muốn chia sẻ cho thiết bị khác, bật **chia sẻ LAN** rồi quét mã QR bằng điện thoại (trên Windows, mạng phải là *Private*).
3. Bấm **Ngắt kết nối** (hoặc thoát từ icon khay) để trả lại DNS gốc.

Nếu DNS có vẻ không đúng, vào **Cài đặt → Khôi phục DNS ngay** để đưa DNS của hệ thống về trạng thái đã lưu. Bạn cũng có thể chạy lệnh:

```powershell
ghostline.exe --restore
```

Trên Linux, dừng dịch vụ là DNS được trả về: `sudo systemctl stop ghostline` (rồi `sudo systemctl start ghostline`).

## Cách hoạt động

```
ứng dụng ──► DNS hệ thống ──► 127.0.0.1:53 (Ghostline / dnsproxy) ──► DoH · DoT · DoQ · DNSCrypt
            (DNS client của Windows,      │
             NetworkManager, resolved     │
             hoặc resolv.conf)            │
            zapret2 / GoodbyeDPI (tuỳ chọn) biến đổi gói TLS/HTTP/QUIC đi ra để né lọc SNI
```

Trên Linux, Ghostline gồm một dịch vụ chạy bằng root (`ghostlined`, do systemd chạy) nắm DNS, DPI và proxy, và cửa sổ app chạy bằng tài khoản của bạn, nói chuyện với dịch vụ qua socket cục bộ. Đóng cửa sổ không làm dừng bảo vệ.

**Lưới an toàn.** Trước khi đổi DNS, Ghostline ghi lại DNS hiện tại vào `state.json`. Trên Windows, năm lớp sau bảo đảm bản lưu này luôn được khôi phục:

1. **Ngắt kết nối sạch:** trường hợp thông thường.
2. **Watchdog:** một tiến trình `--watchdog` riêng khôi phục DNS trong vài giây nếu app chết.
3. **Lần mở sau:** nếu còn bản lưu sót lại, app khôi phục ngay khi khởi động.
4. **Tác vụ đăng nhập:** tác vụ `Ghostline Recovery` chạy `--restore` sau khi máy treo hoặc mất điện.
5. **Bảo vệ mạng:** trong lúc kết nối, tác vụ `Ghostline Network Guard` chạy một script PowerShell bằng quyền SYSTEM mỗi phút, lúc khởi động máy và ngay sau khi Microsoft Defender xử lý một mối đe doạ. Nếu Ghostline đã tắt mà DNS vẫn trỏ về 127.0.0.1, script trả DNS và proxy hệ thống về như cũ theo `state.json`. Lớp này không cần `ghostline.exe`, nên vẫn hoạt động khi antivirus kill và cách ly app cùng các lớp ở trên.

Trên Linux, systemd đóng vai watchdog: dịch vụ khôi phục khi ngắt kết nối, `ExecStopPost=--restore` chạy mỗi khi dịch vụ dừng hoặc bị crash (kể cả `kill -9`), và lúc khởi động máy dịch vụ khôi phục những gì lần mất điện để lại trước khi làm việc khác.

Chi tiết thiết kế nằm trong [`docs/superpowers/specs`](docs/superpowers/specs), và [`docs/platforms.md`](docs/platforms.md) chỉ ra mỗi tính năng nằm ở code nào trên Windows và Linux.

## Quyền riêng tư

- Không telemetry, không tài khoản, không thống kê.
- Không bao giờ ghi tên miền bạn truy cập xuống đĩa. Nhật ký truy vấn (nếu bật) chỉ nằm trong RAM.
- App chỉ kết nối mạng tới: máy chủ DNS bạn chọn, DNS bootstrap để phân giải tên các máy chủ đó, danh sách máy chủ có chữ ký, danh sách DNSCrypt, và GitHub để kiểm tra bản mới.
- Proxy không ghi đích đến xuống đĩa; danh sách kết nối trực tiếp chỉ nằm trong RAM, giống nhật ký truy vấn.

## Hạn chế đã biết

- **Fake SNI** chỉ áp dụng cho trình duyệt trên máy này đi qua proxy, chỉ cho tên miền có rule `sni=`, và làm hỏng app ghim chứng chỉ. Firefox có thể cần bật `security.enterprise_roots.enabled`.
- **Thiết bị dùng DNS của máy tính sẽ mất mạng khi máy tắt hoặc ngắt kết nối.** iOS không có DNS dự phòng: với profile DoH, iPhone mất mạng ở Wi-Fi nhà cho tới khi bạn chọn *Tự động* trong *Cài đặt › Cài đặt chung › VPN và quản lý thiết bị › DNS* (4G/5G không bị ảnh hưởng). Nút Ngắt kết nối sẽ hỏi lại khi có thiết bị trong mạng đang dùng DNS server; nếu máy tính hay tắt, hãy đặt DNS thủ công cho iPhone thay vì dùng profile. Private DNS của Android không dùng được với Ghostline; hãy đặt DNS tĩnh cho Wi-Fi.

### Windows

- Nếu bạn duyệt UAC bằng **tài khoản admin khác**, `%APPDATA%` và System Proxy là của tài khoản đó, nên "dùng cho máy này" không áp dụng cho người dùng đang đăng nhập.
- Fragment web chỉ giúp được ứng dụng đi qua proxy. Ứng dụng bỏ qua proxy của Windows (một số game, Firefox có cài đặt proxy riêng) cần dùng engine vượt DPI.
- Chia sẻ LAN chỉ hoạt động trên mạng được Windows đánh dấu **Private**; Ghostline không bao giờ tự đổi profile mạng.

### Linux

- GNOME không có khay hệ thống mặc định: khi bật *thu xuống khay*, cửa sổ đã đóng không có icon để bấm; mở lại Ghostline từ menu ứng dụng để hiện cửa sổ, hoặc tắt tuỳ chọn này để bấm ✕ là thoát. Dù cách nào, việc bảo vệ vẫn chạy trong dịch vụ nền.
- Proxy hệ thống chỉ được đặt tự động trên GNOME và KDE; desktop khác hãy tự đặt `127.0.0.1:<cổng>`.
- Fake SNI trong Chrome và Chromium trên Ubuntu, Debian cần gói `libnss3-tools` (`certutil`).
- Kernel không có NFQUEUE (`nfnetlink_queue`, `nft_queue`) thì không vượt DPI được; fragment của proxy vẫn hoạt động.
- Không có GoodbyeDPI (chỉ có trên Windows); engine là zapret2.
- Gói cài chưa ký GPG: hãy kiểm tra `SHA256SUMS`.
- AppImage cần glibc 2.39 trở lên (Ubuntu 24.04, Debian 13, Fedora 40 trở về sau).
- SteamOS (Steam Deck) chưa được kiểm chứng.

## Build từ mã nguồn

**Cần có:** Go 1.27+, Node.js 24, [Wails v3](https://v3.wails.io) `v3.0.0-beta.27`, và [NSIS](https://nsis.sourceforge.io) để tạo bản cài đặt Windows.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27

wails3 dev                     # chạy thử với tự tải lại (kết nối thật cần admin)
wails3 build                   # bin/ghostline.exe
wails3 package                 # bản cài đặt NSIS
wails3 task windows:portable   # file zip portable
```

Trên Linux (gói phát triển GTK 4 và WebKitGTK 6.0, thêm `squashfs-tools` cho AppImage):

```bash
wails3 task linux:build                     # bin/linux/ghostline và bin/linux/ghostlined
wails3 task linux:package VERSION=0.6.1     # deb, rpm, AppImage và tar.gz trong bin/
```

**Test:**

```bash
go test ./...
cd frontend && npm test
```

Test tích hợp thay đổi cài đặt thật của hệ thống, nên trên Windows cần chạy trong terminal **admin**:

```bash
go test -tags integration ./internal/sysdns/... ./internal/startup/...
```

và trên Linux cần chạy bằng root (DNS hệ thống, hàng đợi nftables với nfqws2, khôi phục sau `kill -9`):

```bash
sudo -E env "PATH=$PATH" go test -tags integration_root ./internal/sysdns/ ./internal/dpi/ ./cmd/ghostlined/
```

Trước khi phát hành, đi qua [`docs/release-checklist.md`](docs/release-checklist.md). Đẩy một tag `v*` lên GitHub sẽ tự build và tạo trang Release qua GitHub Actions.

### Danh sách máy chủ có chữ ký

`lists/servers.json` được workflow **servers** sinh lại và ký (ed25519) mỗi tuần, hoặc khi bấm chạy tay, bằng secret `SERVERLIST_SIGNING_KEY`, rồi commit lên `main`. App tải file này mỗi ngày. Workflow **release** không sinh lại danh sách: bản phát hành nhúng đúng file đang có trong repo lúc gắn tag.

## Đóng góp

Báo lỗi, dịch thuật và pull request đều được hoan nghênh. Vui lòng đọc [CONTRIBUTING.md](CONTRIBUTING.md) trước.

## Bảo mật

Vui lòng **không** báo lỗ hổng bảo mật qua issue công khai. Xem [SECURITY.md](SECURITY.md).

## Tuyên bố miễn trừ trách nhiệm

Ghostline được phát triển với mục đích nghiên cứu và học tập về DNS mã hoá, cơ chế lọc mạng và DPI. Mục tiêu chính là bảo vệ quyền riêng tư (truy vấn DNS không bị đọc hay ghi lại), chống giả mạo và chiếm quyền DNS, và chẩn đoán mạng. Người dùng tự chịu hoàn toàn trách nhiệm về cách sử dụng phần mềm, cũng như việc tuân thủ pháp luật nơi mình sinh sống và điều khoản của nhà cung cấp mạng. Không sử dụng Ghostline vào bất kỳ mục đích vi phạm pháp luật nào, bao gồm:

- truy cập trang web, dịch vụ hay nội dung mà cơ quan có thẩm quyền đã yêu cầu chặn theo quy định của pháp luật;
- cờ bạc trực tuyến, vi phạm bản quyền, lừa đảo, hoặc phát tán nội dung bị pháp luật cấm;
- tấn công, gây gián đoạn hoặc truy cập trái phép vào bất kỳ mạng hay hệ thống nào.

Ghostline không đóng gói, không khuyến nghị và không duy trì danh sách các trang bị cơ quan chức năng chặn. Danh sách và rule do người dùng tự thêm thuộc trách nhiệm của người dùng.

Phần mềm được cung cấp "nguyên trạng", không kèm bất kỳ bảo đảm nào. Tác giả không chịu trách nhiệm về bất kỳ thiệt hại, mất mát dữ liệu, gián đoạn dịch vụ hay hệ quả pháp lý nào phát sinh từ việc sử dụng phần mềm. Xem chi tiết tại [LICENSE](LICENSE).

## Giấy phép và ghi công

Ghostline là phần mềm tự do, phát hành theo [giấy phép GNU GPL v3.0 (chỉ phiên bản 3)](LICENSE). Bạn được dùng, nghiên cứu, chia sẻ và sửa đổi; nếu phân phối bản đã sửa, bạn phải công khai mã nguồn của bản đó theo cùng giấy phép. Hai bản v0.1.0 và v0.1.1 đã phát hành theo giấy phép MIT.

Dự án được xây dựng trên [dnsproxy](https://github.com/AdguardTeam/dnsproxy), [zapret2](https://github.com/bol-van/zapret2), [GoodbyeDPI](https://github.com/ValdikSS/GoodbyeDPI), [WinDivert](https://github.com/basil00/WinDivert) và [Wails](https://wails.io), và lấy cảm hứng từ [DNSveil / SecureDNSClient](https://github.com/msasanmh/SecureDNSClient). Giấy phép của các thành phần bên thứ ba được liệt kê trong [NOTICE](NOTICE).

## Ủng hộ

Ghostline miễn phí và sẽ luôn miễn phí. Nếu có thể, bạn hãy ưu tiên ủng hộ các dự án mà Ghostline dựa vào, vì phần việc khó nhất là của họ:

- [zapret2](https://github.com/bol-van/zapret2) của bol-van: engine vượt DPI
- [GoodbyeDPI](https://github.com/ValdikSS/GoodbyeDPI) của ValdikSS
- [WinDivert](https://github.com/basil00/WinDivert) của basil00
- [dnsproxy](https://github.com/AdguardTeam/dnsproxy) của AdGuard
- [Wails](https://wails.io)

Nếu bạn muốn ủng hộ cả Ghostline, bạn có thể gửi qua [PayPal](https://paypal.me/hashcott), stablecoin hoặc MoMo / VietQR. Cảm ơn bạn!

<p align="center">
  <a href="https://paypal.me/hashcott"><img src="https://img.shields.io/badge/PayPal-hashcott-00457C?logo=paypal&logoColor=white" alt="Ủng hộ qua PayPal"></a>
</p>

**Stablecoin (USDT hoặc USDC):**

```
0x3C0E297cC77416DA2Ac108F09360d7Bf7C4E2c8e
```

> [!WARNING]
> Chỉ gửi qua mạng **BNB Smart Chain (BEP20)** hoặc **Arc**. Gửi qua mạng khác, ví dụ Ethereum (ERC20) hay Tron (TRC20), sẽ mất tiền vĩnh viễn.

<details>
<summary><b>MoMo / VietQR</b> (bấm để xem mã QR)</summary>
<br>
Quét bằng MoMo hoặc bất kỳ app ngân hàng nào (VietQR / Napas 247).
<p align="center"><img src="docs/donate-momo.png" width="240" alt="Mã QR ủng hộ qua MoMo / VietQR"></p>
</details>
