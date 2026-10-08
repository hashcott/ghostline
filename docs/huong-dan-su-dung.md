# Hướng dẫn sử dụng Ghostline

[English](user-guide.md)

Hướng dẫn này dành cho người dùng Windows 10/11, không cần biết về kỹ thuật. Bạn chỉ cần đọc phần 1–3 là dùng được ngay; các phần sau dành cho khi bạn muốn tinh chỉnh hoặc gặp sự cố.

## Mục lục

1. [Ghostline làm gì?](#1-ghostline-làm-gì)
2. [Cài đặt](#2-cài-đặt)
3. [Bắt đầu nhanh: một nút bấm](#3-bắt-đầu-nhanh-một-nút-bấm)
4. [Giao diện Đầy đủ](#4-giao-diện-đầy-đủ)
   - [Tổng quan](#41-tổng-quan)
   - [Máy chủ](#42-máy-chủ)
   - [Vượt DPI](#43-vượt-dpi)
   - [Nhật ký](#44-nhật-ký)
   - [Cài đặt](#45-cài-đặt)
   - [Proxy](#46-proxy)
   - [Rules và danh sách](#47-rules-và-danh-sách)
   - [DNS server](#48-dns-server)
   - [Fake SNI](#49-fake-sni)
   - [Công cụ](#410-công-cụ)
   - [Sao lưu và chuyển máy](#411-sao-lưu-và-chuyển-máy)
5. [Icon ở khay hệ thống](#5-icon-ở-khay-hệ-thống)
6. [Khi một trang web vẫn bị chặn](#6-khi-một-trang-web-vẫn-bị-chặn)
7. [Xử lý sự cố](#7-xử-lý-sự-cố)
8. [Gỡ cài đặt](#8-gỡ-cài-đặt)
9. [Câu hỏi thường gặp](#9-câu-hỏi-thường-gặp)

---

## 1. Ghostline làm gì?

Mỗi khi bạn mở một trang web, máy tính phải hỏi **DNS** "trang này ở địa chỉ IP nào?". Bình thường câu hỏi đó được gửi đi **không mã hoá**, nên nhà mạng có thể đọc, ghi lại hoặc trả lời sai để chặn trang.

Ghostline chạy một DNS nhỏ ngay trên máy bạn (`127.0.0.1`), trỏ mọi card mạng về đó, rồi gửi câu hỏi DNS qua kênh **mã hoá** (DoH, DoT, DoQ, DNSCrypt) tới máy chủ nhanh nhất. Nếu nhà mạng còn chặn bằng cách soi gói tin (DPI), Ghostline có thể bật thêm engine vượt DPI (**zapret2** hoặc **GoodbyeDPI**) để vượt qua.

Điều quan trọng nhất: **Ghostline luôn trả lại DNS gốc của bạn** khi ngắt kết nối, kể cả khi app bị tắt đột ngột hay máy mất điện.

## 2. Cài đặt

Tải về từ trang [Releases](https://github.com/hashcott/ghostline/releases). Có hai lựa chọn:

| Bản | Khi nào nên dùng |
| --- | --- |
| `ghostline-amd64-installer.exe` | Dùng lâu dài trên máy của bạn. Có shortcut trong Start Menu, gỡ được qua Settings → Apps. |
| `Ghostline-<phiên bản>-portable.zip` | Không muốn cài, hoặc chạy từ USB. Giải nén ra một thư mục rồi chạy `ghostline.exe`. Mọi dữ liệu nằm trong thư mục `data\` bên cạnh. |

**Kiểm tra file (khuyến nghị).** Mở PowerShell trong thư mục chứa file vừa tải:

```powershell
Get-FileHash .\ghostline-amd64-installer.exe -Algorithm SHA256
```

So mã hiện ra với dòng tương ứng trong file `SHA256SUMS` trên trang Releases. Nếu khác nhau, **đừng chạy file đó**.

**Cảnh báo SmartScreen.** Bản phát hành chưa được ký số nên Windows sẽ hiện "Windows protected your PC". Sau khi đã kiểm tra SHA-256, bấm **More info → Run anyway**.

**Quyền admin.** Ghostline cần quyền quản trị để đổi DNS và chạy GoodbyeDPI, nên Windows sẽ hỏi UAC mỗi lần mở. Chọn **Yes**. Nếu bật *Khởi động cùng Windows*, app được mở qua Task Scheduler và không hỏi UAC nữa.

**Phần mềm diệt virus.** zapret2 và GoodbyeDPI dùng driver **WinDivert**, hay bị antivirus báo nhầm. Ghostline kiểm tra mã băm của engine trước mỗi lần chạy. Nếu zapret2 bị chặn, Ghostline tạm dùng GoodbyeDPI và trang Vượt DPI hiện đường dẫn thư mục `bin\zapret2` để bạn thêm vào danh sách loại trừ (exclusions) của Windows Defender.

### Linux

Chọn file cho bản phân phối của bạn ở trang Releases (xem bảng trong README): `.deb` cho Ubuntu và Debian, `.rpm` cho Fedora, `tar.gz` (hoặc `PKGBUILD` đính kèm) cho Arch, AppImage cho các bản khác. Kiểm tra bằng `sha256sum -c SHA256SUMS --ignore-missing`.

- **Hai phần.** Dịch vụ nền `ghostline.service` chạy bằng root: đổi DNS, chạy zapret2 và proxy, vẫn bảo vệ khi đã đóng cửa sổ, sau khi khởi động lại máy và cả khi bạn đã đăng xuất. Cửa sổ app chạy bằng tài khoản của bạn và chỉ nói chuyện với dịch vụ, không bao giờ cần `sudo`.
- **Ai được điều khiển.** Thành viên các nhóm `wheel`, `sudo`, `admin` hoặc `ghostline`. Cho tài khoản khác: `sudo usermod -aG ghostline <tài khoản>` rồi đăng nhập lại. Tài khoản khác sẽ thấy "Tài khoản của bạn không có quyền điều khiển Ghostline".
- **Dịch vụ chưa chạy.** Cửa sổ báo điều đó và có nút **khởi động dịch vụ** (deb, rpm, Arch) hoặc **cài dịch vụ** (AppImage, tar.gz); cả hai hỏi mật khẩu qua hộp thoại của hệ thống. Trong terminal: `sudo systemctl enable --now ghostline`.
- **Khởi động cùng hệ thống.** Trong Cài đặt, *khởi động cùng hệ thống* tự kết nối lúc bật máy (khi bật *tự động kết nối*) và mở Ghostline ở khay khi bạn đăng nhập.

### Steam Deck (SteamOS) — chưa kiểm chứng

Ghostline chưa được thử trên Steam Deck. Cách dự kiến, trong Desktop Mode:

1. Mở Konsole và đặt mật khẩu cho tài khoản `deck` một lần: `passwd`. Bước cài dịch vụ sẽ hỏi mật khẩu này.
2. Tải AppImage, cho phép chạy (*Properties → Permissions*) rồi mở.
3. Bấm **cài dịch vụ** và nhập mật khẩu. Kết nối.
4. Chuyển sang Game Mode: dịch vụ vẫn bảo vệ, không cần mở cửa sổ.

Nếu AppImage báo thiếu WebKitGTK 6.0 thì bản SteamOS này chưa chạy được cửa sổ; hãy mở issue kèm phiên bản SteamOS.

## 3. Bắt đầu nhanh: một nút bấm

<p align="center"><img src="screenshots/simple-vi.png" width="320" alt="Giao diện Đơn giản"></p>
<a href="videos/clips/connect-vi.mp4"><img src="videos/clips/connect-vi.webp" width="640" alt="Kết nối ở giao diện Đơn giản"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

Khi mở lần đầu, Ghostline ở giao diện **Đơn giản**.

1. Bấm **nút nguồn tròn ở giữa**.
2. Ghostline lần lượt: kiểm tra hệ thống → chọn máy chủ → bật engine → chụp DNS gốc → bật lưới an toàn → đặt DNS → xác minh không rò rỉ. Để chọn máy chủ, Ghostline giữ một bảng xếp hạng toàn bộ máy chủ cho mỗi mạng. Lần đầu trên một mạng, app quét hết danh sách (khoảng 900 máy chủ, 30–60 giây; bước này hiện tiến độ, ví dụ *chọn máy chủ 120/906*) rồi lấy những máy chủ nhanh nhất không lọc nội dung, máy chủ đã ghim được ưu tiên. Những lần sau app kết nối trong vài giây nhờ bảng xếp hạng. Khi bảng xếp hạng cũ hơn một ngày, app kiểm tra nhanh các máy chủ đứng đầu trước khi dùng và quét lại cả bảng ở nền sau khi đã kết nối.
3. Khi vòng tròn sáng xanh và hiện **[ ĐÃ BẢO VỆ ]** là xong: mọi truy vấn DNS của máy đã được mã hoá.

Muốn huỷ khi đang kết nối: bấm nút nguồn lần nữa. Muốn tắt bảo vệ: bấm nút nguồn khi đang ở trạng thái Đã bảo vệ, DNS được trả về như cũ.

<a href="videos/clips/disconnect-vi.mp4"><img src="videos/clips/disconnect-vi.webp" width="640" alt="Ngắt kết nối"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

**Lần kết nối đầu tiên: tự dò.** Lần đầu bấm kết nối sau khi cài, Ghostline thêm một bước vào danh sách: khi vượt DPI (nếu bật) đã chạy, app kiểm tra các trang mẫu, và nếu vẫn còn trang bị chặn thì tự dò vượt DPI trên các trang đó, kèm cách đang thử (ví dụ *tự dò vượt DPI: thử cách 2/4*). Tìm được chiến lược thì vượt DPI tự bật và mức chuyển sang **DNS + vượt DPI**; không trang nào bị chặn thì giữ nguyên mức. Bước này chỉ chạy một lần; ngắt kết nối giữa chừng thì lần kết nối sau chạy lại. Trong lúc tự dò, các nút mức tạm khoá.

**Mức bảo vệ.** Dưới dòng trạng thái, chọn Ghostline làm tới đâu. Đổi được bất cứ lúc nào, kể cả khi đang kết nối:

| Mức | Bật gì | Khi nào |
| --- | --- | --- |
| **Chỉ DNS** | DNS mã hoá | Nhà mạng chỉ chặn bằng DNS; nhẹ nhất |
| **DNS + vượt DPI** *(khuyên dùng)* | Thêm vượt DPI cho mọi ứng dụng | Đổi DNS rồi mà trang vẫn bị chặn |
| **Tối đa** | Thêm proxy cho máy này (đặt proxy hệ thống của Windows); trình duyệt tự được fragment khi gặp trang bị chặn | Vượt DPI vẫn chưa đủ với vài trang |
| **Tuỳ chỉnh** | Tổ hợp bạn tự chỉnh ở giao diện Đầy đủ | Sáng lên khi cài đặt ở đó không khớp mức nào |

Chọn một mức cũng là tìm máy chủ tốt nhất: app quét lại toàn bộ danh sách (dòng dưới các mức hiện *Đang tìm máy chủ tốt nhất 120/906…*) và, khi đang kết nối, chuyển sang những máy chủ nhanh nhất mà không ngắt kết nối. Bấm lại mức đang chọn để tìm lại.

<a href="videos/clips/levels-vi.mp4"><img src="videos/clips/levels-vi.webp" width="640" alt="Chọn mức bảo vệ"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

Mỗi mức chỉ đổi các công tắc trên; engine, chiến lược, máy chủ, rules và các cài đặt khác giữ nguyên. Khi chuyển từ **Tuỳ chỉnh** sang một mức, Ghostline ghi nhớ tổ hợp của bạn, bấm **Tuỳ chỉnh** để lấy lại. Fake SNI cần proxy: chọn mức không có proxy sẽ hỏi trước khi tắt Fake SNI, và **Tuỳ chỉnh** bật lại nó.

**Bảng thông tin bên dưới:**

| Dòng | Ý nghĩa |
| --- | --- |
| máy chủ | Máy chủ DNS đang dùng; `+4` nghĩa là còn 4 máy chủ khác chạy song song làm dự phòng |
| độ trễ | Thời gian trung bình để nhận câu trả lời DNS |
| vượt dpi | Preset GoodbyeDPI đang chạy, hoặc *tắt* |
| thời gian | Đã bảo vệ được bao lâu |

**Các trạng thái có thể gặp:**

| Trạng thái | Ý nghĩa |
| --- | --- |
| CHƯA BẢO VỆ | Đang dùng DNS gốc của máy |
| ĐANG KẾT NỐI | Đang chạy các bước ở trên |
| ĐÃ BẢO VỆ | Mọi thứ hoạt động bình thường |
| SUY GIẢM | Máy chủ phản hồi chậm hoặc không phản hồi; Ghostline đang tự tìm máy chủ khác. Mạng vẫn dùng được |
| LỖI | Không kết nối được. **DNS của máy không bị thay đổi**. Đọc thông báo bên dưới để biết cách xử lý (xem [phần 7](#7-xử-lý-sự-cố)) |

## 4. Giao diện Đầy đủ

Bấm **ĐẦY ĐỦ** ở góc trên bên trái để xem mọi trang và cài đặt. Bấm **ĐƠN GIẢN** để quay lại. Thanh bên trái luôn có trạng thái hiện tại và nút **⏻ KẾT NỐI / NGẮT KẾT NỐI**.

Thanh bên chia trang thành nhóm: **cơ bản** (tổng quan, máy chủ, vượt DPI), **nâng cao** (proxy, rules, DNS server) và **chẩn đoán** (công cụ), cuối cùng là cài đặt. **Fake SNI** là một tab của trang **proxy**, còn **nhật ký** là tab đầu tiên của trang **công cụ**.

### 4.1. Tổng quan

![Tổng quan](screenshots/overview-vi.png)

- **Dòng trên cùng:** trạng thái, thời gian bảo vệ, và đường đi của DNS: `127.0.0.1 → các máy chủ đang dùng`.
- **Độ trễ · 60 giây:** biểu đồ độ trễ trong một phút gần nhất. Đường càng thấp càng tốt.
- **Truy vấn:** số câu hỏi DNS đã xử lý từ lúc kết nối.
- **Máy chủ đang dùng:** các máy chủ Ghostline gửi truy vấn tới song song; câu trả lời nhanh nhất được dùng.

### 4.2. Máy chủ

![Máy chủ](screenshots/servers-vi.png)

<a href="videos/clips/servers-vi.mp4"><img src="videos/clips/servers-vi.webp" width="640" alt="Quét, tìm và ghim máy chủ"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

**Từng bước: dùng máy chủ bạn thích**

1. Mở **Máy chủ**, bấm **⟳ quét toàn bộ** và chờ quét xong.
2. Gõ tên, nhà cung cấp hoặc giao thức vào ô tìm kiếm, ví dụ `cloudflare`.
3. Bấm ngôi sao **☆** ở máy chủ muốn dùng, hoặc **★ ghim N kết quả** để ghim tất cả kết quả.
4. Nếu đang kết nối, bấm **kết nối lại để áp dụng**.
5. Muốn chỉ dùng máy chủ đã ghim, bật **chỉ dùng máy chủ đã ghim**.

Danh sách toàn bộ máy chủ DNS mã hoá mà Ghostline biết (vài trăm máy chủ), được cập nhật mỗi ngày từ danh sách có chữ ký số.

- **⟳ quét toàn bộ:** đo lại độ trễ của mọi máy chủ, loại các máy chủ trả kết quả sai (bị đầu độc) và, khi đang kết nối, chuyển sang những máy chủ nhanh nhất mà không ngắt kết nối. Ghostline cũng tự làm việc này khi bảng xếp hạng cũ hơn một ngày hoặc khi bạn vào mạng khác. Máy chủ có lọc nội dung (`adblock`, `family`) vẫn được đo nhưng ghi *không tự chọn*: app không bao giờ tự chọn chúng; muốn dùng thì ghim.
- **"Đạt" nghĩa là gì:** máy chủ trả lời đúng tên miền thử 2 lần trong 3 giây, bằng IP công cộng. "Đạt" không phát hiện được máy chủ chặn riêng vài trang, nhà mạng trả IP công cộng giả, hay lỗi DNSSEC; muốn kiểm tra kỹ hơn, dùng **Công cụ › Scanner**. Tên miền thử đổi ở **Cài đặt**.
- **Lọc:** chọn giao thức (`doh`, `dot`, `doq`, `dnscrypt`) hoặc loại máy chủ:
  - `no-filter`: không chặn gì.
  - `adblock`: chặn quảng cáo và theo dõi.
  - `family`: chặn nội dung người lớn, phù hợp cho máy của trẻ em.
  - **chỉ đạt:** chỉ hiện các máy chủ đang hoạt động tốt.
- **Bấm tiêu đề cột** (tên, độ trễ…) để sắp xếp.
- **☆ Ghim:** bấm ngôi sao (hoặc double-click vào dòng) để ghim máy chủ bạn thích. Chuột phải vào dòng để có thêm: chỉ dùng máy chủ này, kiểm tra lại, sao chép địa chỉ/IP, xoá. Máy chủ đã ghim luôn nằm trên cùng bảng và được **ưu tiên khi kết nối**: mọi máy chủ ghim đạt kiểm tra đều được dùng trước, các chỗ còn lại mới lấy máy chủ nhanh nhất. Bật **chỉ dùng máy chủ đã ghim** (cạnh ô tìm kiếm) để không dùng máy chủ nào khác.
- **Tìm kiếm và ghim hàng loạt:** gõ vào ô tìm kiếm (tên, nhà cung cấp, giao thức, địa chỉ, IP hoặc nhãn; nhiều từ thì phải khớp tất cả), rồi bấm **★ ghim tất cả kết quả**. Chip **★ đã ghim (N)** chỉ hiện máy chủ đã ghim; **bỏ ghim tất cả** để xoá hết. Đổi ghim khi đang kết nối thì bấm **kết nối lại để áp dụng**.
- **+ thêm:** thêm máy chủ riêng. Dán URL (`https://…`, `tls://…`, `quic://…`) hoặc stamp `sdns://…`, mỗi dòng một máy chủ, hoặc import từ file. Máy chủ tự thêm có nút **✕** để xoá.
- **Trạng thái:** *đang dùng* (đang nhận truy vấn), *đạt* (hoạt động tốt), *chưa kiểm tra*.

### 4.3. Vượt DPI

![Vượt DPI](screenshots/dpi-vi.png)

<a href="videos/clips/dpi-vi.mp4"><img src="videos/clips/dpi-vi.webp" width="640" alt="Trang Vượt DPI"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

**Từng bước: mở một trang đang bị chặn**

1. Bấm **Kết nối** trước; engine chỉ chạy khi đang kết nối.
2. Ở **Vượt DPI**, bật công tắc và giữ engine **zapret2**.
3. Thêm trang đó vào **Trang mẫu** (mỗi dòng một trang) rồi bấm **⟳ thử lại**.
4. Kết quả **✕ TLS** thường là do DPI: bấm **⚡ tự dò** và chờ. Ghostline giữ chiến lược nhẹ nhất mở được mọi trang mẫu.
5. Nếu chỉ vài trang bị chặn, đặt **phạm vi** là **danh sách đen** và chỉ liệt kê các trang đó.

Dùng khi DNS đã được mã hoá nhưng kết nối tới một trang **vẫn bị can thiệp**: thiết bị trên đường truyền soi tên trang trong gói tin (SNI) rồi ngắt kết nối.

> ⚖️ Bạn tự chịu trách nhiệm tuân thủ pháp luật và điều khoản của nhà mạng. Không dùng các tính năng này để truy cập nội dung bị cấm theo quy định của pháp luật. Xem [Tuyên bố miễn trừ trách nhiệm](../README.vi.md#tuyên-bố-miễn-trừ-trách-nhiệm).

**Vượt DPI cho mọi ứng dụng**

- **Công tắc:** bật/tắt. Engine chỉ chạy khi Ghostline **đang kết nối**:
  - `● zapret2 đã chạy (preset …)`: đang hoạt động.
  - `○ đang khởi động…`: chờ vài giây để driver WinDivert sẵn sàng.
  - `○ Đã bật — sẽ chạy khi kết nối`: bạn đã bật nhưng chưa kết nối.
- **Engine:**
  - **zapret2 (khuyên dùng):** mạnh hơn, có gói giả, nhiều kiểu cắt và hỗ trợ QUIC (YouTube, Google). Danh sách chiến lược được Ghostline cập nhật hằng ngày (có chữ ký), không cần cài bản mới.
  - **GoodbyeDPI:** engine cũ. Bản cài từ trước khi có zapret2 vẫn dùng GoodbyeDPI cho tới khi bạn tự đổi.
  - Nếu zapret2 bị antivirus chặn, Ghostline tạm chạy GoodbyeDPI, báo *suy giảm* và hiện nút **thử lại zapret2**.
- **Preset:** mức độ can thiệp vào gói tin.
  - **Nhẹ → Vừa → Mạnh → Cực mạnh:** mức càng cao càng dễ vượt chặn nhưng có thể làm vài trang chậm hoặc lỗi. Nên bắt đầu từ **Nhẹ**.
  - **Mode 1–6** (chỉ GoodbyeDPI): các cấu hình có sẵn của GoodbyeDPI, thử khi các mức trên không hiệu quả.
  - **Tự nhập:** nhập tham số của riêng bạn. Với zapret2 chỉ nhận `--lua-desync=…` gọi các hàm có sẵn; Ghostline từ chối các tham số nguy hiểm.
- **⚡ tự dò:** Ghostline tự thử từng preset từ nhẹ đến mạnh và giữ preset nhẹ nhất mở được tất cả *trang mẫu*. Cần **kết nối trước** khi tự dò. Bấm lần nữa để huỷ.
- **Phạm vi:**
  - **mọi kết nối:** áp dụng cho mọi trang web.
  - **danh sách đen:** chỉ áp dụng cho các domain trong danh sách, mỗi dòng một domain, rồi **lưu**. Cách này ít ảnh hưởng tới các trang khác nhất.
- **Tự phát hiện trang bị chặn** (zapret2, phạm vi danh sách đen): zapret2 tự nhận ra trang bị chặn và thêm vào một danh sách riêng hiện ngay bên dưới; bạn xoá được từng trang.
- **Dòng lệnh:** cho xem chính xác lệnh engine sẽ chạy.

**Fragment DNS**

Chia nhỏ gói tin gửi tới máy chủ DoH để nhà mạng khó nhận ra. Chỉ cần khi **không tìm được máy chủ nào** (nhà mạng chặn cả kết nối DNS mã hoá). Khi đang bật engine vượt DPI (zapret2 hoặc GoodbyeDPI) thì Fragment là thừa.

- **Số mảnh:** chia thành bao nhiêu phần (2–20).
- **Độ trễ (ms):** thời gian chờ giữa các mảnh.

**Trang mẫu**

Danh sách trang dùng để kiểm tra kết nối (mặc định: youtube.com, discord.com, x.com). Bấm **⟳ thử lại** để kiểm tra; mỗi trang hiện:

| Kết quả | Ý nghĩa |
| --- | --- |
| ✓ | Mở được |
| ✕ DNS | Không phân giải được tên |
| ✕ TCP | Không kết nối được tới máy chủ của trang |
| ✕ TLS | Bị chặn ở bước bắt tay mã hoá, thường do DPI → bật GoodbyeDPI hoặc tự dò |
| ✕ HTTP | Kết nối được nhưng trang trả lỗi |

Bạn có thể sửa danh sách trang mẫu trong ô bên dưới, mỗi dòng một trang.

### 4.4. Nhật ký

Mở ở **công cụ › nhật ký** (tab đầu tiên).

![Nhật ký](screenshots/logs-vi.png)

Ghi lại các sự kiện: kết nối, đổi máy chủ, bật/tắt GoodbyeDPI, lỗi.

- **Lọc:** tất cả, engine, dpi, hệ thống.
- **tạm dừng / tiếp tục:** dừng cuộn để đọc.
- **copy / lưu file:** sao chép hoặc lưu thành `ghostline-log.txt` để gửi khi báo lỗi.
- **hiện truy vấn:** xem từng truy vấn DNS theo thời gian thực. Chỉ giữ trong RAM, tối đa 500 dòng, **không bao giờ ghi xuống đĩa**.

### 4.5. Cài đặt

![Cài đặt](screenshots/settings-vi.png)

| Mục | Ý nghĩa |
| --- | --- |
| ngôn ngữ | VI hoặc EN (cũng đổi được bằng nút VI/EN ở góc trên) |
| khởi động cùng windows | Tự mở Ghostline khi đăng nhập, không hỏi UAC |
| tự kết nối khi mở | Bấm kết nối ngay khi app mở |
| đóng → thu xuống khay | Bấm ✕ thì ẩn xuống khay thay vì thoát. Ghostline vẫn bảo vệ ở chế độ nền |
| card mạng | **tự động**: bảo vệ mọi card mạng đang dùng (khuyến nghị). **chọn tay**: chỉ bảo vệ các card bạn chọn |
| tên miền thử | Các tên miền bộ quét máy chủ hỏi, mỗi dòng một tên (mặc định `www.google.com`); máy chủ phải trả lời đúng tất cả. Nên dùng 1–2 trang luôn mở được: mỗi tên miền thêm làm mọi lượt quét lâu hơn (tối đa 5). Tên miền mới được kiểm tra khi lưu: tên miền không có địa chỉ IPv4 (ví dụ `steam.com`; hãy dùng `store.steampowered.com`) sẽ bị từ chối. Nếu sau này một tên miền trượt ở hầu hết máy chủ, lượt quét bỏ qua nó và hiện cảnh báo nhờ bạn sửa. Áp dụng từ lần quét sau |
| bootstrap | DNS thường dùng để tìm địa chỉ của các máy chủ DoH lúc khởi động (mặc định `1.1.1.1:53`, `8.8.8.8:53`). Đây là lưu lượng DNS không mã hoá duy nhất, và chỉ dùng để tra tên máy chủ DoH |
| số máy chủ tối đa | Số máy chủ dùng song song (mặc định 5). Nhiều hơn thì ổn định hơn nhưng tốn băng thông hơn một chút |
| cập nhật danh sách máy chủ | Tải danh sách máy chủ mới mỗi ngày (có kiểm tra chữ ký) |
| báo có bản mới | Hiện thông báo khi có phiên bản mới. Ghostline **không bao giờ tự cập nhật** |
| ⚠ KHÔI PHỤC DNS NGAY | Đưa DNS mọi card mạng về trạng thái đã lưu. Dùng khi nghi ngờ DNS bị sai |

### 4.6. Proxy

![Proxy](screenshots/proxy-vi.png)

<a href="videos/clips/proxy-vi.mp4"><img src="videos/clips/proxy-vi.webp" width="640" alt="Bật và chia sẻ proxy"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

**Từng bước: dùng proxy cho máy này và cho điện thoại**

1. Bật **bật proxy**. Proxy chạy khi bạn đang kết nối.
2. Cho trình duyệt trên máy này: bật **dùng cho máy này**.
3. Cho điện thoại: bật **chia sẻ LAN**. Trên điện thoại vào *Wi-Fi › mạng này › Proxy › Thủ công* và nhập địa chỉ hiện ra, hoặc quét mã QR.
4. Nếu Ghostline báo mạng là *Public*, đổi sang *Private* trong Cài đặt Windows › Mạng.

Ghostline có thể chạy một proxy cục bộ trên một cổng (mặc định `8080`), hiểu được **HTTP, HTTPS (CONNECT) và SOCKS4/4a/5**. Proxy bật và tắt cùng nút **Connect**, và luôn phân giải tên miền qua DNS mã hoá của Ghostline nên không bao giờ rò DNS plain.

| Cài đặt | Ý nghĩa |
| --- | --- |
| bật proxy | Chạy proxy khi đã kết nối |
| dùng cho máy này | Đặt System Proxy của Windows trỏ vào Ghostline. Cài đặt cũ được lưu lại trước và trả về khi ngắt, khi app crash hay mất điện. Nếu app khác (VPN, proxy công ty) đã đặt proxy, Ghostline hỏi trước khi ghi đè, và không bao giờ giành lại khi app khác đổi nó sau đó |
| chia sẻ LAN | Cho điện thoại và thiết bị khác cùng Wi-Fi dùng proxy. Chỉ nhận địa chỉ mạng riêng, luật tường lửa `Ghostline Proxy` chỉ áp dụng cho mạng *Private* |
| cổng | 1024–65535 |

**Trên điện thoại:** bật *chia sẻ LAN*, rồi trên điện thoại vào Wi-Fi → mạng này → Proxy → Thủ công, nhập địa chỉ hiện trên trang (hoặc quét mã QR). Nếu trang báo mạng đang là *Public*, đổi sang *Private* trong Cài đặt Windows → Mạng.

**Fragment web** cắt nhỏ ClientHello của TLS để DPI không đọc được tên trang:

- **tự động khi bị chặn** (mặc định): kết nối bình thường; nếu bị reset hoặc treo trước khi server trả lời thì thử lại một lần có fragment và ghi nhớ trang đó cho mạng này (7 ngày). Lần đầu vào một trang bị chặn có thể chậm thêm tối đa 3 giây.
- **luôn bật** / **tắt**.
- **kiểu cắt:** TCP (cắt quanh SNI), TLS record (cắt thành nhiều bản ghi TLS), hoặc kết hợp (mặc định).
- Danh sách **domain đã ghi nhớ** cho thấy những gì đã học được ở mạng này; xoá mục nào nếu trang đó đã vào được mà không cần giúp.

Nếu thống kê có kết nối *bị chặn dù đã fragment*, mạng đó cần dùng GoodbyeDPI.

**Upstream proxy** (SOCKS5 hoặc HTTP, có thể kèm user/mật khẩu) cho phép rules đẩy một số trang qua proxy khác, ví dụ Tor. Mật khẩu được mã hoá bằng DPAPI của Windows. Bấm **kiểm tra** để thử.

### 4.7. Rules và danh sách

![Rules và danh sách](screenshots/rules-vi.png)

<a href="videos/clips/rules-vi.mp4"><img src="videos/clips/rules-vi.webp" width="640" alt="Thêm rule và thử tên miền"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

**Từng bước: chặn một tên miền**

1. Ở **Rules**, gõ mẫu như `ads.example.com` vào ô mẫu.
2. Chọn hành động **chặn**, rồi bấm **+ thêm rule**.
3. Gõ tên miền vào ô **thử tên miền** và bấm **thử**: Ghostline cho biết rule hay danh sách nào quyết định.
4. Muốn xoá rule, bấm **✕** của nó.

Rules quyết định cách xử lý một tên miền, cho cả DNS lẫn proxy. Rule đầu tiên khớp được dùng; nếu không rule nào khớp thì xét các danh sách theo thứ tự.

| Mẫu | Khớp với |
| --- | --- |
| `example.com` | example.com và mọi subdomain |
| `=example.com` | chỉ example.com |
| `*.example.com` | chỉ các subdomain |
| `~ads` | mọi tên có chứa "ads" |
| `/^ad[0-9]+\./` | biểu thức chính quy (RE2) |
| `10.0.0.0/8` | dải IP (chỉ áp dụng ở proxy) |

| Hành động | Tác dụng |
| --- | --- |
| `block` | DNS trả 0.0.0.0 (hoặc NXDOMAIN, xem *chặn ở DNS trả về*); proxy từ chối |
| `allow` | đi thẳng và bỏ qua các danh sách, dùng để chữa chặn nhầm |
| `ip=1.2.3.4` | DNS giả (lặp lại cho IPv6) |
| `fragment=auto\|on\|off` | ghi đè fragment web cho trang này |
| `upstream=<id>` | đi qua upstream proxy |

Sửa rules ở tab **bảng** hoặc chuyển sang **text** (mỗi dòng một rule, `#` là chú thích, `#!` là rule đang tắt). Không có gì được lưu cho tới khi mọi dòng hợp lệ; dòng sai được đánh dấu kèm số dòng.

**Danh sách:** dán link GitHub bất kỳ dạng nào (blob, raw, gist, jsDelivr) hoặc đường dẫn file trên máy, chọn hành động, rồi bấm **+ thêm danh sách**. Ghostline tự nhận diện định dạng (hosts, domain, AdBlock/AdGuard, dnsmasq, Unbound, RPZ, Clash/Surge, v2ray domain-list-community, sing-box JSON, CIDR), cho biết đọc được bao nhiêu mục và bỏ qua dòng nào, và tự cập nhật mỗi 24 giờ. **Thêm nhanh** gợi ý các danh sách phổ biến kèm license và repo gốc. Nếu GitHub bị chặn, Ghostline tự chuyển sang jsDelivr.

**Thử tên miền** cho biết rule hay danh sách nào quyết định một tên, ví dụ *chặn — danh sách HaGeZi Light, dòng 120*.

### 4.8. DNS server

![DNS server](screenshots/dnsserver-vi.png)

<a href="videos/clips/dnsserver-vi.mp4"><img src="videos/clips/dnsserver-vi.webp" width="640" alt="Chia sẻ DNS server và mở trang cài đặt cho điện thoại"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

Chia sẻ DNS mã hoá của Ghostline cho trình duyệt trên máy này và cho thiết bị khác trong mạng nhà. DNS server chạy khi bạn đang kết nối.

- **DoH cục bộ:** trình duyệt trên máy này có thể dùng `https://127.0.0.1/dns-query` làm DNS an toàn tuỳ chỉnh.
- **Chia sẻ cho LAN:** trả lời thêm DNS cổng 53 và DoH trên IP LAN của máy. Chỉ thiết bị trong mạng **Private** dùng được; truy vấn từ nơi khác bị từ chối.
- **Cổng DoH:** mặc định 443; đổi nếu chương trình khác đang dùng cổng này.

**Dùng cho thiết bị khác** (DNS cổng 53 không cần chứng chỉ):

| Thiết bị | Cách làm |
| --- | --- |
| Router, TV, máy chơi game | đặt DNS là IP của máy tính |
| Android | tắt *DNS riêng tư*, rồi đặt DNS tĩnh cho Wi-Fi nhà |
| Steam Deck | đặt DNS thủ công cho riêng Wi-Fi nhà (không đặt cho mọi mạng) |
| iPhone / iPad | *Cài đặt › Wi-Fi › (i) › Định cấu hình DNS › Thủ công* với IP máy tính — hoặc cài profile DoH bên dưới |

**Từng bước: profile DoH cho iPhone**

1. Bấm **Kết nối**, rồi ở **DNS server** bật **bật DoH cục bộ** và **chia sẻ cho LAN** (mạng phải là *Private*).
2. Chọn hoặc nhập **tên Wi-Fi nhà**, đúng như trên điện thoại. Ghostline liệt kê các Wi-Fi máy biết; máy cắm dây có thể không có, và điện thoại cũng nhập được trên trang.
3. Bấm **mở trang cài đặt cho điện thoại** và quét mã QR bằng iPhone. Trang mở trong 10 phút.
4. Trên điện thoại, so vân tay với vân tay trong Ghostline, bấm **Tải profile** rồi cài trong *Cài đặt › Đã tải về hồ sơ*.
5. **Bắt buộc:** mở *Cài đặt › Cài đặt chung › Giới thiệu › Cài đặt tin cậy chứng chỉ* và bật **Ghostline LAN CA** (Tin cậy hoàn toàn). Thiếu bước này, iPhone mất mạng ở Wi-Fi nhà.
6. Kiểm tra *Cài đặt › Cài đặt chung › VPN và quản lý thiết bị › DNS* đang chọn **Ghostline DNS**.

DNS mã hoá chỉ dùng khi ở Wi-Fi nhà; khi dùng 4G/5G hay mạng khác, iPhone dùng DNS như bình thường. **lưu file…** ghi chứng chỉ và profile ra đĩa thay vì mở trang.

<p align="center"><img src="screenshots/setuppage-vi.png" width="300" alt="Trang cài đặt trên điện thoại"></p>

**Khi máy tính tắt hoặc ngắt kết nối,** mọi thiết bị dùng DNS của máy sẽ mất mạng ở mạng nhà. iOS không tự chuyển sang DNS khác. Để iPhone có mạng lại, mở *Cài đặt › Cài đặt chung › VPN và quản lý thiết bị › DNS* và chọn **Tự động** (hoặc xoá profile). Nếu máy tính hay tắt, hãy dùng cách đặt DNS thủ công ở trên thay cho profile: không cần chứng chỉ và đổi lại rất nhanh. Khi có thiết bị trong mạng dùng DNS server trong 10 phút gần nhất, nút **Ngắt kết nối** (trong app và trên khay) sẽ hỏi lại trước; tắt Windows và **Thoát** thì không hỏi.

**CA LAN:** chứng chỉ mà thiết bị khác tin. Nó chỉ ký được cho IP nội bộ và `*.ghostline.lan`, nên không thể dùng để giả mạo trang web. **tạo lại** sinh CA mới (thiết bị phải cài lại); **gỡ** xoá CA và tắt DNS server.

### 4.9. Fake SNI

![Fake SNI](screenshots/fakesni-vi.png)

<a href="videos/clips/fakesni-vi.mp4"><img src="videos/clips/fakesni-vi.webp" width="640" alt="Bật Fake SNI"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

**Từng bước: bật Fake SNI**

1. Bấm **Kết nối**. Ở **Proxy**, bật **bật proxy** và **dùng cho máy này**.
2. Mở **proxy › fake sni**, đọc và cuộn cảnh báo tới cuối, tích **tôi đã hiểu**, rồi bấm **tiếp tục**.
3. Bật **bật Fake SNI**, rồi bật một **nhóm preset** hoặc viết rule `sni=` ở trang **Rules**.
4. Mở trang đó trong trình duyệt. Bộ đếm cho biết số kết nối đã giải mã hoặc phải quay về fragment.
5. Xong thì bấm **tắt Fake SNI** trên dải tím.

Tính năng nâng cao cho trang nằm sau CDN cho phép *domain fronting*. Proxy giải mã HTTPS của trình duyệt cho những tên miền bạn chọn và kết nối tới máy chủ bằng một tên khác được phép, nên nhà mạng thấy tên đó thay vì trang thật.

- Lần đầu phải đọc hết cảnh báo và xác nhận.
- Cần bật **proxy** và **dùng cho máy này** (chỉ áp dụng cho trình duyệt trên máy này).
- Bật một **nhóm preset** hoặc viết rule như `youtube.com sni=www.google.com connect=www.google.com`. `sni=none` không gửi tên nào. `connect=` chọn host để lấy địa chỉ kết nối.
- Khi đang chạy, mọi trang có banner tím cho biết đang giải mã bao nhiêu tên miền. **tắt Fake SNI** dừng ngay.
- Nếu máy chủ từ chối tên giả, Ghostline tự quay về fragment; bộ đếm trên trang cho thấy điều này.
- Chứng chỉ được cài chỉ tồn tại khi đang kết nối, chỉ ký được cho các tên miền trong rule, và bị gỡ khi ngắt kết nối, khi app bị tắt đột ngột (watchdog gỡ) và khi gỡ cài đặt.
- Không dùng cho ngân hàng hay tài khoản quan trọng; app ghim chứng chỉ sẽ lỗi với các tên miền này. Firefox có thể cần bật `security.enterprise_roots.enabled` trong `about:config`.

Danh sách từ nguồn khác chỉ được dùng rule `sni=` sau khi bạn bật **tin cho Fake SNI**; preset của Ghostline có chữ ký.


### 4.10. Công cụ

Trang công cụ gồm nhật ký (tab đầu tiên, [phần 4.4](#44-nhật-ký)) và bốn công cụ chẩn đoán.

<a href="videos/clips/tools-vi.mp4"><img src="videos/clips/tools-vi.webp" width="640" alt="Lookup, Scanner, IP Cloudflare và Stamp"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

| Lookup | IP Cloudflare |
| --- | --- |
| ![Lookup](screenshots/lookup-vi.png) | ![IP Cloudflare](screenshots/cfscan-vi.png) |
| **Scanner** | **Stamp** |
| ![Scanner](screenshots/scanner-vi.png) | ![Stamp](screenshots/stamp-vi.png) |

**Lookup** tra một tên miền qua nhiều nguồn cùng lúc và cho biết các câu trả lời có khớp nhau không.

1. Gõ tên miền (dán link từ trình duyệt cũng được) và chọn loại bản ghi. Với **PTR**, gõ một IP.
2. Tích các nguồn. Mặc định có Ghostline (khi đã kết nối) và các server nhanh nhất từ lần quét gần nhất.
3. Bấm **tra**. Thẻ ở trên cho biết:
   - **Bị đầu độc DNS:** một nguồn trả về IP nội bộ, hoặc báo tên miền không tồn tại trong khi nguồn khác tìm thấy. Đó là cách nhà mạng chặn bằng DNS.
   - **Kết quả khác nhau:** địa chỉ khác nhau. CDN trả IP theo vị trí, nên chỉ vậy thì chưa chắc là bị chặn.
   - **Kết quả khớp nhau:** cùng địa chỉ, hoặc cùng một CDN.
4. **chi tiết** hiện từng câu trả lời như `dig` in ra, có TTL và các cờ.

Nguồn **DNS nhà mạng** là chỗ duy nhất Ghostline gửi truy vấn không mã hoá: nhà mạng thấy tên miền bạn tra. Ghostline không bao giờ tích sẵn nguồn này; chỉ tích khi cần so sánh. Khi máy nhận DNS từ router, Ghostline đưa ra địa chỉ của router.

**Scanner** chấm nhiều server cùng lúc: độ trễ qua nhiều lượt (trung vị, p90, jitter), tỉ lệ mất gói, server có kiểm tra DNSSEC không, có lọc quảng cáo không, và có trả địa chỉ giả cho các trang mẫu ở trang **Vượt DPI** không. Quét danh sách server có lọc, hoặc dán danh sách địa chỉ. Mặc định một lượt quét tối đa 500 server (chỉnh 50–2000 trong **tuỳ chọn**); bộ lọc khớp nhiều hơn thì quét các server hữu ích nhất trước: đã ghim, chạy tốt lần trước, rồi server có sẵn. Quét càng nhiều càng lâu: 2000 server mất khoảng 6–8 phút. Từ kết quả có thể ghim server, chỉ dùng một server, thêm server vừa dán vào danh sách, hoặc xuất CSV mở được bằng Excel.

**IP Cloudflare** tìm các địa chỉ Cloudflare còn dùng được và nhanh trên mạng của bạn.

1. Bấm **quét**. Ghostline thử một địa chỉ trong mỗi khối mạng của Cloudflare, chỉ qua cổng 443, tối đa 200 kết nối mới mỗi giây, đi thẳng ra mạng (không qua proxy của Ghostline). Đủ 50 địa chỉ dùng được thì dừng.
2. 10 địa chỉ nhanh nhất được đo thêm tốc độ tải.
3. Chọn vài địa chỉ rồi **sao chép**, hoặc **tạo rule**: gõ các domain (ví dụ `example.com` và `*.example.com`), Ghostline thêm rule `ip=` vào trang **Rules**.

Rule `ip=` chỉ có tác dụng với ứng dụng dùng DNS hoặc proxy của Ghostline, và chỉ đúng với domain thật sự nằm sau Cloudflare. Kết quả được lưu theo từng mạng; **kiểm tra lại** thử lại các địa chỉ đang chọn. Địa chỉ dùng được hôm nay có thể bị chặn ngày mai: quét lại khi trang không vào được nữa.

**Stamp** đọc và tạo stamp `sdns://`. Dán stamp để xem bên trong có gì, hoặc điền biểu mẫu (hay **điền từ URL**) để tạo stamp, rồi **thêm vào danh sách server**. Stamp relay và ODoH đọc được nhưng Ghostline không dùng.

### 4.11. Sao lưu và chuyển máy

Trong **Cài đặt › Sao lưu và chuyển máy**:

<a href="videos/clips/backup-vi.mp4"><img src="videos/clips/backup-vi.webp" width="640" alt="Tên miền thử và xuất cài đặt"></a>

*Ảnh động được tua nhanh, không có tiếng; bấm vào để xem video có lồng tiếng.*

- **xuất cài đặt…** lưu một file `.ghostline.json`. Chọn phần cần xuất: cài đặt, rule và danh sách, server tự thêm, danh sách đen vượt DPI, danh sách tự học của zapret2. File không bao giờ chứa tên Wi-Fi nhà, card mạng, mật khẩu proxy, nhật ký hay chứng chỉ. Danh sách trỏ tới một file trên máy này không được xuất.
- **nhập cài đặt…** chỉ dùng được khi đã ngắt kết nối. Ghostline cho xem trước những gì sẽ thay đổi, và hỏi riêng trước khi nhập rule chuyển hướng lưu lượng đã giải mã (`sni=`, `connect=`). Khi nhập, Fake SNI, DNS server, chia sẻ trong LAN và khởi động cùng Windows luôn bị tắt, và danh sách từ nguồn khác không còn được tin cho Fake SNI: bật lại trên máy này nếu cần. Mật khẩu proxy phải nhập lại.
- Nếu ghi lỗi giữa chừng, mọi file được trả về như cũ. File bị thay khi nhập được giữ lại bên cạnh với tên `*.bak-import`.

`Ghostline.exe --export <file>` ghi cùng bản sao lưu đó từ dòng lệnh, để gửi cho người đang giúp bạn. Chạy trong Command Prompt hoặc PowerShell để thấy kết quả; giống như khi mở Ghostline, Windows sẽ hỏi quyền quản trị.

## 5. Icon ở khay hệ thống

Ghostline có icon hình vòng tròn ở khay (góc dưới bên phải, cạnh đồng hồ). Màu icon cho biết trạng thái. **Bấm chuột phải** để mở menu:

- **Kết nối / Ngắt kết nối** (hỏi lại khi có thiết bị trong mạng đang dùng DNS của máy)
- **Vượt DPI:** bật/tắt GoodbyeDPI nhanh
- **Proxy: bật/tắt:** bật hoặc tắt proxy cục bộ
- **Mở Ghostline:** hiện lại cửa sổ
- **Thoát:** ngắt kết nối, trả DNS về như cũ, rồi tắt app

## 6. Khi một trang web vẫn bị chặn

> ⚖️ Bạn tự chịu trách nhiệm tuân thủ pháp luật và điều khoản của nhà mạng. Không dùng các tính năng này để truy cập nội dung bị cấm theo quy định của pháp luật. Xem [Tuyên bố miễn trừ trách nhiệm](../README.vi.md#tuyên-bố-miễn-trừ-trách-nhiệm).

Làm lần lượt, dừng lại khi trang đã mở được:

1. **Kết nối Ghostline.** Nhiều trang chỉ bị chặn bằng DNS, nên kết nối là đủ.
2. **Xoá cache trình duyệt** hoặc mở thử bằng cửa sổ ẩn danh (trình duyệt có thể còn nhớ kết quả DNS cũ).
3. Vào **Vượt DPI**, bật **GoodbyeDPI** với preset **Nhẹ**.
4. Bấm **⚡ tự dò** để Ghostline tự tìm preset phù hợp. Thêm trang bạn cần vào **Trang mẫu** trước để tự dò kiểm tra đúng trang đó.
5. Vẫn không được: thử **Mode 1–6**.
6. Nếu chỉ vài trang bị chặn, chuyển **Phạm vi** sang **danh sách đen** và thêm các trang đó, để GoodbyeDPI không ảnh hưởng tới phần còn lại.

> **Lưu ý về trình duyệt:** Chrome, Edge và Firefox có tuỳ chọn *Secure DNS / DNS over HTTPS* riêng. Nếu bật, trình duyệt sẽ bỏ qua Ghostline. Hãy tắt tuỳ chọn đó, hoặc để ở chế độ "dùng DNS của hệ thống".

## 7. Xử lý sự cố

| Thông báo | Nguyên nhân và cách xử lý |
| --- | --- |
| **Ghostline cần quyền quản trị (admin) để đổi DNS** | Bạn đã mở app không có quyền admin. Đóng lại, chuột phải → **Run as administrator** |
| **Cổng 53 đang bị … chiếm** | Một chương trình khác đang chạy DNS trên 127.0.0.1 (thường là WSL, Hyper-V hoặc một phần mềm DNS khác; Mobile Hotspot không còn gây lỗi này). Tắt chương trình đó, hoặc dùng nút **Tạm dừng dịch vụ …** mà Ghostline đưa ra. Ghostline luôn hỏi trước khi dừng dịch vụ nào |
| **Không tìm được máy chủ hoạt động** | Mạng đang mất kết nối, hoặc nhà mạng chặn cả DNS mã hoá. Kiểm tra mạng, rồi thử bật **Fragment DNS** |
| **Truy vấn DNS không đi qua Ghostline** | Có VPN hoặc phần mềm khác đang chiếm DNS. Nếu dưới lỗi có dòng **card mạng có DNS riêng**, đó là card mạng Windows vẫn hỏi DNS qua đường khác (VPN, PPPoE, USB 4G...): tắt nó hoặc chọn nó trong Cài đặt → card mạng, rồi kết nối lại |
| **Không đặt được DNS cho …** | Card mạng đó không cho đổi DNS (thường là card ảo của VPN/máy ảo). Vào **Cài đặt → card mạng → chọn tay** và bỏ card đó ra |
| **Không trả được DNS về như cũ cho …** | Bấm **⚠ KHÔI PHỤC DNS NGAY**. Thông báo này sẽ còn hiện cho tới khi khôi phục thành công |
| **GoodbyeDPI không chạy được** | Thử preset khác. Xem thêm chi tiết trong ngoặc của thông báo |
| **GoodbyeDPI bị phần mềm diệt virus chặn** | Thêm thư mục Ghostline vào danh sách loại trừ của antivirus |
| **File GoodbyeDPI đã bị thay đổi** | File GoodbyeDPI không còn khớp mã băm gốc (có thể bị antivirus sửa hoặc bị can thiệp). Cài lại Ghostline |
| **Không tìm được cấu hình vượt DPI phù hợp** | Tự dò không tìm được preset mở được mọi trang mẫu. Thử Mode 1–6 hoặc tham số tự nhập |
| **Cần kết nối trước khi tự dò vượt DPI** | Bấm Kết nối rồi tự dò lại |

**Mất mạng sau khi dùng Ghostline?** Gần như không thể xảy ra vì có 5 lớp khôi phục. Kể cả khi antivirus kill Ghostline và xoá `ghostline.exe`, tác vụ `Ghostline Network Guard` vẫn tự trả DNS về như cũ trong khoảng 1 phút, và chạy lại khi khởi động máy. Nếu vẫn gặp:

1. Mở Ghostline → **Cài đặt → ⚠ KHÔI PHỤC DNS NGAY**.
2. Hoặc mở PowerShell bằng quyền admin và chạy:
   ```powershell
   & "C:\Program Files\Ghostline\Ghostline\ghostline.exe" --restore
   ```
   (với bản portable, thay bằng đường dẫn tới `ghostline.exe` của bạn).
3. Cách cuối cùng: vào **Settings → Network & internet → card mạng → DNS server assignment → Edit → Automatic (DHCP)**.

**Báo lỗi:** vào **Nhật ký → lưu file**, rồi mở issue tại [GitHub](https://github.com/hashcott/ghostline/issues) kèm file đó. Nhật ký không chứa tên các trang bạn đã truy cập.

**Gỡ chứng chỉ Ghostline bằng tay:** trong app, vào **Cài đặt → chứng chỉ → gỡ tất cả chứng chỉ Ghostline**. Không có app thì chạy `certlm.msc`, mở *Trusted Root Certification Authorities → Certificates* và xoá các mục bắt đầu bằng `Ghostline`.

## 8. Gỡ cài đặt

- **Bản cài đặt:** Settings → Apps → Ghostline → Uninstall. Trình gỡ cài đặt tự trả DNS về như cũ, xoá các tác vụ khởi động, driver WinDivert và mọi chứng chỉ Ghostline.
- **Bản portable:** trong app bấm **Ngắt kết nối**, tắt **khởi động cùng Windows**, thoát từ khay, rồi xoá thư mục.
- **Linux:** `sudo apt remove ghostline` (`apt purge` xoá luôn cài đặt), `sudo dnf remove ghostline`, `sudo pacman -R ghostline-bin`; AppImage và tar.gz: `sudo ./uninstall.sh [--purge]` trong thư mục đã giải nén, hoặc `sudo /var/lib/ghostline/bin/ghostlined --uninstall-system [--purge]`. DNS, proxy hệ thống, chứng chỉ (cả trong profile Firefox), luật firewall và bảng nftables được trả lại trước. rpm và Arch giữ lại `/var/lib/ghostline` và `/var/log/ghostline`; xoá hai thư mục này để gỡ sạch.

## 9. Câu hỏi thường gặp

**Ghostline có phải VPN không?**
Không. Ghostline chỉ mã hoá **DNS** (câu hỏi "trang này ở đâu?"), không đổi địa chỉ IP của bạn và không mã hoá nội dung bạn truy cập. Nếu bạn cần ẩn IP, hãy dùng VPN; lưu ý VPN và Ghostline thường không chạy cùng lúc được.

**Ghostline có làm chậm mạng không?**
Thường là không. Ghostline gửi truy vấn tới nhiều máy chủ cùng lúc, lấy câu trả lời nhanh nhất, và có bộ nhớ đệm. GoodbyeDPI ở preset cao có thể làm vài trang chậm hơn một chút.

**Ghostline có thu thập dữ liệu của tôi không?**
Không. Không telemetry, không tài khoản, không ghi tên trang bạn truy cập xuống đĩa. Mã nguồn mở, bạn có thể tự kiểm tra.

**Tắt máy khi đang kết nối thì sao?**
Không sao. Ghostline trả DNS về trước khi Windows tắt. Nếu máy mất điện đột ngột, lần đăng nhập sau tác vụ *Ghostline Recovery* sẽ tự khôi phục DNS, kể cả khi bạn không mở Ghostline.

**Dùng chung với Mobile Hotspot được không?**
Được. Mobile Hotspot nghe cổng 53 trên mọi địa chỉ, nhưng Windows vẫn cho Ghostline dùng 127.0.0.1:53, nên bạn kết nối được ngay cả khi đang bật hotspot.
