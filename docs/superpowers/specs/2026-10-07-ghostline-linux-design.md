# Ghostline — Hỗ trợ Linux: Thiết kế

- **Ngày:** 2026-10-07
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Đưa Ghostline lên Linux (Ubuntu 24.04+, Debian 13+, Fedora 41+, Arch và các bản dựa trên Arch, SteamOS), ngang bằng tính năng với Windows trong cùng một bản phát hành, **v0.6.0**. Đây là spec tổng: chốt kiến trúc và chia công việc thành 5 sub-spec (L1–L5, mục 21). Mỗi sub-spec có plan riêng.
- **Dựa trên:** [Giai đoạn 1](2026-10-04-ghostline-phase1-design.md), [2A](2026-10-04-ghostline-phase2a-design.md), [engine zapret2](2026-10-05-ghostline-zapret2-design.md), [2B](2026-10-05-ghostline-phase2b-design.md), [3](2026-10-06-ghostline-phase3-design.md) (đã phát hành ở v0.5.1). Mọi thứ không nhắc lại ở đây giữ nguyên.

---

## 1. Mục tiêu

- **Ngang bằng Windows:** mọi tính năng của v0.5 chạy được trên Linux: DNS mã hoá cho toàn hệ thống, chọn server tự động, kiểm tra rò rỉ, vượt DPI bằng zapret2, proxy cục bộ có fragmentation, rule và danh sách, DNS server cho LAN, Fake SNI, công cụ chẩn đoán, backup. Ngoại lệ duy nhất: GoodbyeDPI chỉ có trên Windows.
- **Không bao giờ làm mất mạng:** cùng cam kết như Windows. DNS gốc luôn được trả lại, kể cả khi app crash, bị kill hay máy mất điện.
- **Chạy nền trên SteamOS:** bật ở Desktop Mode, chuyển sang Game Mode và khởi động lại máy vẫn được bảo vệ.
- **Windows không đổi gì:** không chậm hơn, exe không phình to, hành vi giữ nguyên.
- **Dễ bảo trì cho nhiều hệ điều hành:** mỗi khác biệt giữa các OS chỉ nằm ở một chỗ. **Có kế hoạch hỗ trợ macOS sau này**, nên mọi interface mới phải trung lập về OS (mục 20).

### Tiêu chí thành công

1. Trên Ubuntu 24.04, Fedora 41, Arch và SteamOS: cài gói, bấm Connect, thì `resolvectl query`/`dig` cho thấy truy vấn đi qua Ghostline và phần kiểm tra rò rỉ báo sạch. Disconnect thì DNS trở về đúng như trước khi Connect.
2. `kill -9` daemon khi đang kết nối: trong ≤ 5 giây DNS đã về như cũ và mạng vẫn chạy. Rút điện rồi bật lại: sau khi boot, DNS về như cũ, hoặc tự Connect lại nếu bật "khởi động cùng hệ thống".
3. Bật zapret2: bảng nftables `inet ghostline` xuất hiện và site bị chặn mở được. `kill -9 nfqws2` thì mạng không bị chặn. Dừng daemon thì bảng biến mất.
4. Đóng GUI, hoặc trên SteamOS chuyển sang Game Mode: DNS và DPI vẫn hoạt động.
5. GUI chạy bằng user thường, không bao giờ chạy root. User không thuộc nhóm admin hay nhóm `ghostline` thì không điều khiển được daemon.
6. Gỡ gói (hoặc `--uninstall-system`): DNS, proxy hệ thống, chứng chỉ, luật firewall và luật nftables đều trở về như trước khi cài, và không còn file nào của Ghostline ngoài `/var/lib/ghostline` (xoá được bằng `purge`).
7. Bản Windows v0.6: exe tăng không quá 2% so với v0.5.1 và không liên kết gói nào chỉ dành cho Linux (CI kiểm chứng). Mọi test Windows hiện có vẫn qua.
8. `GOOS=linux` và `GOOS=windows` đều lint, build và test xanh trên CI. Mọi chuỗi mới có đủ tiếng Việt và tiếng Anh.

## 2. Phạm vi

### Có

| Nhóm | Nội dung |
|---|---|
| Nền tảng | Build tag, interface hoá phần còn dính Windows, điểm ghép nối duy nhất `internal/platform`, CI hai nền tảng |
| Daemon + GUI | `ghostlined` (root, systemd), GUI Wails GTK4 chạy bằng user, RPC qua Unix socket, proxy `Service` sinh tự động, session-agent |
| DNS hệ thống | 3 backend: NetworkManager, systemd-resolved, `/etc/resolv.conf` thuần. Theo dõi và áp lại cấu hình, flush cache, tìm tiến trình chiếm cổng 53 |
| Khôi phục | 4 tầng của Windows chuyển sang systemd: `ExecStopPost`, khôi phục khi daemon khởi động, xử lý sleep/resume |
| DPI | zapret2 `nfqws2` + NFQUEUE, luật nftables trong bảng riêng, fail-open |
| Tích hợp | Proxy hệ thống (GNOME, KDE), kho chứng chỉ (hệ thống, Firefox policy, NSS của user), firewall (firewalld, ufw), định danh mạng, SSID, secrets |
| Đóng gói | `.deb`, `.rpm`, AUR `ghostline-bin`, AppImage, `tar.gz`. Cách cài trên SteamOS. Tài liệu Linux |

### Không có

- Ubuntu 20.04/22.04, Debian 12 và mọi distro không có `webkitgtk-6.0` (không làm bản GTK3).
- arm64 và các kiến trúc khác ngoài x86_64 (để sau).
- Flatpak/Snap cho cả app. Flatpak cho riêng GUI chỉ được cân nhắc nếu spike SteamOS ở L5 thất bại (mục 16.4).
- GoodbyeDPI trên Linux. Điều khiển Ghostline ngay trong Game Mode (plugin Decky, thêm vào Steam).
- Tự đặt proxy hệ thống cho XFCE, wlroots và các desktop khác ngoài GNOME/KDE.
- Ký GPG cho gói, kho apt/dnf riêng, đưa vào kho chính thức của distro.
- macOS (chỉ chuẩn bị interface, mục 20).
- Từ các giai đoạn trước vẫn chưa làm: tự tải và cài bản cập nhật, ký số file exe. **Không bao giờ có telemetry.**

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Phạm vi | Ngang bằng Windows trong một bản phát hành **v0.6.0**, làm theo 5 sub-spec L1–L5 nối tiếp |
| Distro | Chỉ GTK4 (`webkitgtk-6.0`): Ubuntu 24.04+, Debian 13+, Fedora 41+, Arch, SteamOS. Chỉ x86_64 |
| Mô hình quyền | **Daemon root + GUI user.** Không chạy GUI bằng root (`pkexec` cả app hỏng trên Wayland, và đóng GUI là mất bảo vệ) |
| Số binary trên Linux | Hai binary: `ghostlined` (`CGO_ENABLED=0`, static, không cần GTK) và `ghostline` (GUI). Windows giữ một exe |
| Mô hình trên Windows | **Giữ nguyên:** chạy trong một tiến trình, không daemon, không RPC |
| Giao tiếp GUI ↔ daemon | Unix socket `/run/ghostline/ctl.sock`, JSON-RPC theo dòng. Không dùng TCP loopback vì mọi user trên máy đều kết nối được |
| Phân quyền | Kiểm tra `SO_PEERCRED`: cho phép root và thành viên `wheel`/`sudo`/`admin`/`ghostline`. **Không dùng polkit lúc chạy**, vì polkit action nằm trong `/usr/share` mà thư mục này chỉ đọc trên SteamOS. `pkexec` chỉ dùng cho thao tác cài đặt một lần |
| Việc theo phiên user | Daemon chạy `ghostlined --session-agent` dưới UID của user (proxy hệ thống, NSS của user). Không cần GUI đang mở |
| DNS | Thứ tự backend: NetworkManager (`GlobalDnsConfiguration`) → systemd-resolved (drop-in trong `/run`) → `/etc/resolv.conf`. Cấu hình áp cho toàn hệ thống, không chọn adapter |
| DPI | `nfqws2` của bản zapret2 đang pin, luật nftables qua `github.com/google/nftables` (không cần lệnh `nft`), mọi luật `queue` có `bypass` |
| Settings | Chung cho cả máy, lưu ở `/var/lib/ghostline/data`. Tuỳ chọn riêng của cửa sổ ở `~/.config/ghostline` |
| Gói | `.deb`/`.rpm`/gói Arch qua **nfpm** từ một file cấu hình; AUR `ghostline-bin`; AppImage; `tar.gz` |
| Daemon từ AppImage/tar.gz | Luôn **sao chép** sang `/var/lib/ghostline/bin` (root, `0755`) trước khi chạy dưới quyền root. Không bao giờ để service root thực thi file user ghi được |
| Chuẩn bị cho macOS | Interface mới đặt tên và thiết kế trung lập; transport RPC, session-agent và việc giám sát daemon đều nằm sau interface (mục 20) |

## 4. Kiến trúc

### 4.1 Vai trò của từng binary

| Binary / chế độ | Nền tảng | Chạy bằng | Làm gì |
|---|---|---|---|
| `ghostline.exe` | Windows | Admin | Như hiện nay: GUI + toàn bộ `internal/app` trong một tiến trình, cùng các chế độ headless `--watchdog`/`--restore`/`--remove-certs`/`--export` |
| `ghostlined --daemon` | Linux | root (systemd) | Chạy `internal/app`: orchestrator, engine DNS, proxy, DPI, DNS server LAN, công cụ. Giữ `state.json`. Phục vụ RPC |
| `ghostlined --restore` / `--remove-certs` / `--export` | Linux | root | Các chế độ headless như Windows |
| `ghostlined --install-system` / `--uninstall-system` | Linux | root (`pkexec`/`sudo`) | Cài/gỡ dịch vụ từ AppImage hoặc tar.gz (mục 16.3) |
| `ghostlined status\|connect\|disconnect` | Linux | user được phép | CLI nhỏ qua RPC, dùng cho SSH và test |
| `ghostlined --session-agent` | Linux | UID của user (daemon khởi chạy) | Đặt/khôi phục proxy hệ thống, NSS của user (mục 5.5) |
| `ghostline` | Linux | user | GUI Wails GTK4 + tray, là client của daemon |

### 4.2 Điểm ghép nối duy nhất: `internal/platform`

`internal/platform` là nơi **duy nhất** chọn implementation theo OS. Mỗi OS có một file `platform_<os>.go` trả về:

```go
type Deps struct {
    DNS        sysdns.Backend      // snapshot / áp / khôi phục DNS hệ thống
    DNSWatch   netwatch.Watcher    // báo thay đổi mạng (đã debounce)
    SysProxy   sysproxy.API
    Certs      certstore.Store
    Firewall   firewall.Manager
    DPIFilter  dpi.PacketFilter    // WinDivert service hoặc bảng nftables
    DPIRunner  dpi.Runner
    Startup    startup.Manager     // Task Scheduler / systemd / sau này launchd
    Secrets    secrets.Protector   // DPAPI / khoá file
    NetID      netid.Source        // gateway + MAC + SSID
    Lock       store.Locker
    Procs      procs.Inspector     // tiến trình sống/chết, ai giữ cổng
    UsesDaemon bool                // true trên Linux: GUI là client
}
```

`internal/shell` và `headless.go` không còn gọi `NewWindows*`, `winutil` hay `schtasks` trực tiếp; chúng chỉ đọc từ `platform.Deps`. `UsesDaemon` là cờ do `platform` đặt, **không** phải một nhánh `runtime.GOOS` trong code dùng chung.

### 4.3 Package mới

| Package | Vai trò | Ghi chú |
|---|---|---|
| `internal/platform` | Điểm ghép nối (4.2) | `platform_windows.go`, `platform_linux.go` |
| `internal/rpc` | Giao thức, server, client, xác thực peer | Go thuần. Không được import trong bản Windows (CI kiểm tra) |
| `internal/rpc/gen` + `tools/genrpc` | Sinh proxy `Service` phía GUI | Chạy bằng `go generate`; CI kiểm tra bản sinh còn khớp |
| `internal/daemon` | Vòng đời daemon: khởi động, khôi phục, phục vụ RPC, tín hiệu, sleep/resume | Dùng `internal/app` y như shell Windows |
| `internal/sessionagent` | Chạy lệnh theo phiên user dưới UID của user | Daemon gọi; một tiến trình con cho mỗi tác vụ |
| `internal/firewall` | Tách từ `winutil/firewall*.go`: interface + netsh / firewalld / ufw | Sửa luôn lỗi `firewall.go:100` gọi `netsh` không có build tag |
| `internal/secrets` | Interface `Protector` + DPAPI / AES-GCM với khoá file | |
| `internal/netid` | Gateway, MAC gateway, SSID, Wi-Fi đã lưu → `scanner.NetworkKey` | Chuyển `networkKey`/`gatewayMAC` khỏi `shell/system_windows.go` |
| `internal/netwatch` | Báo thay đổi mạng | Windows: `NotifyIpInterfaceChange`. Linux: netlink + tín hiệu D-Bus + inotify |
| `internal/procs` | Tiến trình còn sống, thời điểm khởi động, ai giữ cổng | Windows: code hiện có trong `winutil`. Linux: `/proc` |
| `cmd/ghostlined` | `main` của daemon | Chỉ build cho Linux (sau này thêm macOS) |

### 4.4 Thay đổi ở package cũ

| Package | Thay đổi |
|---|---|
| `internal/app` | Phát event qua `Sink` thay vì gọi thẳng `wails/application`, để `ghostlined` build được không cần CGO. Kiểu `winutil.PortOwner` trong `deps.go` thay bằng `procs.PortOwner`. Dialog/mở URL/hỏi xác nhận đi qua interface `UI` (mục 5.4) |
| `internal/sysdns` | `API` tách thành `Manager` dùng chung (retry, thứ tự bước) và `Backend` riêng mỗi OS. `NetshSetDNS`, GUID, LUID, IfType rút khỏi interface chung vào backend Windows |
| `internal/sysproxy` | `SysProxySnapshot` thành dạng có trường riêng cho mỗi OS. Thêm backend GNOME/KDE chạy qua session-agent |
| `internal/certstore` | Thêm store gộp Linux (mục 10). `Location` vẫn chỉ có ở Windows |
| `internal/startup` | Thành interface `Manager`. Windows: `schtasks` như cũ. Linux: bật/tắt tự Connect lúc boot + file autostart của tray |
| `internal/dpi` | `Services` (WinDivert) thành `PacketFilter`. `zapret2` tách `args_windows.go`/`args_linux.go`. Bảng cổng/chiều dùng chung (mục 8.2) |
| `assets/zapret2`, `assets/goodbyedpi` | Embed tách theo build tag. GoodbyeDPI chỉ build cho Windows |
| `internal/store` | `ResolvePaths` có bản Linux (mục 4.6). `state.json` v4 (mục 15) |
| `internal/winutil` | Chỉ còn helper Windows thuần. Mọi file đều có tag `windows`. Phần dùng chung (`lanip.go`) chuyển sang `internal/netid` hoặc giữ nguyên nếu không đụng OS |
| `internal/shell` | Đọc `platform.Deps`. Trên Linux nhận proxy RPC thay vì `*app.Service` thật. `system_windows.go` thu gọn còn phần UI Windows (WebView2, `MessageBox`, `WndProc`) |
| `internal/watchdog` | Giữ nguyên logic. Tên luật firewall trong `AllFirewallRules` lấy từ `firewall.Manager` thay vì ghi cứng tên netsh |
| `tools/fetchdpi` | Lấy thêm `binaries/linux-x86_64/nfqws2` và đối chiếu `sha256sum.txt` |
| `frontend` | Thêm `platform` vào snapshot; helper `t(key)` tự chọn biến thể `.linux`; Settings bỏ ô chọn adapter trên Linux |

### 4.5 Ranh giới

- Code dùng chung **không bao giờ** rẽ nhánh theo `runtime.GOOS`. Khác biệt chỉ nằm trong file `_<os>.go` và chỉ được chọn ở `internal/platform`.
- File `_<os>.go` chỉ gọi syscall, D-Bus, netlink hoặc lệnh hệ thống. Retry, thứ tự bước và so sánh snapshot nằm trong code dùng chung để chỉ test một lần.
- `internal/app` không biết mình chạy trong GUI (Windows) hay trong daemon (Linux).
- Bản Windows không import `internal/rpc`, `internal/daemon`, `internal/sessionagent`, `google/nftables`, `godbus`. `ghostlined` không import `wails` hay `x/sys/windows`. CI kiểm chứng cả hai (mục 17).

### 4.6 Thư mục dữ liệu trên Linux

| Đường dẫn | Chủ / quyền | Nội dung |
|---|---|---|
| `/var/lib/ghostline/` | root `0755` (systemd `StateDirectory=`, `StateDirectoryMode=0755`) | Chỉ chứa hai thư mục dưới |
| `/var/lib/ghostline/data/` | root `0700` | `settings.json`, `state.json`, rule, danh sách, cache, `secret.key`, CA cho LAN, engine `nfqws2` đã giải nén và kiểm hash |
| `/var/lib/ghostline/bin/` | root `0755` | `ghostlined` khi cài từ AppImage/tar.gz. Phải đọc/thực thi được bởi user, vì session-agent chạy dưới UID của user |
| `/var/log/ghostline/` | root `0750` (`LogsDirectory=`) | Log xoay vòng như Windows; stdout của daemon vào journald |
| `/run/ghostline/` | root `0755` (`RuntimeDirectory=`) | `ctl.sock` (`0666`, phân quyền ở mục 5.1), `state.lock` |
| `~/.config/ghostline/` | user | Tuỳ chọn riêng của GUI (kích thước cửa sổ, chế độ Đơn giản/Nâng cao) |

Không có khái niệm "portable" như Windows: daemon chạy cho cả máy nên dữ liệu luôn ở `/var/lib/ghostline`.

## 5. Daemon và GUI client

### 5.1 Socket và phân quyền

- Daemon mở `/run/ghostline/ctl.sock` (`0666`). Mỗi kết nối đọc `SO_PEERCRED` (uid, gid, pid) một lần.
- **Được phép** nếu uid là 0, hoặc user thuộc một trong các group `wheel`, `sudo`, `admin`, `ghostline` (tra bằng `os/user`, kể cả group phụ). Không được phép thì daemon trả lỗi `NOT_AUTHORIZED` rồi đóng kết nối.
- Group `ghostline` do gói tạo (`sysusers.d`) để admin cấp quyền cho user không thuộc nhóm admin.
- Quyết định áp cho cả kết nối. Mức bảo vệ tương đương Windows: ai chạy được app admin thì điều khiển được.

### 5.2 Giao thức

- JSON-RPC theo dòng (mỗi thông điệp một dòng JSON, giới hạn 4 MiB mỗi dòng):
  - `{"id":n,"call":"Method","args":[…]}` → `{"id":n,"result":…}` hoặc `{"id":n,"error":{"code":…,"message":…}}`
  - `{"event":"state","data":…}` do daemon đẩy cho mọi client đang kết nối (khoảng 15 loại event hiện có ở `app/events.go`)
  - `{"hello":{"version":"0.6.0","protocol":1}}` là thông điệp đầu tiên của mỗi bên
- Server dùng reflection trên `*app.Service`; chỉ method exported nằm trong danh sách sinh tự động mới được gọi.
- Transport là một interface (`net.Listener`/`net.Conn`). Unix socket chạy được trên cả Linux và macOS.

### 5.3 Proxy `Service` sinh tự động

- `tools/genrpc` đọc tập method của `app.Service` và sinh một kiểu có **đúng** các method đó; mỗi method chỉ gọi `client.Call`. GUI Linux bind kiểu này vào Wails thay vì `*app.Service`.
- Thêm method mới vào `Service` thì chỉ cần chạy `go generate`, không sửa RPC bằng tay. CI chạy `go generate ./...` rồi `git diff --exit-code`.
- **Điểm chưa chắc (spike đầu L2):** Wails v3 tính ID binding theo đường dẫn package. Nếu proxy ở package khác làm lệch ID thì có hai cách, chọn trong spike: sinh proxy với tên/đường dẫn trùng qua tuỳ chọn service của Wails, hoặc một adapter TS mỏng ánh xạ binding cũ sang proxy. Frontend không được phải sửa từng chỗ gọi.

### 5.4 Việc chỉ GUI làm được

- `internal/app` gọi interface `UI` cho: chọn file mở/lưu, mở URL, hộp thoại hỏi xác nhận. Trên Windows `UI` là Wails trong cùng tiến trình như hiện nay. Trên Linux `UI` nằm ở GUI; daemon không gọi `UI`.
- Backup: GUI chọn file và đọc/ghi bytes; RPC chỉ chuyển nội dung (`ExportSettingsBytes`/`ImportSettingsBytes`). **Daemon không bao giờ ghi file vào thư mục của user.** Tương tự cho `ExportAdvancedCSV`.
- Không có GUI nào kết nối thì các thao tác cần `UI` trả lỗi `NO_UI`; các thao tác khác vẫn chạy.

### 5.5 Session-agent

- Việc theo phiên user: proxy hệ thống (gsettings, `kioslaverc`), NSS của user (`certutil`).
- Daemon chạy `ghostlined --session-agent <task>` với `SysProcAttr.Credential` = uid/gid của user, `HOME`, `XDG_RUNTIME_DIR=/run/user/<uid>`, `DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/<uid>/bus`. Nhiệm vụ và tham số truyền qua stdin dạng JSON, kết quả qua stdout.
- UID là của peer đã bật tính năng, được ghi vào snapshot để khôi phục đúng user. User không còn phiên nào (`/run/user/<uid>` không tồn tại) thì việc khôi phục được hoãn, ghi vào `state.json` và làm lại khi user đăng nhập (daemon theo dõi `UserNew` của logind).
- Binary dùng cho session-agent là bản `ghostlined` trong thư mục root, không phải binary user ghi được.

### 5.6 Bắt tay phiên bản

- GUI và daemon trao `hello`. Khác `protocol` thì GUI hiện trang "Cần cập nhật dịch vụ nền" kèm nút chạy `pkexec ghostlined --install-system` (bản AppImage/tar.gz) hoặc hướng dẫn cập nhật gói.
- Khác `version` nhưng cùng `protocol` thì chỉ hiện cảnh báo nhỏ trong Cài đặt.
- Không kết nối được daemon: GUI hiện trang `DAEMON_UNREACHABLE` kèm nút "Khởi động dịch vụ" (`pkexec systemctl enable --now ghostline`) hoặc "Cài dịch vụ" (AppImage/tar.gz chưa cài).

## 6. DNS hệ thống trên Linux

### 6.1 Interface

`sysdns.Manager` (dùng chung) làm `Detect → Snapshot → Apply(loopback) → Verify → Restore → Flush`, kèm retry và log. `sysdns.Backend` (mỗi OS, mỗi cơ chế) chỉ có các thao tác nguyên thuỷ:

```go
type Backend interface {
    Name() string                         // "networkmanager", "resolved", "resolvconf", "windows"
    Available() bool
    Snapshot() (DNSSnapshot, error)
    Apply(servers []netip.Addr) error     // 127.0.0.1, ::1
    Restore(DNSSnapshot) error
    Flush() error
    Describe() BackendInfo                // cho trang Cài đặt
}
```

Linux có một danh sách theo thứ tự `[]Backend{nm, resolved, resolvconf}`; `Detect` lấy backend đầu tiên `Available()`. Backend được ghi vào snapshot, nên khôi phục luôn dùng đúng backend đã áp.

### 6.2 Ba backend

| Backend | Khi nào | Áp | Snapshot / khôi phục |
|---|---|---|---|
| NetworkManager | Có `org.freedesktop.NetworkManager` trên system bus | Đặt thuộc tính `GlobalDnsConfiguration` = `{searches: [], domains: {"*": {servers: ["127.0.0.1","::1"]}}}`. NM bỏ qua DNS của từng kết nối và tự đẩy xuống resolved/`resolv.conf`; DHCP renew hay kết nối lại không ghi đè | Lưu giá trị cũ (thường rỗng), khôi phục bằng cách đặt lại đúng giá trị đó. **NM ghi giá trị này xuống đĩa**, nên các tầng khôi phục (mục 7) bắt buộc xoá nó |
| systemd-resolved | Có `org.freedesktop.resolve1`, không có NM | Ghi `/run/systemd/resolved.conf.d/ghostline.conf` (`DNS=127.0.0.1 ::1`, `Domains=~.`) rồi reload resolved. Qua D-Bus đặt `SetLinkDefaultRoute(false)` cho mọi link đang có DNS, để resolved không gửi song song ra DNS nhà mạng | Lưu cờ DefaultRoute từng link. Xoá file drop-in và đặt lại cờ. `/run` là tmpfs nên reboot là tự sạch |
| `/etc/resolv.conf` | Không có cả hai | Ghi `nameserver 127.0.0.1`, `nameserver ::1`, `options edns0 trust-ad` (ghi file tạm rồi `rename`) | Lưu nội dung, đích symlink (nếu là symlink) và quyền. Khôi phục đúng như cũ. **Không** dùng `chattr +i` |

Kiểm tra lại sau khi áp (`Verify`): đọc lại cấu hình hiệu lực (`resolve1` `DNS`/`CurrentDNSServer`, hoặc `resolv.conf`) và so với `127.0.0.1`/`::1`. Sai thì báo `DNS_APPLY_FAILED` và khôi phục.

### 6.3 Theo dõi và áp lại

- `netwatch` Linux: netlink (`RTMGRP_LINK`, route IPv4/IPv6), `PropertiesChanged` của NM/resolved, inotify trên `/etc/resolv.conf` và thư mục cha của nó.
- Debounce 2 giây rồi gọi lại luồng hiện có của orchestrator (giống `Watch` Windows): kiểm tra cấu hình hiệu lực, áp lại nếu bị đổi, cập nhật khoá mạng (mục 12).

### 6.4 Flush cache

`resolve1.FlushCaches` nếu có resolved; `nscd -i hosts` nếu có `nscd`. Không có gì để flush thì bỏ qua.

### 6.5 Cổng 53

- Mặc định giữ `127.0.0.1:53` và `[::1]:53`. resolved nghe ở `127.0.0.53`/`127.0.0.54` nên không xung đột.
- Bị chiếm (dnsmasq, libvirt, pi-hole…): `procs` tìm chủ bằng `/proc/net/{tcp,udp}{,6}` → inode → `/proc/*/fd`, tìm unit systemd từ `/proc/<pid>/cgroup`. Lựa chọn "dừng dịch vụ xung đột" hiện như Windows và gọi `systemctl stop <unit>` qua D-Bus của systemd.

### 6.6 Trang Cài đặt

Thay ô chọn adapter (theo GUID) bằng dòng chỉ đọc: backend đang dùng (ví dụ "NetworkManager → systemd-resolved") và danh sách interface. `Settings.Adapters` giữ nguyên ý nghĩa trên Windows và bị bỏ qua trên Linux.

## 7. Khôi phục trên Linux

| Tầng trên Windows | Trên Linux |
|---|---|
| 1. Ngắt kết nối sạch | Giống hệt |
| 2. Tiến trình `--watchdog` | `ExecStopPost=ghostlined --restore` trong unit. systemd chạy lệnh này **cả khi daemon crash hoặc bị kill**. Sau đó `Restart=on-failure` khởi động lại daemon ở trạng thái chưa kết nối |
| 3. Khôi phục khi mở lại app | Daemon luôn bật lúc boot (`WantedBy=multi-user.target`, `After=NetworkManager.service systemd-resolved.service`). Khởi động mà `state.json` còn dở thì chạy `watchdog.RunRestore` trước mọi việc khác |
| 4. Task `--restore` lúc đăng nhập | Thừa: tầng 3 chạy ở mọi lần boot, kể cả sau mất điện |
| `WM_ENDSESSION` / resume | Tắt máy: systemd dừng daemon → ngắt kết nối sạch → lần boot sau sạch. Ngủ/thức: theo dõi `PrepareForSleep` của logind, khi thức dậy chạy lại luồng `onResume` hiện có |

- `watchdog.Deps` trên Linux: `StopDPI` xoá bảng `inet ghostline` và kill `nfqws2` sót lại; `RestoreSysProxy` gọi session-agent; `DeleteRule` gọi `firewall.Manager`; `RemoveCert`/`SweepSession` dùng store gộp (mục 10).
- `--restore` dùng lock `/run/ghostline/state.lock` (`flock`) thay cho named mutex.
- "Khởi động cùng hệ thống" trên Linux nghĩa là **tự Connect lúc boot**. Daemon lúc nào cũng chạy vì nó giữ vai trò khôi phục.

## 8. DPI zapret2 trên Linux

### 8.1 Engine

- `nfqws2` (linux-x86_64) trong bản zapret2 **v1.0.5.2** đang pin. Cùng cú pháp `--lua-desync`, cùng file Lua và `strategies.json` với `winws2`. Auto-tune, blacklist, tự phát hiện site bị chặn, DoH fragmentation giữ nguyên logic.
- Pin thêm SHA-256 của `nfqws2` vào `zapret2/pins.go`. `tools/fetchdpi` lấy thêm thư mục `binaries/linux-x86_64/` và đối chiếu với `sha256sum.txt` của release. **Hash phải lấy lại trực tiếp từ release khi làm L4**, không dùng số đọc qua công cụ tóm tắt.
- Danh sách host truyền thẳng đường dẫn (`--hostlist=`, `--hostlist-auto=`); không cần copy ra `blacklist.txt` như Windows. `HotReloadsLists` vẫn là `true`.
- `strategies.json` thêm trường tuỳ chọn `platforms` (mặc định mọi nền tảng). **Trước khi dùng:** kiểm tra `strategies.Parse` của v0.5 bỏ qua trường lạ; nếu không thì tăng phiên bản định dạng và giữ file cũ cho client cũ.

### 8.2 Một bảng cổng và chiều gói cho mọi nền tảng

`zapret2/filter.go` (dùng chung) khai báo cổng, giao thức, chiều và số gói đầu cần bắt. `args_windows.go` sinh `--wf-tcp-out`/`--wf-udp-out`/`--wf-dup-check` từ bảng này; `nftfilter_linux.go` sinh luật nft cũng từ bảng này. Hai nền tảng không thể lệch nhau.

### 8.3 Luật nftables

Bảng riêng `inet ghostline`, cài qua `github.com/google/nftables` (netlink, không cần lệnh `nft`):

- Chain `post`: hook `postrouting`, priority `mangle`.
  - Bỏ qua gói có fwmark của `nfqws2` (chống lặp), `oifname lo`, đích LAN/private/loopback.
  - TCP 80/443 và UDP 443 (QUIC) chỉ cho `ct original packets 1-N` → `queue num 200 bypass`.
- Chain `pre`: hook `prerouting`, vài gói trả về đầu tiên của TCP 80/443 → `queue num 200 bypass` (cho autohostlist).
- `bypass`: nếu không có tiến trình nào giữ queue, gói đi thẳng. **Không bao giờ chặn mạng.**
- Gỡ = xoá cả bảng; không đụng tới bảng của firewalld/ufw/docker. Luật nftables chỉ tồn tại khi máy đang chạy.
- Giá trị fwmark, số queue và N lấy từ bảng ở 8.2 và từ tài liệu `nfqws2`; spike đầu L4 kiểm chứng tên cờ fwmark và cờ hạ quyền của `nfqws2` trong bản pin.

### 8.4 Vòng đời tiến trình

- `nfqws2` là con của daemon, `Pdeathsig=SIGKILL`, nằm trong cgroup của unit (`KillMode=control-group`).
- Thứ tự Start: kiểm hash → cài bảng nft → chạy `nfqws2` → đợi queue được giữ (tối đa 2 giây, giống Windows) → báo đã chạy. Stop: kill `nfqws2` → xoá bảng.
- Hạ quyền: nếu bản pin hỗ trợ (spike L4) thì `nfqws2` chạy với cờ hạ quyền sau khi mở queue.

### 8.5 Kernel

Trước Start kiểm tra `nfnetlink_queue`, `nft_queue`, `nf_conntrack` (qua `/proc/modules` và `/sys/module`); thiếu thì `modprobe`. Vẫn không được thì báo `DPI_KERNEL_UNSUPPORTED`, và UI gợi ý bật proxy (fragmentation không cần kernel). Không có nhánh lùi về GoodbyeDPI.

## 9. Proxy hệ thống

Chạy qua session-agent (mục 5.5). Mỗi desktop là một backend; `Detect` đọc `XDG_CURRENT_DESKTOP` của phiên user.

| Desktop | Đặt | Snapshot / theo dõi |
|---|---|---|
| GNOME, Cinnamon, Budgie, Unity | gsettings `org.gnome.system.proxy`: `mode=manual`, `http`/`https`/`socks` host+port, `ignore-hosts` | Lưu mọi key cũ; theo dõi bằng `gsettings monitor` |
| KDE Plasma (gồm SteamOS Desktop) | `[Proxy Settings]` trong `~/.config/kioslaverc` qua `kwriteconfig6` (hoặc `5`), rồi tín hiệu D-Bus `org.kde.KIO.Scheduler.reparseSlaveConfiguration` | Lưu các key cũ; inotify |
| Khác | Không tự đặt; trang Proxy hiện địa chỉ và hướng dẫn tay; lỗi `PROXY_DESKTOP_UNSUPPORTED` nếu bật "dùng cho máy này" | — |

Danh sách bypass định dạng WinINET chuyển thành `ignore-hosts`/`NoProxyFor` bằng một hàm dùng chung có test.

## 10. Kho chứng chỉ

Store gộp Linux, implement `certstore.Store` (`Install`/`Remove`/`List`), luôn chỉ đụng chứng chỉ có tiền tố `certs.SessionPrefix`:

| Đích | Ai làm | Cách |
|---|---|---|
| Kho hệ thống | daemon | Debian/Ubuntu: `/usr/local/share/ca-certificates/<tên>.crt` + `update-ca-certificates`. Fedora: `/etc/pki/ca-trust/source/anchors/` + `update-ca-trust`. Arch/SteamOS: `/etc/ca-certificates/trust-source/anchors/` + `update-ca-trust`. Chọn theo công cụ có sẵn trên máy |
| Firefox | daemon | `Certificates.Install` trong `/etc/firefox/policies/policies.json` (bản deb, rpm và snap đều đọc). Chỉ thêm/xoá mục của Ghostline, giữ nguyên mọi policy khác; ghi file tạm rồi `rename` |
| Chrome/Chromium (nơi NSS không đọc p11-kit, như Ubuntu) | session-agent | `certutil -d sql:$HOME/.pki/nssdb -A/-D`. Không có `certutil` thì báo `CERT_NSS_TOOL_MISSING` (gợi ý cài `libnss3-tools`) |

Fedora và Arch: NSS đọc kho hệ thống qua p11-kit, nên Chrome và Firefox nhận chứng chỉ từ đích đầu tiên. CA cho LAN (cho điện thoại) không cài vào máy này, nên không đổi.

## 11. Firewall (chia sẻ proxy và DNS server trên LAN)

`firewall.Manager` (`Allow(rule)`/`Remove(rule)`/`List()`), các backend Linux theo thứ tự:

| Backend | Khi nào | Cách |
|---|---|---|
| firewalld | `org.fedoraproject.FirewallD1` đang chạy | `addPort` **runtime** (không permanent) trong zone của interface; reload/reboot là tự mất |
| ufw | `ufw status` là active | `ufw allow <port>/<proto> comment ghostline-<rule>`; gỡ và khôi phục theo comment |
| Không có | Còn lại | Không mở gì. Nếu thấy chain `input` policy drop của bên khác thì cảnh báo, vì accept trong bảng của Ghostline không thắng drop ở bảng khác |

Kiểm tra "mạng Public" của Windows: interface nằm trong zone firewalld `public`/`external`/`block`/`drop` thì hiện cảnh báo như cũ. Không có firewalld thì bỏ qua kiểm tra này.

## 12. Định danh mạng và SSID

- Gateway và interface từ netlink `RTM_GETROUTE`. MAC gateway từ bảng neighbor `RTM_GETNEIGH`; chưa có thì gửi một gói UDP tới gateway để kích ARP rồi đọc lại; vẫn không có thì dùng MAC của chính adapter (như Windows). Kết quả đưa vào `scanner.NetworkKey` không đổi, nên xếp hạng server theo mạng giữ nguyên.
- SSID và danh sách Wi-Fi đã lưu: D-Bus của NetworkManager (`ActiveAccessPoint.Ssid`, `Settings.ListConnections` loại `802-11-wireless`). SteamOS dùng NM với backend iwd nên API như nhau. Không có NM thì không có SSID (tính năng dùng SSID hiện "không xác định").

## 13. Thay thế các phần còn lại của `winutil`

| Windows | Linux | Interface |
|---|---|---|
| DPAPI user (`PassEnc`), DPAPI máy (khoá CA cho LAN) | AES-GCM với khoá 32 byte ở `/var/lib/ghostline/data/secret.key` (root `0600`, tạo lần đầu). Định dạng trường giữ nguyên (base64), gắn với máy; backup vẫn bỏ qua | `secrets.Protector` |
| Named mutex `Local\Ghostline-State` | `flock` trên `/run/ghostline/state.lock` | `store.Locker` |
| `SecureDir` (SDDL SY+BA), `OwnedByAdmins` | `data/` root `0700` do daemon tạo; kiểm tra chủ là root và quyền đúng mỗi lần khởi động | trong `platform` |
| `PortOwner(s)`, `ProcessAlive`, `ProcessStartTime`, `ProcessName` | `/proc` | `procs.Inspector` |
| `StartDetached`, `Job`, `HiddenCmd` | systemd và `Pdeathsig` | không cần interface chung |
| `AttachParentConsole`, `SmallIconSize` | Không cần (CLI có stdout; icon cố định) | — |
| `IsAdmin` → `NOT_ADMIN` | Phân quyền ở socket → `NOT_AUTHORIZED` | — |
| `MessageBox` (thiếu WebView2) | Trang lỗi trong GUI (mục 5.6) | — |
| `CurrentSSID`, `WifiNames` | NM D-Bus (mục 12) | `netid.Source` |
| Service SCM (`StopService` cho cổng 53) | systemd D-Bus | `procs.Inspector` |

## 14. Giao diện

- Snapshot gửi frontend có thêm `platform` (`"windows"`, `"linux"`). Frontend có helper `t(key)`: nếu có khoá `<key>.linux` (hoặc sau này `.darwin`) thì dùng, không thì dùng khoá gốc. Component không tự rẽ nhánh theo nền tảng.
- Chuỗi cần biến thể `.linux` (theo khảo sát `en.json`/`vi.json`): loại trừ Defender, "chứng chỉ trong Windows", proxy Windows, WinDivert, svchost/Hotspot/WSL, Public/Private, "khởi động cùng Windows", `NOT_ADMIN`.
- Trang DPI trên Linux: ẩn GoodbyeDPI, phần loại trừ antivirus và đường dẫn `bin\zapret2`; hiện trạng thái bảng nftables và queue.
- Trang Cài đặt: dòng backend DNS (mục 6.6); "Khởi động cùng hệ thống" gồm hai tuỳ chọn: tự Connect lúc boot (daemon) và mở tray khi đăng nhập (`~/.config/autostart/ghostline.desktop`, do GUI tự ghi).
- Tray dùng StatusNotifierItem của Wails. GNOME không có tray mặc định (Ubuntu có extension sẵn, Fedora thì không): khi không có tray, đóng cửa sổ là thoát GUI, bảo vệ vẫn chạy trong daemon. Lần đầu đóng cửa sổ hiện một thông báo giải thích điều này.
- Trang lỗi mới: `DAEMON_UNREACHABLE`, cần cập nhật dịch vụ, `PKEXEC_NO_PASSWORD` (SteamOS).

## 15. Settings và state

### `settings.json`

Giữ **phiên bản 5**, không thêm trường. `StartWithWindows` giữ tên JSON (tương thích backup giữa hai nền tảng) và trên Linux nghĩa là tự Connect lúc boot. `Adapters` bị bỏ qua trên Linux. Tuỳ chọn riêng GUI nằm ở `~/.config/ghostline`, không vào `settings.json`.

Backup từ Windows nhập vào Linux (và ngược lại) vẫn hợp lệ: các trường riêng của một nền tảng bị bỏ qua ở nền tảng kia; quy tắc an toàn của giai đoạn 3 (không tự bật Fake SNI, chia sẻ LAN) giữ nguyên.

### `state.json` phiên bản 4

```jsonc
{
  "version": 4,
  "phase": "...",
  "dns": {
    "backend": "networkmanager",          // hoặc "resolved", "resolvconf", "windows"
    "windows": { "adapters": [ /* AdapterSnapshot cũ */ ] },
    "linux": {
      "nmGlobalDns": { /* giá trị GlobalDnsConfiguration cũ */ },
      "resolvedLinks": [ { "ifindex": 2, "defaultRoute": true } ],
      "resolvConf": { "content": "...", "symlink": "", "mode": 420 }
    }
  },
  "sysProxy": {
    "windows": { /* SysProxySnapshot cũ */ },
    "linux": { "uid": 1000, "desktop": "kde", "keys": { "...": "..." } }
  },
  "pendingUserRestores": [ { "uid": 1000, "what": "sysproxy" } ]
  // các trường khác giữ nguyên
}
```

Migration v3 → v4: chuyển snapshot adapter và proxy cũ vào nhánh `windows`. Có test cho migration và cho việc đọc state v3 dở dang rồi khôi phục được.

## 16. Đóng gói và phát hành

### 16.1 File phát hành (x86_64)

| File | Cho ai | Cách làm |
|---|---|---|
| `ghostline_<ver>_amd64.deb` | Ubuntu 24.04+, Debian 13+ | nfpm. Phụ thuộc `libgtk-4-1`, `libwebkitgtk-6.0-4`; gợi ý `libnss3-tools` |
| `ghostline-<ver>.x86_64.rpm` | Fedora 41+ | nfpm. Phụ thuộc `gtk4`, `webkitgtk6.0` |
| AUR `ghostline-bin` | Arch, Manjaro, CachyOS | PKGBUILD lấy `tar.gz` từ release; CI đẩy lên AUR bằng SSH key trong secret, chỉ sau khi release đã đăng |
| `Ghostline-<ver>-x86_64.AppImage` | Distro khác, SteamOS | GUI + `ghostlined`; lần đầu đề nghị cài dịch vụ |
| `ghostline-<ver>-linux-amd64.tar.gz` | Tự cài | Hai binary, `install.sh`, `uninstall.sh`, file unit, `.desktop`, icon |
| `SHA256SUMS` | | Một file chung cho Windows và Linux |

Build trên `ubuntu-24.04` (glibc 2.39). `ghostlined` static nên không phụ thuộc glibc.

### 16.2 Gói deb/rpm/Arch cài gì

- `/usr/bin/ghostline`, `/usr/lib/ghostline/ghostlined`.
- `/usr/lib/systemd/system/ghostline.service`: `ExecStart=… --daemon`, `ExecStopPost=… --restore`, `Restart=on-failure`, `StateDirectory=ghostline`, `LogsDirectory=ghostline`, `RuntimeDirectory=ghostline`, `CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SETUID CAP_SETGID CAP_KILL CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER`, `NoNewPrivileges=yes` (hạ UID cho session-agent không cần đặc quyền mới). **Không** dùng `ProtectHome`, vì session-agent kế thừa namespace của daemon và phải ghi được `~/.config/kioslaverc` và `~/.pki`. Danh sách capability cuối cùng chốt khi làm L5 bằng cách chạy đủ bộ integration test với unit đã hardening.
- `/usr/lib/sysusers.d/ghostline.conf` (group `ghostline`), `/usr/share/applications/ghostline.desktop`, icon hicolor, `/usr/share/metainfo/` .
- **postinst:** `systemctl daemon-reload` + `enable --now ghostline.service`.
- **prerm** (gỡ, không phải nâng cấp): `ghostlined --remove-certs` (khôi phục DNS/proxy, gỡ chứng chỉ khỏi cả ba đích, xoá bảng nft, gỡ luật firewall, gỡ mục policy Firefox), rồi `disable --now`.
- **purge** (deb) / ghi chú trong tài liệu (rpm, Arch): xoá `/var/lib/ghostline` và `/var/log/ghostline`.

### 16.3 AppImage, tar.gz

- `ghostlined --install-system` (qua `pkexec` từ GUI, hoặc `sudo ./install.sh`):
  1. Sao chép `ghostlined` vào `/var/lib/ghostline/bin/` (root, `0755`), kiểm SHA-256 với giá trị nhúng trong binary nguồn.
  2. Ghi unit vào `/etc/systemd/system/ghostline.service` (trỏ tới bản đã sao chép), group `ghostline` qua `groupadd` nếu chưa có.
  3. `daemon-reload` + `enable --now`.
- GUI của AppImage tự ghi `.desktop` cho user nếu người dùng đồng ý (không cần root).
- `ghostlined --uninstall-system`: làm như prerm, rồi xoá unit và `/var/lib/ghostline/bin`.

### 16.4 SteamOS

- Root filesystem chỉ đọc; `/etc` và `/var` được giữ qua cập nhật → cài theo 16.3 bằng AppImage.
- User `deck` mặc định chưa có mật khẩu nên `pkexec` thất bại: GUI phát hiện (lỗi `PKEXEC_NO_PASSWORD`) và hướng dẫn chạy `passwd` trong Konsole.
- **Spike đầu L5 (trên Steam Deck thật):** ảnh SteamOS hiện tại có `webkitgtk-6.0` và glibc ≥ 2.39 không; kernel có đủ NFQUEUE/nft_queue không; `/etc/systemd/system` và `/var/lib` có thật sự còn sau một lần cập nhật SteamOS không. Nếu thiếu webkit/glibc: AppImage đóng gói kèm WebKitGTK, hoặc làm riêng GUI cho SteamOS dạng Flatpak (daemon vẫn cài theo 16.3, Flatpak được `--filesystem=/run/ghostline`). Chọn phương án trong spike và ghi lại vào sub-spec L5.

### 16.5 Tài liệu

- README/README.vi: badge nền tảng, mục Cài đặt cho từng distro, mục Hạn chế đã biết trên Linux (GNOME không tray, desktop ngoài GNOME/KDE không tự đặt proxy, Chrome trên Ubuntu cần `libnss3-tools`, kernel thiếu NFQUEUE, không có GoodbyeDPI, gói chưa ký GPG).
- `docs/user-guide.md` và `docs/huong-dan-su-dung.md`: phần Linux và phần SteamOS (Desktop Mode, `passwd`, cài dịch vụ, Game Mode vẫn được bảo vệ).
- `docs/platforms.md` (mới, cho người bảo trì): bảng tính năng × Windows × Linux, mỗi ô ghi cơ chế, file và test.
- `docs/release-checklist.md`: thêm danh sách test thủ công Linux (mục 19.3).

## 17. CI và release

- `ci.yml`: ma trận `windows-latest` + `ubuntu-24.04`.
  - Cả hai: lint (`golangci-lint` với GOOS tương ứng), `go test ./...`, race, build.
  - Linux: build `ghostlined` (`CGO_ENABLED=0`) và GUI; `nfpm` chế độ kiểm tra cấu hình.
  - Rào chắn:
    - `go generate ./... && git diff --exit-code` (proxy RPC và code sinh khác).
    - Test phụ thuộc: `GOOS=windows go list -deps .` không chứa `internal/rpc`, `internal/daemon`, `internal/sessionagent`, `github.com/google/nftables`, `github.com/godbus/dbus`; `go list -deps ./cmd/ghostlined` không chứa `github.com/wailsapp/wails` và `golang.org/x/sys/windows`.
    - Lint chéo: job Windows chạy thêm `GOOS=linux go vet ./...` và ngược lại, để lỗi build tag lộ ra ngay ở PR.
- `linux-integration` (ubuntu-24.04, `sudo`, tag `integration`): backend resolved và resolv.conf, bảng nft + `nfqws2` + `bypass`, kill-9 daemon rồi kiểm tra khôi phục, ufw, kho chứng chỉ Debian.
- `release.yml`: thêm job `linux` song song với job Windows, dùng chung artifact frontend. Job gộp tạo `SHA256SUMS`, đăng release, in kích thước exe Windows và **báo lỗi nếu tăng quá 2%** so với release trước. Job `aur` chạy sau cùng.
- `servers.yml` giữ nguyên.

## 18. Xử lý lỗi

Mã lỗi mới (khai báo một lần trong `app/errors.go`, có chuỗi vi/en):

| Mã | Khi nào | UI |
|---|---|---|
| `DAEMON_UNREACHABLE` | GUI không kết nối được socket | Trang lỗi + nút khởi động/cài dịch vụ |
| `DAEMON_PROTOCOL_MISMATCH` | `protocol` khác nhau | Trang "Cần cập nhật dịch vụ nền" |
| `NOT_AUTHORIZED` | Peer không thuộc nhóm được phép | Trang lỗi + hướng dẫn `usermod -aG ghostline <user>` |
| `NO_UI` | Thao tác cần hộp thoại khi không có GUI | Trả về cho CLI |
| `DNS_BACKEND_UNSUPPORTED` | Không backend nào `Available()` | Không cho Connect, giải thích |
| `DNS_APPLY_FAILED` | `Verify` sau khi áp thấy sai | Khôi phục, báo lỗi kèm backend |
| `DPI_KERNEL_UNSUPPORTED` | Thiếu module NFQUEUE/nft | Gợi ý bật proxy |
| `PROXY_DESKTOP_UNSUPPORTED` | Desktop không phải GNOME/KDE | Hiện hướng dẫn cấu hình tay |
| `CERT_NSS_TOOL_MISSING` | Cần `certutil` mà không có | Cảnh báo, các đích khác vẫn cài |
| `PKEXEC_NO_PASSWORD` | `pkexec` thất bại vì user không có mật khẩu | Hướng dẫn `passwd` |
| `SESSION_UNAVAILABLE` | User đã đăng xuất khi cần khôi phục proxy/NSS | Hoãn, ghi `pendingUserRestores` |

Mọi lỗi trong `Restore` không dừng các bước còn lại (giữ `errors.Join` như `watchdog` hiện nay).

## 19. Kiểm thử

### 19.1 Unit test (CI, cả hai nền tảng khi có thể)

- Mỗi backend (NM, resolved, resolvconf, GNOME, KDE, firewalld, ufw, kho chứng chỉ) có fake D-Bus/filesystem/lệnh và test snapshot → áp → khôi phục cho ra đúng trạng thái cũ.
- **Contract test:** một bộ test cho mỗi interface (`sysdns.Backend`, `sysproxy.API`, `certstore.Store`, `firewall.Manager`, `dpi.PacketFilter`) chạy với fake ở CI và với implementation thật ở job integration.
- RPC: vòng gọi/trả, event, lỗi, `hello`, phân quyền (fake peercred), dòng quá 4 MiB.
- Sinh luật nft so với golden file; `Args()` Windows và Linux sinh từ cùng bảng 8.2; pins; lọc `platforms`.
- Migration `state.json` v3 → v4.
- Test hiện có của Windows chạy không đổi.

### 19.2 Integration (job `linux-integration`, root)

Như mục 17, cộng kịch bản: Connect → `kill -9 ghostlined` → trong 5 giây DNS về như cũ và `ExecStopPost` đã chạy.

### 19.3 Thủ công (trước mỗi bản phát hành, thêm vào `release-checklist.md`)

| Máy | Kiểm tra |
|---|---|
| Ubuntu 24.04 GNOME (VM) | Cài deb, Connect, rò rỉ, DPI, proxy GNOME, Fake SNI với Firefox snap và Chrome, ufw, gỡ sạch |
| Fedora 41 (VM) | Cài rpm, NM + resolved, firewalld zone, Fake SNI qua p11-kit, gỡ sạch |
| Arch KDE (VM hoặc máy thật) | Cài từ AUR, proxy KDE, resolv.conf thuần (tắt NM), gỡ sạch |
| Steam Deck | AppImage, `passwd`, cài dịch vụ, Connect + DPI, chuyển Game Mode, reboot, cập nhật SteamOS rồi kiểm tra lại |
| Bất kỳ | Rút điện khi đang kết nối → boot lại → DNS về như cũ |

## 20. Nguyên tắc bảo trì và chuẩn bị cho macOS

1. Mỗi package đụng OS có cùng hình dạng: `x.go` (interface, kiểu, **toàn bộ logic**), `x_<os>.go` (chỉ gọi hệ thống, càng mỏng càng tốt), fake để test.
2. Chỉ `internal/platform` chọn implementation. Thêm OS = thêm một `platform_<os>.go` và các file `_<os>.go`.
3. Backend trong một OS là danh sách theo thứ tự với `Available()`. Thêm backend = thêm một file và một dòng.
4. Mỗi loại dữ liệu chỉ có một nguồn: bảng cổng DPI, mã lỗi, chuỗi UI (biến thể theo nền tảng qua `t(key)`).
5. Không viết tay code lặp lại: proxy RPC sinh tự động, CI kiểm tra bản sinh còn khớp.
6. Contract test giữ fake khớp với thực tế và giữ các nền tảng hành xử như nhau.
7. Snapshot có phiên bản; phần riêng của mỗi OS nằm trong nhánh riêng; mỗi lần đổi định dạng có migration và test.
8. Rào chắn CI: lint chéo GOOS, test phụ thuộc, giới hạn kích thước exe, code sinh còn khớp.
9. Chỉ tách những file phải đụng vào và đang làm quá nhiều việc (ví dụ `shell.go`); không refactor chỗ không liên quan.
10. `docs/platforms.md` cho người bảo trì.

**Chuẩn bị cho macOS (không làm trong v0.6):**

- Interface mới đặt tên theo việc, không theo cơ chế: `dpi.PacketFilter` (không phải "Nft"), `sysdns.Backend`, `firewall.Manager`, `startup.Manager`, `netwatch.Watcher`. Không kiểu nào trong interface chung chứa khái niệm D-Bus, netlink, nftables, systemd hay WinDivert.
- Mô hình daemon + GUI client và giao thức RPC trên Unix socket dùng lại được trên macOS (launchd thay systemd, `getpeereid` thay `SO_PEERCRED`). Giám sát daemon nằm sau `startup.Manager`; xác thực peer nằm sau một hàm trong `internal/rpc` có file `_<os>.go`.
- `UsesDaemon` là cờ trong `platform.Deps`, nên macOS chỉ cần đặt `true`.
- Dữ liệu cho frontend dùng `platform` dạng chuỗi và helper `t(key)` đã hỗ trợ biến thể `.darwin`.

## 21. Chia sub-spec

Mỗi sub-spec có spec ngắn (chỉ ghi phần chi tiết thêm so với spec này), plan và nhánh riêng. Sau mỗi sub-spec, bản Windows vẫn phát hành được và mọi test Windows qua.

| # | Sub-spec | Nội dung | Spike đầu tiên | Xong khi |
|---|---|---|---|---|
| **L1** | Nền tảng đa nền tảng | `internal/platform`, `procs`, `secrets`, `firewall`, `netid`, `netwatch`, `startup.Manager`; `sysdns.Backend`; `app` phát event qua `Sink` và dùng `UI`; build tag cho mọi file Windows; embed DPI tách theo tag; `state.json` v4; ma trận CI + rào chắn | `ghostlined` build được với `CGO_ENABLED=0` khi `internal/app` không còn import `wails/application` | `GOOS=linux` build/vet/test xanh (backend Linux là stub trả `…_UNSUPPORTED`); Windows không đổi hành vi; rào chắn CI chạy |
| **L2** | Daemon + GUI client | `internal/rpc`, `tools/genrpc`, `internal/daemon`, `internal/sessionagent`, `cmd/ghostlined`, unit systemd tạm cho dev, trang lỗi GUI, `hello` | ID binding Wails với proxy ở package khác (mục 5.3) | GUI user điều khiển daemon root đủ mọi method/event; đóng GUI vẫn chạy; peer không được phép bị từ chối |
| **L3** | DNS + khôi phục | 3 backend DNS, `netwatch` Linux, flush, cổng 53, mục 7, `PrepareForSleep` | NM `GlobalDnsConfiguration` trên Fedora 41 và Ubuntu 24.04: đẩy đúng xuống resolved, sống qua kết nối lại | Tiêu chí 1, 2 |
| **L4** | DPI + tích hợp | Mục 8–13 | Cờ fwmark và hạ quyền của `nfqws2` bản pin; luật nft tối thiểu chạy được | Tiêu chí 3; proxy GNOME/KDE, chứng chỉ, firewall, SSID hoạt động |
| **L5** | Đóng gói + phát hành | Mục 16–17, tài liệu, checklist | SteamOS (mục 16.4) | Tiêu chí 4, 6, 7; phát hành v0.6.0 |

Thứ tự bắt buộc L1 → L2 → L3. L4 và L5 có thể chồng một phần (L5 bắt đầu đóng gói khi L3 xong).

## 22. Cấu trúc repo (thêm)

```
cmd/ghostlined/main.go
internal/platform/{platform.go,platform_windows.go,platform_linux.go}
internal/rpc/{proto.go,server.go,client.go,peer_linux.go,gen/…}
internal/daemon/
internal/sessionagent/
internal/firewall/{firewall.go,netsh_windows.go,firewalld_linux.go,ufw_linux.go,none_linux.go}
internal/secrets/{secrets.go,dpapi_windows.go,filekey_linux.go}
internal/netid/{netid.go,netid_windows.go,netid_linux.go}
internal/netwatch/{netwatch.go,netwatch_windows.go,netwatch_linux.go}
internal/procs/{procs.go,procs_windows.go,procs_linux.go}
internal/sysdns/{manager.go,backend.go,windows.go,nm_linux.go,resolved_linux.go,resolvconf_linux.go}
internal/sysproxy/{…,gnome_linux.go,kde_linux.go}
internal/certstore/{…,linux.go,firefoxpolicy_linux.go,nss_linux.go}
internal/dpi/zapret2/{filter.go,args_windows.go,args_linux.go,nftfilter_linux.go}
internal/startup/{startup.go,tasks_windows.go,systemd_linux.go}
tools/genrpc/
build/linux/{nfpm.yaml,ghostline.service,ghostline.desktop,ghostline.sysusers,install.sh,uninstall.sh,appimage/}
packaging/aur/PKGBUILD
docs/platforms.md
```

Tên file cụ thể có thể chỉnh trong từng sub-spec, nhưng phải giữ quy tắc: logic trong file không hậu tố, chỉ gọi hệ thống trong file `_<os>.go`.

## 23. Rủi ro và cách giảm thiểu

| Rủi ro | Giảm thiểu |
|---|---|
| NM ghi `GlobalDnsConfiguration` xuống đĩa; daemon chết mà không khôi phục thì máy mất DNS sau reboot | `ExecStopPost` luôn chạy; daemon bật lúc boot khôi phục trước mọi việc; test rút điện trong checklist |
| systemd-resolved vẫn gửi song song ra DNS nhà mạng (rò rỉ) | `SetLinkDefaultRoute(false)` cho mọi link, theo dõi và áp lại; kiểm tra rò rỉ hiện có bắt được |
| Wails v3 (beta) đổi API hoặc ID binding | Spike L2; proxy sinh tự động nên chỉ cần sửa trình sinh; ghim phiên bản Wails |
| SteamOS thiếu webkit/glibc, kernel thiếu NFQUEUE, hoặc cập nhật xoá `/etc` | Spike L5 trên máy thật; phương án AppImage kèm WebKit hoặc GUI Flatpak; DPI lùi về gợi ý proxy |
| Session-agent không vào được phiên user (đã đăng xuất, Wayland khác bus) | Hoãn và ghi `pendingUserRestores`, làm lại khi `UserNew`; lỗi `SESSION_UNAVAILABLE` rõ ràng |
| Firefox policy file đã có nội dung của tổ chức | Chỉ sửa mục của Ghostline; parse/ghi lại có test; không parse được thì không đụng và cảnh báo |
| Service root bị lợi dụng để leo thang đặc quyền | Socket phân quyền theo group; daemon chỉ chạy binary root sở hữu; session-agent dùng bản root; không có method nào chạy lệnh tuỳ ý hay ghi file theo đường dẫn từ client |
| Hash `nfqws2` sai do đọc qua công cụ tóm tắt | Lấy lại từ `sha256sum.txt` của release khi làm L4, `fetchdpi` đối chiếu tự động |
| Phân mảnh distro (KDE 5 vs 6, ufw vs firewalld, p11-kit vs không) | Backend theo danh sách với `Available()`; contract test; ma trận test thủ công |
| Bản Windows bị ảnh hưởng ngoài ý muốn | Test Windows hiện có, rào chắn phụ thuộc và kích thước exe trong CI |
| Khối lượng lớn cho một bản phát hành | Chia L1–L5, mỗi phần giữ Windows phát hành được; có thể dời ngày v0.6 mà không phải phát hành dở dang |
