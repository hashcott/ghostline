# Ghostline — Linux L5 (đóng gói + phát hành): Thiết kế

- **Ngày:** 2026-10-09
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Sub-spec L5 của [Hỗ trợ Linux](2026-10-07-ghostline-linux-design.md): unit systemd chính thức, cài dịch vụ cho bản AppImage/tar.gz, khởi động cùng hệ thống, gói `.deb`/`.rpm`/AppImage/`tar.gz`/PKGBUILD, CI và quy trình phát hành, tài liệu, checklist. Sau L5, v0.6.0 đủ điều kiện phát hành.
- **Dựa trên:** [spec tổng Linux](2026-10-07-ghostline-linux-design.md) (§5.6, §14, §16–§17, §19.3), [L4](2026-10-08-ghostline-linux-l4-design.md) (đã merge vào `feat/linux`). Spec này chỉ ghi phần chi tiết thêm hoặc khác spec tổng; mọi thứ không nhắc lại ở đây giữ nguyên.

---

## 1. Mục tiêu

- Người dùng Ubuntu/Debian, Fedora, Arch và các distro khác cài được Ghostline bằng một file tải về, và gỡ sạch.
- Bảo vệ chạy nền ngay sau khi cài, sống qua reboot, không cần mở GUI.
- Bản AppImage và `tar.gz` tự cài được dịch vụ nền từ GUI (qua `pkexec`) hoặc bằng một lệnh.
- Một lần push tag tạo đủ file Windows và Linux, một `SHA256SUMS` chung.
- Windows không đổi hành vi, exe không phình.

### Tiêu chí thành công

1. **Cài bằng gói (tiêu chí 4 spec tổng):** `apt install ./ghostline_*.deb` (hoặc `dnf install ./…rpm`) → dịch vụ `ghostline` chạy, GUI mở từ menu điều khiển được. Đóng GUI → bảo vệ vẫn chạy.
2. **Gỡ sạch (tiêu chí 6):** gỡ gói khi đang kết nối → DNS, proxy hệ thống, chứng chỉ (kể cả trong profile Firefox), luật firewall, bảng nft về như trước khi cài; không còn file nào ngoài `/var/lib/ghostline` và `/var/log/ghostline`; `purge` (deb) hoặc `--purge` xoá nốt hai thư mục đó. CI kiểm tự động cho deb và `tar.gz`.
3. **AppImage/tar.gz:** máy chưa có dịch vụ → GUI hiện "Cài dịch vụ"; bấm, nhập mật khẩu → dịch vụ chạy, GUI kết nối được. Chạy lại `--install-system` từ bản mới hơn = cập nhật.
4. **Khởi động cùng hệ thống:** bật trong Cài đặt → không còn lỗi unsupported; reboot → tự Connect (nếu bật "tự kết nối") và GUI mở xuống khay khi đăng nhập.
5. **Windows (tiêu chí 7):** exe tăng không quá 2% so với v0.5.1 (size guard trong release), mọi test Windows qua, nội dung gói Windows không đổi.
6. **CI:** `linux:package` chạy xanh trên `ubuntu-24.04`; `makepkg` với PKGBUILD chạy xanh trong container Arch.

## 2. Phạm vi

### Có

| Nhóm | Nội dung |
|---|---|
| Hệ thống | Unit mẫu một nguồn, hardening, `sysusers.d`, `ghostlined --install-system`/`--uninstall-system [--purge]`, `startup` trên Linux |
| GUI | Nhận biết cách cài, nút `pkexec` trên trang `DAEMON_UNREACHABLE` và trang cần cập nhật dịch vụ, dòng nhắc `passwd` trên SteamOS, file autostart của user |
| Gói | `.deb`, `.rpm` (nfpm), AppImage (appimagetool, không kèm GTK/WebKit), `tar.gz` + `install.sh`/`uninstall.sh`, PKGBUILD `ghostline-bin` |
| CI/release | Đóng gói thử, kiểm cài/gỡ deb và tar.gz trên runner, `makepkg` trong container Arch, job `linux` + `publish` trong `release.yml` |
| Tài liệu | README/README.vi, hai bản hướng dẫn, `platforms.md`, `release-checklist.md` |

### Không có

- Spike SteamOS trên máy thật (không có Steam Deck). SteamOS ghi "chưa kiểm chứng"; spike chuyển vào checklist (mục 9).
- AppImage gói kèm GTK/WebKit, Flatpak cho GUI: chỉ làm trong sub-spec riêng nếu spike SteamOS cho thấy cần.
- Đẩy lên AUR tự động (chưa có tài khoản AUR): release đính kèm PKGBUILD đã điền version và SHA-256, người bảo trì tự đẩy.
- Ký GPG, kho apt/dnf riêng, arm64 (như spec tổng).
- Tạo và push tag `v0.6.0`: làm sau L5, khi người bảo trì đồng ý.

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Cách đóng gói | Taskfile `build/linux` + nfpm (deb, rpm) + appimagetool (AppImage) + script (`tar.gz`). Không dùng `wails3 package` cho Linux: AppImage của Wails gói kèm GTK/WebKit (80–150 MB), ép `GDK_BACKEND=x11`, tải linuxdeploy bản `continuous` không pin |
| Kích thước | GUI ≈ 23,7 MB, `ghostlined` ≈ 19,8 MB sau strip; mỗi file phát hành ≈ 13–17 MB. Giữ hai binary (daemon static không CGO, không Wails; GUI cần CGO/GTK) |
| AppImage | Dùng GTK4/WebKitGTK 6.0 của máy; `AppRun` báo rõ khi thiếu |
| SteamOS | Chưa kiểm chứng; không chặn L5 |
| AUR | Chỉ PKGBUILD + kiểm `makepkg` trong CI; release đính kèm PKGBUILD đã điền |
| Unit | Một file mẫu `internal/sysinstall/ghostline.service.tmpl` (chỗ trống `@DAEMON@`), embed trong package cài đặt cho `--install-system`, `sed` cho gói |
| `pkexec` | Chỉ GUI gọi; daemon không bao giờ gọi |
| Autostart GUI | GUI tự đồng bộ `~/.config/autostart/ghostline.desktop` theo `StartWithWindows`, không thêm method RPC |
| Frontend trong release | Mỗi job tự build (vài giây), không chia sẻ artifact (khác §17 spec tổng, cho workflow đơn giản) |

## 4. Unit systemd và group

- `internal/sysinstall/ghostline.service.tmpl` thay `build/linux/ghostline.service` (cùng package với code cài, để `go:embed` đọc được):
  - `ExecStart=@DAEMON@ --daemon`, `ExecStopPost=@DAEMON@ --restore`, `Restart=on-failure`, `RestartSec=2`.
  - `StateDirectory=ghostline` (`0755`), `LogsDirectory=ghostline`, `RuntimeDirectory=ghostline` (`0755`).
  - `NoNewPrivileges=yes`: không chặn `setuid()` của session-agent và của `nfqws2 --user=nobody`, chỉ chặn exec binary setuid.
  - `CapabilityBoundingSet=` danh sách ở §16.2 spec tổng. Không dùng `ProtectSystem`/`ProtectHome`: daemon ghi `/etc` (resolv.conf, anchors, policy Firefox), session-agent ghi trong home.
  - Danh sách capability cuối cùng chốt bằng lần chạy root ở mục 10 (bộ test root + kịch bản Connect đủ tính năng với unit đã hardening). Thiếu capability nào thì thêm, kèm dòng ghi lý do trong spec này.
- `@DAEMON@`: `/usr/lib/ghostline/ghostlined` cho gói; `/var/lib/ghostline/bin/ghostlined` cho bản tự cài.
- `build/linux/sysusers.d/ghostline.conf`: `g ghostline -`.

## 5. `ghostlined --install-system` / `--uninstall-system`

Chỉ có trên Linux (file `_linux.go`; trên Windows lệnh không tồn tại). Cần root; không phải root → thoát mã 1, in lỗi ra stderr.

### 5.1 Cài

1. Có `/usr/lib/systemd/system/ghostline.service` (cài bằng gói) → từ chối: "Ghostline đã được cài bằng gói; dùng trình quản lý gói để cập nhật".
2. Chép chính binary đang chạy (`os.Executable`) vào `/var/lib/ghostline/bin/ghostlined.new` (`0755`, root), đọc lại và so SHA-256 với file nguồn, `rename` thành `ghostlined`. `/var/lib/ghostline` và `bin/` tạo `0755` (nfqws2 chạy bằng `nobody` cần đi qua; khớp `prepareEngineDir`).
3. Ghi unit (từ mẫu embed, `@DAEMON@` = `/var/lib/ghostline/bin/ghostlined`) vào `/etc/systemd/system/ghostline.service` qua file tạm + rename.
4. Group `ghostline` chưa có → `groupadd --system ghostline`.
5. `systemctl daemon-reload`; dịch vụ đang chạy → `restart`, không thì `enable --now`.

Chạy lại lệnh với binary mới hơn = cập nhật. Mọi bước idempotent.

### 5.2 Gỡ

1. `--remove-certs` (cùng code như prerm: khôi phục DNS/proxy, gỡ CA khỏi mọi đích, xoá bảng nft, gỡ luật firewall).
2. `systemctl disable --now ghostline`; xoá unit trong `/etc/systemd/system`; `daemon-reload`.
3. Xoá `/var/lib/ghostline/bin`.
4. `--purge`: xoá thêm `/var/lib/ghostline` và `/var/log/ghostline`.

Group `ghostline` giữ lại (user có thể đã được thêm vào; xoá group là việc của admin).

### 5.3 Cấu trúc code

- Logic trong một package dùng chung (`internal/sysinstall`), các thao tác hệ thống qua interface nhỏ (`systemctl`, `groupadd`, đường dẫn gốc) để test trên thư mục root giả. `cmd/ghostlined` chỉ phân tích cờ và gọi.
- `ghostlined` vẫn không phụ thuộc Wails; depcheck giữ nguyên.

## 6. GUI

### 6.1 Nhận biết cách cài (chỉ Linux)

| Điều kiện | Cách cài | Nút trên trang `DAEMON_UNREACHABLE` |
|---|---|---|
| Có `$APPIMAGE` | `appimage` | Unit chưa có: "Cài dịch vụ". Có unit: "Khởi động dịch vụ" |
| Có `/usr/lib/systemd/system/ghostline.service` | `package` | "Khởi động dịch vụ" |
| Còn lại | `tarball` | Như `appimage` |

- "Khởi động dịch vụ": `pkexec systemctl enable --now ghostline.service`.
- "Cài dịch vụ" (AppImage): GUI chép `$APPDIR/usr/lib/ghostline/ghostlined` sang `$XDG_RUNTIME_DIR/ghostline-install/ghostlined` (root không đọc được điểm mount FUSE của AppImage), rồi `pkexec <bản chép> --install-system`. Tarball: `pkexec <thư mục của GUI>/../lib/ghostline/ghostlined --install-system` nếu có, không thì hướng dẫn chạy `sudo ./install.sh`.
- Sau khi `pkexec` xong, GUI thử kết nối lại daemon (tối đa ~10 giây).
- `pkexec` thất bại (huỷ, sai mật khẩu, không có tác nhân polkit) → thông báo lỗi kèm lệnh tương đương để chạy trong terminal.

### 6.2 Cần cập nhật dịch vụ

Khác `protocol` (trang `DAEMON_PROTOCOL_MISMATCH` đã có):
- `appimage`/`tarball`: nút "Cập nhật dịch vụ" (= "Cài dịch vụ").
- `package`: hướng dẫn cập nhật gói (không có nút).

### 6.3 SteamOS

`/etc/os-release` có `ID=steamos` → dưới mọi nút `pkexec` luôn có dòng nhắc: user `deck` cần đặt mật khẩu bằng `passwd` trong Konsole trước. Không cố đoán lý do `pkexec` thất bại.

### 6.4 Khởi động cùng hệ thống

- Daemon: `startup.ServiceManaged.SetAutostart` trả `nil` (unit đã được bật lúc cài; tự Connect lúc boot dùng `StartWithWindows` + `AutoConnect` như hiện có).
- GUI: nhận snapshot/event `settings` → `StartWithWindows` bật thì ghi `~/.config/autostart/ghostline.desktop` (`Exec=<đường dẫn GUI> --autostart`; AppImage dùng `$APPIMAGE`), tắt thì xoá. Chỉ ghi khi nội dung khác. Mỗi user có GUI riêng tự đồng bộ file của mình.
- Chuỗi `.linux` cho mục này trong Cài đặt: "Khởi động cùng hệ thống" thay "Khởi động cùng Windows" (nếu chưa có).

### 6.5 Dữ liệu cho frontend

Theo cách đã có cho `SetMode`/`GetSnapshot`: method mới khai báo trên `app.Service` (để frontend có binding trên cả hai nền tảng), `tools/genrpc` bỏ qua chúng, package `internal/rpc/client` viết tay để chạy ngay trong GUI, không qua daemon (daemon có thể đang không chạy).

- `ServiceInstall() InstallInfo`: `{kind: "appimage"|"package"|"tarball"|"", unit: bool, steamos: bool}`. Trên Windows (và trong daemon) trả giá trị rỗng: frontend không hiện nút.
- `InstallService() error`, `StartService() error`: §6.1; trên Windows trả lỗi unsupported (không bao giờ được gọi vì nút không hiện).
- Phần nhận biết và gọi `pkexec` nằm trong file `_linux.go` của package client, logic thuần (bảng ở §6.1) tách ra để test.

## 7. Gói

### 7.1 Build (`build/linux/Taskfile.yml`)

- `linux:build`: frontend → GUI (`CGO_ENABLED=1`, `-trimpath -ldflags "-s -w -X …brand.Version=<ver>"`) → `ghostlined` (`CGO_ENABLED=0`, cùng ldflags).
- `linux:package`: tạo đủ bốn định dạng vào `bin/`.
- Công cụ pin: nfpm qua `go run github.com/goreleaser/nfpm/v2/cmd/nfpm@<version>`; appimagetool tải bản phát hành cố định và kiểm SHA-256 ghi trong Taskfile.

### 7.2 File chung (`build/linux/`)

- `ghostline.desktop`, icon hicolor (từ `build/appicon.png`, 256 và 512), `io.github.hashcott.ghostline.metainfo.xml`, `sysusers.d/ghostline.conf`, `LICENSE`, `NOTICE`.

### 7.3 `.deb` và `.rpm` (một `nfpm.yaml`)

| | |
|---|---|
| File | `/usr/bin/ghostline`, `/usr/lib/ghostline/ghostlined`, `/usr/lib/systemd/system/ghostline.service`, `/usr/lib/sysusers.d/ghostline.conf`, `/usr/share/applications/ghostline.desktop`, icon, metainfo, license |
| Phụ thuộc deb | `libgtk-4-1`, `libwebkitgtk-6.0-4`; khuyên `libnss3-tools` |
| Phụ thuộc rpm | `gtk4`, `webkitgtk6.0`; khuyên `nss-tools` |
| postinst | `systemd-sysusers ghostline.conf`; `daemon-reload`; cài mới → `enable --now`; nâng cấp → `try-restart` |
| prerm | Chỉ khi gỡ hẳn (deb `remove`, rpm `$1 = 0`): `ghostlined --remove-certs`, `disable --now` |
| postrm | deb `purge`: xoá `/var/lib/ghostline`, `/var/log/ghostline` |

### 7.4 `tar.gz`

`ghostline-<ver>-linux-amd64/`: hai binary, `.desktop`, icon, metainfo, license, `install.sh`, `uninstall.sh` (unit và group do `--install-system` tạo).
- `install.sh` (root): GUI → `/usr/local/bin/ghostline`, `ghostlined` → `/usr/local/lib/ghostline/ghostlined` (để GUI tìm được, §6.1), `.desktop`/icon/metainfo → `/usr/local/share`, rồi `ghostlined --install-system`.
- `uninstall.sh [--purge]` (root): `/var/lib/ghostline/bin/ghostlined --uninstall-system [--purge]`, xoá các file trong `/usr/local`.

### 7.5 AppImage

- AppDir: `AppRun`, `usr/bin/ghostline`, `usr/lib/ghostline/ghostlined`, `.desktop`, icon. Không kèm GTK/WebKit.
- `AppRun`: `ldconfig -p` không thấy `libwebkitgtk-6.0.so.4` → in (và `notify-send` nếu có) thông báo cần cài gói nào cho Ubuntu/Fedora/Arch, thoát mã 1. Có → `exec usr/bin/ghostline "$@"`.

### 7.6 PKGBUILD (`build/linux/aur/`)

- `ghostline-bin`: `source` = `tar.gz` từ release + `sha256sums`; cài vào cùng đường dẫn như §7.3; `depends=(gtk4 webkitgtk-6.0)`, `optdepends=('nss: Fake SNI trong Chrome')`; `install=ghostline.install` làm như postinst/prerm.
- `@VERSION@`/`@SHA256@` điền khi phát hành (job `publish`).

## 8. CI và phát hành

### 8.1 `ci.yml`

Job `linux` thêm, sau các bước hiện có:
1. `task linux:package VERSION=0.0.0-ci`; `dpkg-deb -I`/`-c` và `rpm -qip`/`-qlp` kiểm tên, phụ thuộc, danh sách file.
2. Cài/gỡ deb: `sudo apt install ./bin/ghostline_*.deb` → `systemctl is-active ghostline`, `sudo ghostlined status` (dùng `/usr/lib/ghostline/ghostlined`) trả lời. `sudo apt remove ghostline` → unit không chạy, không còn drop-in resolved, bảng `inet ghostline`, file CA trong anchors, luật ufw/firewalld của Ghostline. `sudo apt purge ghostline` → không còn `/var/lib/ghostline`, `/var/log/ghostline`.
3. Cài/gỡ `tar.gz`: giải nén, `sudo ./install.sh`, kiểm dịch vụ chạy; `sudo ./uninstall.sh --purge`, kiểm sạch như bước 2.
4. Upload `tar.gz` làm artifact.

Job mới `pkgbuild` (`needs: linux`, container `archlinux:base-devel`): tải artifact `tar.gz`, điền PKGBUILD với file cục bộ, chạy `makepkg` bằng user thường.

### 8.2 `release.yml`

- `windows`: job `release` hiện có đổi tên; giữ build, đóng gói, size guard; bỏ bước tạo release và checksum; upload installer + portable làm artifact.
- `linux` (song song): `task linux:package VERSION=<tag>`; upload bốn định dạng.
- `publish` (`needs: [windows, linux]`, ubuntu): tải mọi artifact; một `SHA256SUMS` (LF); điền PKGBUILD (`@VERSION@`, `@SHA256@` của `tar.gz`); `gh release create <tag> … --generate-notes` với mọi file + PKGBUILD + `ghostline.install`.
- Nội dung gói Windows không đổi.

## 9. Tài liệu và checklist

- **README/README.vi:** badge Linux; mục Cài đặt: Ubuntu/Debian (deb), Fedora (rpm), Arch/CachyOS (`tar.gz` + `install.sh` hoặc `makepkg` với PKGBUILD), distro khác (AppImage); nhóm `ghostline`; mục Hạn chế đã biết trên Linux (GNOME không khay, desktop ngoài GNOME/KDE không tự đặt proxy, Chrome trên Ubuntu cần `libnss3-tools`, kernel thiếu NFQUEUE thì không có DPI, không có GoodbyeDPI, gói chưa ký GPG, SteamOS chưa kiểm chứng).
- **`docs/user-guide.md`, `docs/huong-dan-su-dung.md`:** phần Linux (cài/gỡ, dịch vụ nền, nhóm `ghostline`, khởi động cùng hệ thống); phần SteamOS (Desktop Mode, `passwd`, cài dịch vụ từ AppImage), ghi rõ chưa kiểm chứng.
- **`docs/platforms.md`:** thêm dòng đóng gói, cài dịch vụ, khởi động cùng hệ thống.
- **`docs/release-checklist.md`:** danh sách thủ công Linux theo §19.3 spec tổng, thêm:
  - Ubuntu: CA biến mất khỏi profile Firefox snap sau khi tắt Fake SNI và sau khi gỡ gói.
  - Steam Deck (tuỳ chọn, khi có máy): spike §16.4 (webkitgtk-6.0, glibc ≥ 2.39, NFQUEUE/`nft_queue`, `/etc` và `/var` còn sau cập nhật), rồi kịch bản AppImage → `passwd` → cài dịch vụ → Connect + DPI → Game Mode → reboot.

## 10. Kiểm thử

- **Unit:** `internal/sysinstall` trên root giả (cài lần đầu, cài lại = cập nhật, từ chối khi có unit của gói, gỡ, `--purge`, không phải root); render unit từ mẫu; nhận biết cách cài; đồng bộ file autostart (ghi, xoá, không ghi lại khi không đổi, `$APPIMAGE`); dòng nhắc SteamOS; `ServiceManaged.SetAutostart`.
- **Frontend:** nút trên `DAEMON_UNREACHABLE` và trang cập nhật dịch vụ theo từng cách cài; SteamOS; đủ chuỗi vi/en.
- **CI:** mục 8.1.
- **Root (máy dev, script để người bảo trì chạy):** cài bằng `tar.gz` với unit đã hardening; bộ test root; Connect với DPI, proxy, Fake SNI, chia sẻ LAN; `kill -9 ghostlined` → DNS về trong 5 giây; gỡ với `--purge`; so trạng thái trước/sau.
- **Windows:** mọi test cũ, lint chéo, depcheck, size guard.

## 11. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Hardening thiếu capability làm hỏng một tính năng chỉ lộ ra khi chạy thật | Lần chạy root đủ tính năng với unit đã hardening trước khi merge; checklist phát hành chạy trên deb/rpm |
| Runner GitHub khác máy thật (không có desktop, NM) | CI chỉ kiểm cài/gỡ và trạng thái hệ thống; tính năng desktop kiểm trong checklist |
| AppImage không chạy trên SteamOS | Ghi rõ chưa kiểm chứng; phương án dự phòng trong sub-spec riêng |
| `pkexec` không có tác nhân polkit (một số WM tối giản) | Thông báo lỗi kèm lệnh terminal tương đương |
| Gỡ gói khi daemon đã chết | prerm gọi `--remove-certs`, chạy được không cần daemon (cùng đường khôi phục như `ExecStopPost`) |
