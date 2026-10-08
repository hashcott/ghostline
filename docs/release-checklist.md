# Danh sách kiểm tra trước khi phát hành

Chạy trên Windows 11 x64, terminal **admin**. Đánh dấu từng mục; mục nào hỏng thì không phát hành.

## Test tự động

- [ ] `go test ./...` và `cd frontend && npm test` xanh
- [ ] `go test -tags integration ./internal/sysdns/... ./internal/startup/... ./internal/sysproxy/...` xanh (admin)

## Kiểm tra thủ công (spec §11)

- [ ] **Máy Windows 11 vừa cài mới**: tải zip portable, chạy, bấm Connect → **Đã bảo vệ ≤ 25 giây**. Ngắt rồi Connect lại → **≤ 5 giây** (dùng kết quả quét đã lưu).
- [ ] **Connect / Disconnect**: sau Disconnect, `Get-DnsClientServerAddress` cho mọi card mạng trả về đúng DNS ban đầu (DHCP hay tĩnh).
- [ ] **App bị kill**: khi đang kết nối chạy `taskkill /F /IM ghostline.exe`; trong **≤ 3 giây** DNS trở về như cũ (watchdog). Đo bằng `Get-DnsClientServerAddress` lặp mỗi 0,5 giây.
- [ ] **Khởi động lại máy khi đang kết nối** (giữ nút nguồn): sau khi đăng nhập, DNS được khôi phục dù không mở Ghostline (tác vụ `Ghostline Recovery`).
- [ ] **Cổng 53 bị ICS giữ**: bật Mobile Hotspot (svchost giữ `0.0.0.0:53`) → Connect vẫn thành công, DNS đi qua Ghostline, hotspot vẫn chạy.
- [ ] **Cổng 53 bị chiếm thật**: một chương trình khác nghe `127.0.0.1:53` → Connect báo `PORT53_BUSY` kèm tên và PID; nút "Tạm dừng dịch vụ" (khi là dịch vụ) hỏi xác nhận trong trang; không có gì bị dừng khi chưa xác nhận.
- [ ] **Đổi Wi-Fi sang dây mạng** khi đang kết nối: card mới được chụp DNS và đặt về loopback; Disconnect khôi phục cả hai card.
- [ ] **Máy ngủ rồi thức** khi đang kết nối: DNS vẫn hoạt động; nếu engine hỏng, nhật ký có "đang tìm máy chủ khác".
- [ ] **Wireshark** với filter `udp.port==53 || tcp.port==53` trên card mạng thật: khi đã bảo vệ, không có truy vấn DNS plain ra ngoài, trừ bootstrap tới 1.1.1.1/8.8.8.8 để phân giải hostname máy chủ DoH.
- [ ] **Vượt DPI**: bật GoodbyeDPI preset Nhẹ → chạy; tắt → service `WinDivert1.4` biến mất (`sc query WinDivert1.4`).
- [ ] **Installer**: cài, chạy, kết nối; gỡ cài đặt khi đang kết nối → DNS khôi phục, tác vụ `Ghostline` và `Ghostline Recovery` bị xoá, service `WinDivert1.4` bị xoá.
- [ ] **Ngôn ngữ**: chuyển VI ↔ EN, không còn chuỗi nào chưa dịch.

## Kiểm tra thủ công giai đoạn 2A (spec 2A §11)

- [ ] **Trình duyệt vào trang bị chặn theo SNI** qua system proxy (GoodbyeDPI tắt): bật proxy + "dùng cho máy này", Connect, mở trang → vào được; lần thứ hai không chậm thêm.
- [ ] **Điện thoại trong LAN** dùng proxy qua mã QR (mạng Private) → duyệt web được; trang Proxy hiện 1 thiết bị LAN.
- [ ] **Mạng Public** → trang Proxy hiện cảnh báo, điện thoại không vào được, Ghostline không tự đổi profile mạng.
- [ ] **`taskkill /F /IM ghostline.exe`** khi đang dùng system proxy + chia sẻ LAN → trong ≤ 3 giây system proxy trở về như cũ và luật `Ghostline Proxy` bị xoá (`netsh advfirewall firewall show rule name="Ghostline Proxy"` báo không có).
- [ ] **Khởi động lại máy** khi đang dùng system proxy → sau đăng nhập system proxy được khôi phục.
- [ ] **VPN đổi system proxy giữa chừng** → Ghostline báo `SYSPROXY_TAKEN_OVER`, không giành lại; Disconnect không ghi đè cài đặt của VPN.
- [ ] **Máy đã có proxy/PAC khác** → Connect hỏi xác nhận trước khi ghi đè; chọn "không dùng system proxy" thì proxy vẫn chạy.
- [ ] **Wireshark** (`udp.port==53 || tcp.port==53`) khi duyệt web qua proxy: không có DNS plain do proxy gây ra.
- [ ] **Import danh sách từ link GitHub** cho cả 6 mục "Thêm nhanh" → nhận đúng định dạng, có số mục, ô "Thử tên miền" chỉ đúng danh sách.
- [ ] **Gỡ cài đặt** khi đang chia sẻ LAN → luật `Ghostline Proxy` bị xoá.

## Kiểm tra thủ công engine zapret2 (spec zapret2 §10.2)

- [ ] **Cài mới, Connect:** các trang mẫu mở được bằng zapret2; YouTube trên Chrome chạy QUIC (kiểm tra `chrome://net-internals`) vẫn xem được.
- [ ] **Tự dò với zapret2** trên ít nhất hai nhà mạng (ví dụ Viettel, VNPT): tìm được chiến lược; ghi lại chiến lược thắng để chỉnh `lists/strategies.json`.
- [ ] **Nâng cấp từ v0.2.5:** engine là GoodbyeDPI 0.2.3rc3, preset cũ chạy được, dịch vụ `WinDivert1.4` cũ đã bị gỡ.
- [ ] **Đổi GoodbyeDPI → zapret2 → GoodbyeDPI khi đang kết nối:** `sc qc WinDivert` trỏ đúng file `.sys` của engine đang chạy.
- [ ] **Kill `ghostline.exe` bằng Task Manager:** `winws2` chết theo; dịch vụ được gỡ trong ≤ 3 giây.
- [ ] **Để Defender chặn `winws2.exe`** (bỏ loại trừ, giải nén lại): Connect quay về GoodbyeDPI, banner hướng dẫn hiện ra.
- [ ] **Phạm vi danh sách đen + tự phát hiện:** mở một trang bị chặn không có trong danh sách ba lần → domain hiện trong danh sách tự thêm, lần sau mở được.
- [ ] **Tự chạy một `winws2` ngoài Ghostline rồi Connect:** lỗi `DPI_START_FAILED` với gợi ý đúng.
- [ ] **Defender với bản build:** `ghostline.exe` (đã nhúng `winws2.exe`) không bị Defender xoá khi tải về và khi chạy.
- [ ] **Nâng zapret2:** `grep -n "_G\[\|load(" assets/zapret2/lua/*.lua` — mọi khoá tham số mới mà thư viện tra như tên hàm phải được thêm vào `funcKeys` trong `internal/dpi/zapret2/validate.go`.
- [ ] **Ký danh sách chiến lược** (tăng `version` mỗi lần đổi): `go run ./tools/genservers -sign-file lists/strategies.json -sign-env SERVERLIST_SIGNING_KEY`, commit cả `lists/strategies.json.sig`.
- [ ] **Gửi `winws2.exe` và `ghostline.exe`** lên https://www.microsoft.com/wdsi/filesubmission (báo nhầm); ghi lại mã gửi.

## Kiểm tra thủ công giai đoạn 2B (spec 2B §12)

- [ ] **Preset Fake SNI:** làm theo `lists/fakesni/README.md` trên mạng bị chặn, ghi kết quả; ký từng preset và commit `.sig`. `GHOSTLINE_RELEASE=1 go test ./lists/` phải qua.
- [x] **iPhone:** cài `.mobileconfig` qua trang cài đặt (QR), bật tin cậy hoàn toàn; DoH chạy ở Wi-Fi nhà; ra 4G vẫn có mạng; ghi lại hành vi khi Ghostline ngắt kết nối và cập nhật hướng dẫn.
  - Kết quả (0.4.0-dev, 2026-10-05): thiếu bước bật tin cậy hoàn toàn thì iPhone kết nối Wi-Fi nhưng không vào được mạng; bật xong DoH chạy. Mạng Public: iPhone bị chặn; đổi sang Private thì chạy. 4G: vẫn có mạng. Ghostline ngắt kết nối: iPhone **mất mạng ở Wi-Fi nhà** (iOS không có DNS dự phòng); có mạng lại khi chọn *Tự động* trong *VPN và quản lý thiết bị › DNS*. Hướng dẫn, README và trang cài đặt đã ghi điều này.
- [ ] **Hỏi lại khi ngắt kết nối:** có thiết bị LAN dùng DNS server trong 10 phút gần nhất → nút Ngắt kết nối trong app (cả hai chế độ) và trên khay hỏi lại, chọn Không thì vẫn kết nối; không có thiết bị nào thì không hỏi; tắt Windows và Thoát không hỏi.
- [ ] **Android:** tắt DNS riêng tư, đặt DNS tĩnh là IP máy; duyệt web được.
- [ ] **Steam Deck:** đặt DNS thủ công (ghi lại làm được ở Game Mode hay phải sang Desktop mode, cập nhật hướng dẫn).
- [ ] **Router / TV:** đặt DNS là IP máy; truy vấn hiện trong số thiết bị LAN.
- [ ] **Chrome, Edge, Firefox với Fake SNI:** trang trong preset mở được khi tắt cả hai engine vượt DPI; banner tím hiện ở cả hai chế độ.
- [ ] **`certlm.msc`:** không còn `Ghostline Fake SNI` sau Disconnect, sau `taskkill /F`, sau khởi động lại máy; không còn chứng chỉ Ghostline nào sau gỡ cài đặt.
- [ ] **Wireshark:** không có DNS plain do DoH server, DNS cho LAN hay Fake SNI gây ra (ngoài truy vấn LAN tới cổng 53 của máy).
- [ ] **Mạng Public:** thiết bị khác không vào được (kể cả khi đã bấm Allow ở hộp thoại firewall của Windows); có luật `Ghostline Block Public` khi đang chia sẻ, mất sau Disconnect; gợi ý đổi sang Private hiện ra.
- [ ] **Cổng 53 bị ICS chiếm** (bật chia sẻ Internet): IP đó bị bỏ qua, các IP khác vẫn chạy.

## Kiểm tra thủ công giai đoạn 3 (spec 3 §13)

- [ ] **Bảng xếp hạng máy chủ:** cài mới, Connect → bước 2 quét khoảng 900 máy chủ; Ngắt rồi Connect lại ≤ 5 giây; trang Máy chủ và bước 2 cùng một tổng số; máy chủ `adblock`/`family` ghi *không tự chọn*. Chọn một mức khi đang kết nối → dòng mô tả hiện tiến độ, xong thì máy chủ đang dùng đổi mà không ngắt kết nối.
- [ ] **Hai mạng cùng gateway** (ví dụ hai router đều `192.168.1.1`): mỗi mạng có bảng xếp hạng riêng (mạng thứ hai quét lại từ đầu).
- [ ] **Danh sách DNSCrypt nhúng sẵn:** chạy `go run ./tools/fetchdnscrypt` (tải và kiểm tra chữ ký minisign), `go test ./lists/` phải qua, rồi commit `lists/dnscrypt/`.
- [ ] **Dải IP Cloudflare:** chạy `go run ./tools/gencfranges`, xem lại diff của `internal/cfscan/ranges_v4.txt` rồi commit.
- [ ] **Lookup đầu độc:** trên mạng Viettel, VNPT, FPT, tra một tên miền bị chặn qua DNS nhà mạng và qua Ghostline → thẻ **Bị đầu độc DNS**.
- [ ] **IP Cloudflare:** có ≥ 10 IP dùng được trong ≤ 60 giây; một rule `ip=` tạo từ kết quả mở được một trang thật nằm sau Cloudflare.
- [ ] **Chuyển máy:** xuất trên máy A, nhập trên máy B cài mới → rule, server, ghim, vượt DPI giống nhau; Fake SNI, DNS server cho LAN và chia sẻ proxy vẫn tắt; mật khẩu proxy upstream phải nhập lại.
- [ ] **Tường lửa và antivirus:** quét IP Cloudflare ở mặc định không làm Windows Defender Firewall hay antivirus cảnh báo.
- [ ] **`Ghostline.exe --export x.json`** chạy trong PowerShell: hỏi UAC, ghi được file, in "settings exported to …" (hoặc lỗi) ra console và không mở cửa sổ.

## Kiểm tra thủ công Linux (spec Linux §19.3, L5)

CI đã cài, gỡ và purge gói deb và tar.gz trên runner, và chạy `makepkg` với PKGBUILD. Phần dưới cần máy (hoặc VM) có desktop.

- [ ] **Ubuntu 24.04 GNOME (VM):** cài deb → dịch vụ chạy, mở app từ menu; Connect → `resolvectl query example.com` đi qua Ghostline, kiểm tra rò rỉ sạch; zapret2 mở được site bị chặn; proxy hệ thống GNOME bật/tắt đúng; Fake SNI với Firefox snap và Chrome (có `libnss3-tools`); chia sẻ LAN qua ufw; `apt remove` → DNS, proxy, chứng chỉ, ufw, bảng nft về như trước; CA biến mất khỏi profile Firefox snap (cả sau khi chỉ tắt Fake SNI).
- [ ] **Fedora 41 (VM):** cài rpm; NetworkManager + resolved; firewalld; Fake SNI qua p11-kit; `dnf remove` gỡ sạch.
- [ ] **Arch KDE (VM hoặc máy thật):** build từ PKGBUILD (`makepkg -si`), `systemctl enable --now ghostline` (hoặc nút "khởi động dịch vụ"); proxy KDE; tắt NetworkManager để thử resolv.conf thuần; `pacman -R` gỡ sạch.
- [ ] **AppImage trên distro chưa cài gói:** mở → trang "dịch vụ nền chưa chạy" có nút "cài dịch vụ" → nhập mật khẩu → kết nối được; huỷ hộp thoại mật khẩu → hiện lệnh để chạy trong terminal.
- [ ] **Khởi động cùng hệ thống:** bật → có `~/.config/autostart/ghostline.desktop`; reboot → tự kết nối và app nằm ở khay; tắt → file biến mất.
- [ ] **Rút điện khi đang kết nối** → boot lại → DNS về như cũ (hoặc tự kết nối lại nếu bật khởi động cùng hệ thống).
- [ ] **Steam Deck (tuỳ chọn, khi có máy):** spike §16.4 spec tổng — có `webkitgtk-6.0` và glibc ≥ 2.39 không, kernel có NFQUEUE/`nft_queue` không, `/etc` và `/var` còn sau một lần cập nhật SteamOS không; rồi AppImage → `passwd` → cài dịch vụ → Connect + DPI → Game Mode → reboot. Ghi kết quả vào README (bỏ "chưa kiểm chứng" nếu chạy).

## Cần xác minh trên máy thật (reviewer không kiểm chứng được)

- [ ] **Luật firewall với đường dẫn có dấu cách và chữ có dấu** (`C:\Program Files\…`, `C:\Users\Đức Hạnh\…` cho bản portable): `netsh` tạo đúng luật `Ghostline Proxy` cho exe đó.

- [ ] **Tắt máy khi app đang ẩn ở khay**: Windows gửi `WM_QUERYENDSESSION` tới cửa sổ ẩn, và DNS được trả về trước khi tắt.
- [ ] **Khởi động cùng Windows** (`--autostart` qua Task Scheduler) rồi Connect: watchdog vẫn sống sau khi tác vụ kết thúc (thử `taskkill /F` → DNS khôi phục).
- [ ] **Windows 11 có cấu hình DoH riêng cho từng card mạng**: sau Disconnect, cấu hình DoH ban đầu vẫn còn.
- [ ] **Danh sách đen GoodbyeDPI và zapret2 khi tên người dùng có dấu** (ví dụ `C:\Users\Đức Hạnh`): phạm vi "danh sách đen" hoạt động.
- [ ] **Nhật ký → lưu file** trong WebView2 tải được file `ghostline-log.txt`.
