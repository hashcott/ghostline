# Ghostline — Linux L4 (DPI + tích hợp hệ thống): Thiết kế

- **Ngày:** 2026-10-08
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Sub-spec L4 của [Hỗ trợ Linux](2026-10-07-ghostline-linux-design.md): DPI zapret2 (`nfqws2` + nftables), session-agent, proxy hệ thống GNOME/KDE, kho chứng chỉ, firewall, định danh mạng và SSID, phần giao diện cần cho các tính năng này. Sau L4, mọi tính năng của bản Windows chạy được trên Linux, trừ phần cài đặt và khởi động cùng hệ thống (L5).
- **Dựa trên:** [spec tổng Linux](2026-10-07-ghostline-linux-design.md) (§5.5, §8–§14), [L3](2026-10-08-ghostline-linux-l3-design.md) (đã merge vào `feat/linux`). Spec này chỉ ghi phần chi tiết thêm hoặc khác spec tổng; mọi thứ không nhắc lại ở đây giữ nguyên.

---

## 1. Mục tiêu

- Bật zapret2 trên Linux chặn được DPI như trên Windows, cùng chiến lược, cùng auto-tune, và không bao giờ làm mất mạng.
- Proxy hệ thống, chứng chỉ Fake SNI, chia sẻ trên LAN (firewall) và các tính năng theo mạng/SSID chạy trên Linux.
- Mọi thay đổi lên hệ thống đều được trả lại khi Disconnect, khi daemon crash và khi mất điện, kể cả thay đổi trong phiên của user.
- Interface dùng chung không còn khái niệm chỉ có trên Windows (WinDivert, WinINET). macOS sau này chỉ cần thêm backend.
- Windows không đổi hành vi.

### Tiêu chí thành công

1. **DPI (tiêu chí 3 của spec tổng):** bật zapret2 → bảng `inet ghostline` xuất hiện, site bị chặn mở được. `kill -9 nfqws2` → mạng vẫn chạy (gói đi thẳng). Dừng daemon (kể cả `kill -9`) → bảng biến mất.
2. **Kernel thiếu module:** `nfnetlink_queue`/`nft_queue` không nạp được → lỗi `DPI_KERNEL_UNSUPPORTED`, mạng không bị ảnh hưởng.
3. **Proxy hệ thống trên KDE (máy dev) và GNOME (fake + CI):** bật "dùng cho máy này" → app của desktop đi qua proxy Ghostline. Tắt, Disconnect hoặc `kill -9` daemon → cấu hình proxy của user trở về đúng như trước. App khác đổi proxy → cảnh báo `SYSPROXY_TAKEN_OVER` như Windows.
4. **Proxy khi chưa đăng nhập:** tự Connect lúc boot rồi user mới đăng nhập → proxy được đặt khi phiên đồ hoạ xuất hiện. Daemon chết khi user đã đăng xuất → proxy được khôi phục ở lần đăng nhập sau.
5. **Chứng chỉ:** Fake SNI bật → CA phiên có trong kho hệ thống (và Firefox, NSS của user khi cần). Tắt hoặc khôi phục → không còn file, policy hay mục NSS nào của Ghostline.
6. **Firewall:** chia sẻ trên LAN với ufw (máy dev) hoặc firewalld (fake + CI) → máy khác trong LAN vào được. Tắt hoặc `kill -9` rồi `--restore` → luật biến mất.
7. **SSID và khoá mạng:** đổi Wi-Fi → khoá mạng đổi, SSID hiện đúng; tính năng theo SSID hoạt động.
8. **Windows:** mọi test cũ pass, `state.json` v4 nạp thành v5 đúng, exe tăng không quá 2% so với v0.5.1, CI Windows xanh.
9. **Giao diện:** trên Linux không còn chuỗi nói về Windows, Defender, WinDivert; mọi chuỗi mới có đủ tiếng Việt và tiếng Anh.

## 2. Phạm vi

### Có

| Nhóm | Nội dung |
|---|---|
| DPI | `dpi.Interceptor`, bảng cổng chung, luật nftables qua `google/nftables`, kiểm tra module kernel, nhúng `nfqws2`, `fetchdpi` cho linux-x86_64, hạ quyền |
| Session-agent | `internal/session`: chạy tác vụ trong phiên của user, chọn user theo phiên đồ hoạ, hàng đợi việc chờ phiên |
| Proxy hệ thống | `sysproxy.Backend` trung lập, `model.ProxySnapshot`, backend Windows (chuyển từ code cũ), GNOME, KDE |
| Chứng chỉ | `certstore` Linux gộp ba đích: kho hệ thống, policy Firefox, NSS của user |
| Firewall | firewalld, ufw, không có firewall |
| Mạng | `netid` Linux: khoá mạng, `LiveAdapters`, SSID, Wi-Fi đã lưu |
| Giao diện | `platform` trong snapshot, `t(key)` với biến thể `.linux`, cờ khả năng cho trang DPI/Proxy |
| State | `state.json` v5, việc chờ phiên, thứ tự khôi phục |

### Không có (L5)

- "Khởi động cùng hệ thống" (bật unit systemd, `.desktop` tự mở GUI), thông báo khi GNOME không có tray.
- Trang `DAEMON_UNREACHABLE`, "cần cập nhật dịch vụ nền", `PKEXEC_NO_PASSWORD`.
- Mọi việc đóng gói, `sysusers.d`, SteamOS.

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Engine DPI | Giữ zapret2 `nfqws2`. Không dùng SpoofDPI: mã của nó nằm trong `internal/` nên không import được; phần tách ClientHello của nó Ghostline đã có (`tlsfrag`, `fakesni`, `autofrag`); nó yếu hơn zapret2 ở tầng gói và không dùng chung chiến lược với Windows. Engine tầng gói cho macOS quyết định trong spec macOS |
| Spike `nfqws2` (đã làm khi viết spec) | Bản pin v1.0.5.2, `binaries/linux-x86_64/nfqws2` SHA-256 `de1414b1e0f9a5659d438cddcb0c7e9099b4533bf5c8a3cf841c7bc7e5aa1473` (đối chiếu `sha256sum.txt` của release), 315 KB, liên kết tĩnh. Cờ: `--qnum=<n>`, `--fwmark=<int\|0xHEX>` (mặc định `0x40000000`), `--user=<name>` / `--uid=uid[:gid,…]` để hạ quyền. Phần "luật nft tối thiểu chạy được" cần root: task đầu của plan |
| `strategies.json` | **Khác spec tổng:** không thêm trường `platforms`. `strategies.Parse` của v0.5 dùng `DisallowUnknownFields`, thêm trường sẽ làm client cũ từ chối cả file. Chiến lược zapret2 giống nhau trên hai nền tảng |
| Chạy việc trong phiên user | `ghostlined --session-agent` do daemon gọi, mỗi tác vụ một tiến trình (tác vụ theo dõi thì chạy lâu) |
| User của phiên | **Khác spec tổng:** chủ phiên đồ hoạ đang hoạt động trên `seat0` (logind `Seat.ActiveSession` → `Session.User`, `Session.Desktop`), không phải user của kết nối RPC. `app.Service` không biết người gọi, và tự Connect lúc boot không có kết nối nào |
| Interface proxy | Trung lập như `sysdns.Backend`; snapshot trong `model` |
| Luật nft | `github.com/google/nftables` (netlink), không gọi lệnh `nft` |
| Firewall nhớ cổng | **Khác spec tổng:** backend Linux tự ghi `data/firewall-rules.json` (tên → zone, giao thức, cổng), vì `DeleteNamed(name)` không có cổng |
| Khoá mạng | **Khác spec tổng:** đọc `/proc/net/route` và `/proc/net/arp` thay cho netlink. Cùng kết quả với IPv4, test bằng file mẫu |
| Ẩn/hiện trong UI | Theo cờ khả năng backend báo, không theo tên OS. Chữ: bộ chuỗi ghi đè `en.linux.json`/`vi.linux.json` nạp đè lên bộ gốc khi `platform` là `linux`; component không đổi |
| Việc chờ phiên user | Hàng đợi trong `session` (`data/session-queue.json`), do backend proxy và NSS dùng; `app` và `CertsState` không đổi |
| `state.json` | v5 (chỉ đổi snapshot proxy), migrate từ v4 |

## 4. Interface

### 4.1 DPI

```go
// dpi: thay Services (chỉ có nghĩa với WinDivert).
type Interceptor interface {
	Prepare() error          // trước khi chạy engine: Windows gỡ service WinDivert sót lại; Linux kiểm module, cài bảng nft
	Ready(pid int) bool      // engine đã chặn được gói: Windows driver chạy; Linux queue có người giữ
	Cleanup() error          // sau khi dừng và khi khôi phục: Windows gỡ service; Linux xoá bảng (không có bảng thì không lỗi)
	Info() InterceptorInfo   // cho UI: mô tả cơ chế, cờ AVExclusions
}

type InterceptorInfo struct {
	Mechanism    string // "WinDivert" | "nftables inet ghostline, queue 200"
	AVExclusions bool   // phần loại trừ antivirus có nghĩa không
}
```

- `Manager` gọi `Prepare` → kiểm hash → chạy engine → đợi tối đa 2 giây đến khi `Ready` → báo đã chạy. Stop: kill engine → `Cleanup`. Logic WinDivert hiện có (`driverService`, gỡ service) chuyển nguyên vào `interceptor_windows.go`.
- `platform.Deps`: `DPIServices` thay bằng `DPIInterceptor dpi.Interceptor`.

### 4.2 Bảng cổng chung (`zapret2/filter.go`)

```go
type Rule struct {
	Proto     string // "tcp" | "udp"
	Ports     []int
	OutPackets int   // số gói đầu chiều đi cần bắt (ct original packets)
	InPackets  int   // số gói đầu chiều về (ct reply packets), cho autohostlist
	QUIC       bool  // Windows chỉ bắt khi chiến lược có profile QUIC
}
var Filter = []Rule{{"tcp", []int{80, 443}, 20, 10, false}, {"udp", []int{443}, 5, 3, true}}
```

- Windows: `--wf-tcp-out`, `--wf-udp-out` sinh từ `Filter`. Test giữ đúng chuỗi tham số hiện nay.
- Linux: luật nft sinh từ `Filter` (§5.2).

### 4.3 Proxy

```go
// model
type ProxySnapshot struct {
	Backend string           `json:"backend"` // "windows" | "gnome" | "kde"
	UID     int              `json:"uid,omitempty"`
	Windows *WinINETProxy    `json:"windows,omitempty"` // 4 trường hiện có
	GNOME   *GNOMEProxy      `json:"gnome,omitempty"`   // mode, autoconfig-url, ignore-hosts, http/https/socks host+port, use-same-proxy
	KDE     *KDEProxy        `json:"kde,omitempty"`     // các key [Proxy Settings] đã có (key không có thì ghi là không có)
}

// sysproxy
type Backend interface {
	Snapshot() (Snapshot, error)
	Existing(s Snapshot) (server, pac string, has bool)
	Apply(addr string) error
	IsOurs(addr string) (bool, error)
	RestoreIfOurs(addr string, s Snapshot) (bool, error)
	Watch(onChange func()) (stop func(), err error)
	Info() Info // Desktop ("GNOME", "KDE", "" = không hỗ trợ) cho UI
}
```

- Các method đúng như `app` đang dùng; chỉ đổi kiểu snapshot và gom `Watch` vào backend (như L3 làm với DNS).
- Danh sách bỏ qua proxy (bypass) là một danh sách trung lập dùng chung (`sysproxy.DefaultBypass`); mỗi backend chuyển sang định dạng của nó bằng các hàm `BypassWinINET`, `BypassGNOME`, `BypassKDE`, có test bảng. Chuỗi WinINET Windows đang ghi không đổi.

### 4.4 Session-agent (`internal/session`)

```go
type User struct{ UID, GID int; Groups []int; Home, Desktop string }

type Sessions interface {
	Active() (User, bool)                         // chủ phiên đồ hoạ đang hoạt động trên seat0
	Run(u User, task string, in, out any) error   // tác vụ một lần, timeout 10 giây
	Stream(u User, task string, in any, onLine func([]byte)) (stop func(), err error) // tác vụ chạy lâu
	WatchNew(onNew func(User)) (stop func(), err error) // logind SessionNew
}

// Queue giữ việc phải làm trong phiên của một user chưa đăng nhập.
type Queue interface {
	Add(uid int, task string, args any) error
	Drain(u User, run func(task string, args json.RawMessage) error) error // chạy và xoá việc của u.UID
}
```

- Agent là chính binary của daemon (`/proc/self/exe`), chạy `--session-agent`, `SysProcAttr.Credential` = uid/gid/groups của user. Môi trường: `HOME`, `USER`, `XDG_RUNTIME_DIR=/run/user/<uid>`, `DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/<uid>/bus`, `XDG_CURRENT_DESKTOP`, `PATH` chuẩn. Không truyền biến nào khác của daemon.
- Giao thức: một dòng JSON vào stdin (`{"task":…, "args":…}`), một dòng JSON ra stdout (`{"result":…}` hoặc `{"error":{"code":…,"message":…}}`). Tác vụ chạy lâu in một dòng mỗi sự kiện.
- Tác vụ: `proxy.snapshot`, `proxy.apply`, `proxy.isOurs`, `proxy.restore`, `proxy.watch`, `nss.install`, `nss.remove`, `nss.list`. Agent chỉ nhận tác vụ trong danh sách này.
- Windows không có session-agent: proxy WinINET chạy trong tiến trình như hiện nay.

### 4.5 Chứng chỉ, firewall, mạng

- `certstore.Store` giữ nguyên. Linux: `certstore.NewLinux(targets…)` gộp các `Target` (cùng ba method). Lỗi ở đích không bắt buộc → `Install` trả `ErrPartial` (bọc tên đích); đích bắt buộc lỗi → lỗi như hiện nay.
- `firewall.Manager` giữ nguyên. Linux: `firewall.DetectLinux(dataDir)` chọn firewalld → ufw → không có.
- `netid.Source` giữ nguyên. Linux: `netid.NewLinux(nm)`.

### 4.6 Giao diện

- `platform.Deps.Name` (`"windows"`, `"linux"`) → `Snapshot.Platform`.
- Cờ khả năng: `DPIInfo.AVExclusions`, `DPIInfo.Mechanism`, `ProxyInfo.Desktop`.

## 5. DPI

### 5.1 File và quyền

- `assets/zapret2/embed_linux.go` nhúng `nfqws2` và 3 file Lua. `pins.go` giữ hash Lua (dùng chung); hash file chạy tách sang `pins_windows.go` / `pins_linux.go`. `Engine.Files()` trả hash đúng nền tảng.
- `tools/fetchdpi` tải thêm `binaries/linux-x86_64/nfqws2` và đối chiếu với `sha256sum.txt`.
- Thư mục engine trên Linux: `/var/lib/ghostline/bin/zapret2` (`platform` đặt `Paths.BinDir`), quyền `0750` root:group của `nobody`. `nfqws2` `0755` root, Lua và danh sách `0644`, file autohostlist thuộc `nobody`. `data/` vẫn `0700`.
- Chế độ pre-NAT của zapret2 (`POSTNAT=0`): Ghostline chỉ xử lý gói của chính máy, không làm router. Số gói đầu theo `config.default` bản pin: TCP ra 20, TCP vào 10, UDP ra 5, UDP vào 3. Bảng luôn có cả UDP 443; chiến lược không có profile QUIC thì `nfqws2` để gói đi nguyên.
- Tham số Linux: `--qnum=200 --fwmark=0x40000000 --user=nobody`, sau đó `--lua-init` và các profile như Windows. Danh sách truyền thẳng đường dẫn (`HotReloadsLists` vẫn `true`).

### 5.2 Luật nftables

Bảng `inet ghostline`:

- Chain `post`: hook `postrouting`, priority 99 (pre-NAT, theo `nft.sh` bản pin).
  - Bỏ qua: `meta mark and 0x40000000 != 0`, `oifname "lo"`, đích loopback, LAN/private (RFC1918, link-local, ULA, CGNAT).
  - Mỗi `Rule` ra của `Filter` → `ct original packets 1-N` + cổng → `queue num 200 bypass`.
  - Luật chống lỗi TTL của zapret2 (đánh dấu `ct mark` cho gói do `nfqws2` sinh).
- Chain `pre`: hook `prerouting`, priority -99; `Rule` vào của `Filter` (`ct reply packets 1-N`) → `queue num 200 bypass`; bỏ gói ICMP time-exceeded của kết nối đã đánh dấu.
- `bypass` bảo đảm fail-open. Gỡ = xoá cả bảng; không đụng bảng khác.

### 5.3 Kernel và vòng đời

- `Prepare`: kiểm `nfnetlink_queue`, `nft_queue`, `nf_conntrack` qua `/sys/module`; thiếu thì `modprobe`; vẫn thiếu → `DPI_KERNEL_UNSUPPORTED` (UI gợi ý bật proxy). Sau đó cài bảng.
- `Ready`: `/proc/net/netfilter/nfnetlink_queue` có dòng queue 200 với portid khác 0.
- Runner Linux: `exec` với `Pdeathsig=SIGKILL`, stdout/stderr vào log của daemon.
- `Cleanup` chạy cả khi khôi phục và khi daemon khởi động: không có bảng thì không lỗi.

## 6. Session-agent và proxy hệ thống

### 6.1 Chọn user

- `Active()`: logind `Seat("seat0").ActiveSession` → `Session.User`, `Type` (`x11`/`wayland`), `Desktop`, `Remote=false`. Không có phiên đồ hoạ → không có user.
- uid ghi vào snapshot proxy và `Certs.UserUID`. Khôi phục luôn nhắm đúng uid đã ghi, kể cả khi phiên hoạt động đã đổi sang user khác.

### 6.2 Việc chờ phiên

- **Khôi phục** khi user không có phiên (`/run/user/<uid>` không có): backend thêm việc vào `session.Queue` (`data/session-queue.json`, ghi tạm rồi `rename`) và coi như đã xong; `state.json` được dọn như thường.
- **Đặt** khi chưa có phiên đồ hoạ: `Snapshot` trả `sysproxy.ErrNoSession`; `app` bỏ qua proxy hệ thống cho lần này (Connect vẫn thành công), hiện cảnh báo `SESSION_PENDING` và gọi `ReapplyProxy` khi có phiên mới.
- `core.Start` đăng ký `Sessions.WatchNew`; phiên mới → `Queue.Drain` cho uid đó, rồi `ReapplyProxy` nếu đang có `SESSION_PENDING`. Daemon khởi động mà đã có phiên → `Drain` ngay.

### 6.3 Backend

| Desktop (`Session.Desktop` / `XDG_CURRENT_DESKTOP`) | Backend | Đặt | Theo dõi |
|---|---|---|---|
| GNOME, Cinnamon, Budgie, Unity, ubuntu:GNOME | `gnome` | `gsettings set org.gnome.system.proxy mode manual`, `…http host/port`, `…https host/port`, `ignore-hosts`; snapshot lưu mọi key | `gsettings monitor org.gnome.system.proxy` (tác vụ `proxy.watch`) |
| KDE | `kde` | `kwriteconfig6` (không có thì `kwriteconfig5`) `--file kioslaverc --group "Proxy Settings"`: `ProxyType=1`, `httpProxy`, `httpsProxy`, `NoProxyFor`; rồi phát `org.kde.KIO.Scheduler.reparseSlaveConfiguration` | inotify trên `~/.config/kioslaverc` |
| Khác | không | `PROXY_DESKTOP_UNSUPPORTED` nếu bật "dùng cho máy này"; trang Proxy hiện hướng dẫn tay | — |

- Restore ghi lại đúng từng key; key trước đó không có thì xoá (`gsettings reset`, `kwriteconfig --delete`).
- Windows: `sysproxy.NewWinINET()` chứa nguyên code hiện có.

## 7. Chứng chỉ

| Đích | Bắt buộc | Ai làm | Cách |
|---|---|---|---|
| Kho hệ thống | Có | daemon | Chọn theo công cụ: `update-ca-certificates` → `/usr/local/share/ca-certificates/ghostline-<thumb>.crt`; `update-ca-trust` → `/etc/pki/ca-trust/source/anchors/` (Fedora) hoặc `/etc/ca-certificates/trust-source/anchors/` (Arch, SteamOS). `List` chỉ đọc file `ghostline-*` |
| Firefox | Không | daemon | Khi có Firefox (`/usr/lib*/firefox*`, `/snap/firefox`): thêm/bớt đường dẫn file CA trong `Certificates.Install` của `/etc/firefox/policies/policies.json`, giữ mọi policy khác, ghi tạm rồi `rename`. File do Ghostline tạo mà chỉ còn mục của Ghostline → xoá file. Firefox Flatpak không hỗ trợ |
| NSS của user | Không | session-agent | Chỉ khi NSS không đọc kho hệ thống qua p11-kit (`libnssckbi.so` không trỏ tới `p11-kit-trust.so`) và `~/.pki/nssdb` tồn tại: `certutil -d sql:$HOME/.pki/nssdb -A/-D`. Không có `certutil` → `CERT_NSS_TOOL_MISSING` |

- `ErrPartial` → `app` hiện cảnh báo `CERT_PARTIAL` (tham số: đích) nhưng Fake SNI vẫn bật.
- NSS ghi uid đã cài vào `data/nss-users.json` (uid → thumbprint). Gỡ khi user không có phiên → `session.Queue`.

## 8. Firewall

| Backend | Khi nào | Thêm | Gỡ |
|---|---|---|---|
| firewalld | `org.fedoraproject.FirewallD1` có chủ | `addPort` runtime trong zone của interface LAN (`getZoneOfInterface`, rỗng → zone mặc định) | `removePort` |
| ufw | `ufw status` là `active` | `ufw allow <port>/<proto> comment 'ghostline: <tên>'` | `ufw delete allow <port>/<proto>` |
| không có | còn lại | không làm gì | không làm gì |

- `data/firewall-rules.json` ghi tên → zone, giao thức, cổng ngay khi thêm (ghi tạm rồi `rename`), xoá mục khi gỡ. `DeleteNamed` đọc file này; không có mục → không lỗi.
- `RuleBlockPublic` không làm gì trên Linux. `IsPublicNetwork`: firewalld zone `public`/`external`/`block`/`drop` → `true`; còn lại `false`.
- Không có firewall mà có chain `input` policy `drop` ở bảng nft khác → cảnh báo `FIREWALL_UNKNOWN`.

## 9. Định danh mạng và SSID

- `NetworkKey`: route mặc định từ `/proc/net/route` → gateway và interface; MAC từ `/proc/net/arp`; chưa có thì gửi một gói UDP tới gateway (cổng 9) rồi đọc lại sau 200 ms; vẫn không có thì MAC của chính interface. Kết quả qua `scanner.NetworkKey` như Windows.
- `LiveAdapters`: interface đang bật có route mặc định, kèm địa chỉ từ `net.Interfaces`.
- `CurrentSSID`: NM `Devices` → thiết bị Wi-Fi đang kết nối → `ActiveAccessPoint.Ssid`. `WifiNames`: `Settings.ListConnections` → `GetSettings` → mục `802-11-wireless`. Không có NM → `ErrUnsupported` (UI hiện "không xác định").

## 10. Giao diện

- `Snapshot.Platform`; `frontend/src/i18n` nạp `en.linux.json`/`vi.linux.json` đè lên bộ gốc (`addResourceBundle(…, deep, overwrite)`) khi `platform` là `linux`. Test parity: hai file Linux có cùng khoá, và mọi khoá của chúng có trong bộ gốc.
- Bản ghi đè cho các chuỗi nói về Windows, Defender, WinDivert, "chứng chỉ trong Windows", proxy Windows, svchost/Hotspot/WSL, Public/Private.
- Trang DPI: danh sách engine theo backend (Linux chỉ zapret2); phần loại trừ antivirus chỉ khi `AVExclusions`; hiện `Mechanism`.
- Trang Proxy: hiện `Desktop`; rỗng → hướng dẫn đặt tay.
- Mã lỗi/cảnh báo mới có chuỗi vi/en: `DPI_KERNEL_UNSUPPORTED`, `PROXY_DESKTOP_UNSUPPORTED`, `SESSION_PENDING`, `CERT_PARTIAL`, `CERT_NSS_TOOL_MISSING`, `FIREWALL_UNKNOWN`.

## 11. State và khôi phục

- `state.json` v5:
  - `sysProxy.snapshot` → `model.ProxySnapshot`; v4 (WinINET) migrate vào nhánh `windows`.
  - Việc chờ phiên nằm trong `session.Queue`, uid của NSS trong `data/nss-users.json`; không thêm trường nào khác.
- Thứ tự khôi phục (`--restore`, daemon khởi động, `ExecStopPost`) giữ thứ tự của `watchdog` hiện nay, thêm backend Linux: giết `nfqws2` còn sống → `Interceptor.Cleanup` → proxy (qua agent, hoặc `session.Queue`) → chứng chỉ (kho hệ thống, Firefox ngay; NSS có thể vào `session.Queue`) → firewall (`firewall-rules.json`) → DNS (L3).

## 12. Kiểm thử

| Mức | Nội dung |
|---|---|
| Unit (CI cả hai OS) | Sinh luật nft từ `Filter` (so đáp án); chuỗi `--wf-*` của Windows không đổi; `Manager` với fake `Interceptor`; backend GNOME/KDE với fake chạy lệnh; giao thức agent (test binary tự làm agent); `Pending` và `WatchNew`; ba đích chứng chỉ trên thư mục tạm, gộp/xoá `policies.json`, `ErrPartial`; firewalld/ufw với fake, `firewall-rules.json` qua crash → restore; `/proc/net/route` và `/proc/net/arp` mẫu; migrate v4 → v5; chuyển đổi bypass |
| Integration (CI Linux, root, tag `integration_root`) | Cài/gỡ bảng nft; chạy `nfqws2` thật, `kill -9`, mạng vẫn đi (fail-open); firewall "không có"; kho chứng chỉ của runner (Ubuntu) cài → gỡ |
| Thủ công (máy dev: Arch, KDE Wayland, ufw, Firefox) | Script root như L3: tiêu chí 1, 3, 5, 6, 7; so trạng thái trước/sau (`nft list tables`, `kioslaverc`, anchors, `policies.json`, `ufw status`) phải giống hệt |
| Spike (task đầu plan) | Luật nft tối thiểu + `nfqws2 --qnum=200 --fwmark … --user=nobody` chạy thật trên máy dev: gói vào queue, `kill -9` không chặn mạng |

## 13. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Thứ tự hook/priority làm gói không vào queue hoặc lặp | Theo `nft.sh` bản pin; spike ở task đầu; fwmark loại gói của `nfqws2` |
| Luật nft chặn mạng | Luôn `queue … bypass`; test fail-open trên CI và máy dev |
| Firefox giữ CA đã nhập qua policy sau khi gỡ | Kiểm thật trên máy dev; CA phiên bị giới hạn tên miền và ngắn hạn; nếu đúng thì ghi vào tài liệu |
| Session-agent chạy với quyền user nhưng do root gọi | Chỉ binary root, danh sách tác vụ cố định, môi trường tối thiểu, không đọc file do user chỉ định |
| Proxy bị đặt cho sai user khi đổi phiên | uid ghi trong snapshot; khôi phục theo uid đã ghi |
| ufw/firewalld đổi cú pháp | Fake theo đầu ra thật; thử thật ufw trên máy dev; firewalld trên CI |
| Đổi interface proxy/DPI làm sai Windows | Code Windows chuyển nguyên khối; test cũ giữ nguyên; CI Windows; `sizecheck` |
