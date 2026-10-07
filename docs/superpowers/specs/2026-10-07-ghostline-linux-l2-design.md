# Ghostline — Linux L2 (Daemon + GUI client): Thiết kế

- **Ngày:** 2026-10-07
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Sub-spec L2 của [Hỗ trợ Linux](2026-10-07-ghostline-linux-design.md): daemon `ghostlined` chạy root, GUI Linux là client qua Unix socket, phần lõi tách khỏi Wails để Windows và Linux dùng chung.
- **Dựa trên:** [spec tổng Linux](2026-10-07-ghostline-linux-design.md) (§4, §5, §21) và L1 (nhánh `feat/linux`, plan `2026-10-07-ghostline-linux-l1.md`). Spec này chỉ ghi phần chi tiết thêm hoặc khác spec tổng; mọi thứ không nhắc lại ở đây giữ nguyên.

---

## 1. Mục tiêu

- GUI Linux chạy bằng user thường và điều khiển daemon root. Đóng GUI thì daemon vẫn chạy.
- Logic chỉ có một bản: Windows (trong một tiến trình) và daemon Linux dùng cùng `internal/core`.
- Frontend và các binding TS không đổi một dòng nào.
- Windows không đổi hành vi.

### Tiêu chí thành công

1. `sudo ./bin/ghostlined --daemon` rồi `./bin/ghostline`: GUI hiện đầy đủ giao diện, đọc và lưu được cài đặt, nhận event (trạng thái, log, thống kê) từ daemon.
2. Bấm Connect trên Linux **không** còn báo NOT_ADMIN. Nó dừng ở bước DNS (backend DNS vẫn là stub đến L3) với lỗi đổi DNS hiện có, và hệ thống không bị đổi gì.
3. Đóng GUI rồi mở lại: daemon vẫn chạy, GUI mới thấy đúng trạng thái hiện tại.
4. User không thuộc `wheel`/`sudo`/`admin`/`ghostline` kết nối socket thì nhận `NOT_AUTHORIZED`. Daemon chưa chạy thì GUI hiện trang `DAEMON_UNREACHABLE`.
5. Xuất backup trên Linux: GUI hiện hộp thoại lưu file, file thuộc về user (không phải root). Nhập backup: GUI đọc file, daemon không bao giờ mở đường dẫn của user.
6. `CGO_ENABLED=0 go build ./cmd/ghostlined` chạy được. depcheck chứng minh `ghostlined` không kéo theo Wails hay `x/sys/windows`.
7. CI xanh cả hai job. Test `shell` cũ vẫn qua (đã chuyển theo sang `core` khi cần). Exe Windows không tăng quá 2%.

## 2. Phạm vi

### Có

| Nhóm | Nội dung |
|---|---|
| Lõi dùng chung | `internal/core`: dựng orchestrator, `app.Service`, Bus, khôi phục lúc khởi động, các vòng lặp nền. Không import Wails |
| RPC | `internal/rpc`: giao thức, server, client, xác thực peer, lời gọi ngược `ui` |
| Proxy sinh tự động | `tools/genrpc` → `internal/rpc/client/service_gen.go`, đăng ký ID binding trùng `app.Service` |
| Daemon | `cmd/ghostlined`: `--daemon`, `--socket`, `--restore`, `--remove-certs`, `--export`, `status`, `connect`, `disconnect` |
| GUI Linux | `shell` ở chế độ client: bind proxy, phát lại event, trang lỗi daemon, tray dùng method của `Service` |
| `procs` Linux | `IsAdmin` (`geteuid`), `Alive`/`StartTime`/`WaitForExit` (`/proc`); các phần khác vẫn stub |
| Đường dẫn Linux | Daemon: `/var/lib/ghostline/data`, `/var/log/ghostline`, `/run/ghostline` |
| systemd | `build/linux/ghostline.service` (L5 dùng lại khi đóng gói) |
| Mã lỗi | `DAEMON_UNREACHABLE`, `DAEMON_PROTOCOL_MISMATCH`, `NOT_AUTHORIZED`, `NO_UI` |

### Không có (dời sang sau)

- Session-agent (spec tổng §5.5) → **L4**, nơi đầu tiên cần đến nó (proxy hệ thống, NSS).
- Theo dõi ngủ/thức `PrepareForSleep` → L3, như spec tổng.
- Nút `pkexec` "Khởi động dịch vụ"/"Cài dịch vụ" trên trang lỗi → L5, cùng phần cài đặt.
- Backend DNS, DPI, proxy, chứng chỉ, firewall Linux → L3, L4.

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Tách lõi | `internal/core` nhận `platform.Deps` và các tuỳ chọn, trả về `*Core` (orchestrator, service, bus). `Run(ctx)` chạy các vòng lặp nền |
| ID binding | Proxy gọi `application.RegisterBindingMethodID((*client.Service).M, fnv32a("github.com/hashcott/ghostline/internal/app.Service.M"))` cho mọi method. Đã kiểm chứng trong Wails v3 beta.27 (`pkg/application/bindings.go`) |
| Đường dẫn binding TS | Trình sinh binding vẫn thấy `application.NewService(svc)` với `svc *app.Service` (nhánh trong một tiến trình), nên đường dẫn module TS giữ nguyên |
| Tray | Chỉ gọi method của `Service` qua interface `trayBackend`; Windows truyền `*app.Service`, Linux truyền proxy |
| Hộp thoại file | Các method cần hộp thoại nhận `ctx context.Context` đầu tiên (Wails bỏ qua tham số này, frontend không đổi). `ServiceDeps.SaveFile(ctx, name, data)` và `ServiceDeps.OpenFile(ctx, title) (name string, data []byte, err error)`: trả **nội dung** chứ không trả đường dẫn. Thay cho `ExportSettingsBytes`/`ImportSettingsBytes` của spec tổng §5.4 |
| Lỗi qua RPC | Giữ nguyên chuỗi `Error()`; với `*app.AppError` giữ cả `code` và `params`. Proxy trả về lỗi có cùng chuỗi và cùng JSON mà Wails sẽ tạo nếu gọi trực tiếp |
| Phân quyền | `SO_PEERCRED`; root hoặc thuộc `wheel`/`sudo`/`admin`/`ghostline`. Cờ `--allow-uid <uid>` chỉ dành cho test và dev (chạy daemon không root trên socket tạm) |
| Dữ liệu GUI Linux | GUI không đọc `/var/lib/ghostline`. Chỉ lưu tuỳ chọn cửa sổ ở `~/.config/ghostline` |
| Tắt daemon | SIGTERM/SIGINT → `Disconnect` sạch (tối đa 10 giây, giống `OnShutdown`) → thoát |

## 4. Kiến trúc

### 4.1 Package

| Package | Vai trò |
|---|---|
| `internal/core` | Phần dựng logic chuyển từ `shell.Run`: `catalog`, `strategyBox`, `proxyWiring`, `certWiring`, `dnsWiring`, `updateChecker`, các hàm `run*`. Chỉ phần cửa sổ và tray ở lại `shell` |
| `internal/rpc` | `proto.go` (kiểu thông điệp, giới hạn dòng), `server.go`, `client.go`, `peer_linux.go` (`SO_PEERCRED`, kiểm tra group), `peer_other.go` (từ chối tất cả) |
| `internal/rpc/client` | `service_gen.go` (sinh tự động) + `service.go` (kết nối, phát lại event, lời gọi ngược `ui`) |
| `tools/genrpc` | Đọc tập method của `app.Service` bằng `go/types`, sinh `service_gen.go` |
| `cmd/ghostlined` | `main` của daemon |
| `internal/shell` | Windows: `core` trong cùng tiến trình. Linux: client RPC. Cửa sổ, tray, trang lỗi daemon |

### 4.2 Luồng dữ liệu

```
Windows:  frontend ──Wails──▶ *app.Service ◀── core (cùng tiến trình)

Linux:    frontend ──Wails──▶ client.Service ──ctl.sock──▶ rpc.Server ──▶ *app.Service ◀── core (ghostlined, root)
                       ▲                                        │
                       └──── wapp.Event.Emit ◀── event ─────────┘
                       GUI hộp thoại ◀── ui:saveFile/openFile ──┘
```

### 4.3 Ranh giới

- `internal/core` và `cmd/ghostlined` không import Wails (depcheck).
- `internal/rpc` không import `internal/app` (nó chuyển JSON); chỉ `rpc/client` (sinh tự động) biết kiểu của `app`.
- Bản Windows không import `internal/rpc`, `internal/rpc/client`, `cmd/ghostlined` (luật depcheck có từ L1).

## 5. Giao thức (`protocol: 1`)

Mỗi thông điệp là một dòng JSON, tối đa **16 MiB** (backup tới 8 MiB đi qua lời gọi ngược ở dạng base64, khoảng 10,7 MiB). Dòng dài hơn → đóng kết nối.

| Hướng | Thông điệp |
|---|---|
| Hai chiều, đầu tiên | `{"hello":{"version":"0.6.0","protocol":1}}` |
| GUI → daemon | `{"id":7,"call":"SaveSettings","args":[…]}` |
| daemon → GUI | `{"id":7,"result":…}` hoặc `{"id":7,"error":{"message":"…","code":"…","params":{…}}}` |
| daemon → mọi GUI | `{"event":"state","data":…}` |
| daemon → GUI đã gọi | `{"uid":3,"ui":"saveFile","args":["ghostline-2026-10-07.ghostline.json","<base64>"]}` |
| GUI → daemon | `{"uid":3,"result":…}` hoặc `{"uid":3,"error":{"message":"…"}}` |

- Lệch `protocol` → bên nhận gửi lỗi `DAEMON_PROTOCOL_MISMATCH` rồi đóng.
- Lời gọi ngược `ui` dùng `ctx` của lời gọi đang chạy để biết kết nối nào. Kết nối đó đã đóng → `NO_UI`.
- Lời gọi ngược có thời hạn 10 phút (người dùng có thể để hộp thoại mở lâu); hết hạn → lỗi, method trả lỗi như khi người dùng huỷ.

## 6. GUI Linux

- Khởi động: kết nối socket, trao `hello`, bind proxy, đăng ký phát lại event, mở cửa sổ.
- Không kết nối được → cửa sổ vẫn mở với trang `DAEMON_UNREACHABLE` (nút Thử lại, hướng dẫn `sudo systemctl start ghostline` hoặc `sudo ./bin/ghostlined --daemon` khi dev).
- Mất kết nối giữa chừng → về trang `DAEMON_UNREACHABLE`; Thử lại thì kết nối lại và gọi `GetSnapshot`.
- Tray: bật/tắt, DPI, proxy, hiện cửa sổ, thoát GUI. Đổi ngôn ngữ và cập nhật nhãn tray dựa trên event `state`/`update`, không gọi hook trong tiến trình.

## 7. Cài đặt và dữ liệu

- `settings.json` giữ v5; `state.json` giữ v3 (v4 ở L3).
- `platform.New` trên Linux dùng `/var/lib/ghostline/data` cho daemon. GUI chỉ cần đường dẫn socket: `platform.ClientSocket()` (`$GHOSTLINE_SOCKET` khi dev). Log và tuỳ chọn của GUI nằm ở `~/.config/ghostline`.
- Khoá `state.json` chuyển sang `/run/ghostline/state.lock`.

## 8. Xử lý lỗi

| Mã | Khi nào | UI |
|---|---|---|
| `DAEMON_UNREACHABLE` | GUI không kết nối được hoặc mất kết nối | Trang lỗi + Thử lại + hướng dẫn |
| `DAEMON_PROTOCOL_MISMATCH` | `protocol` khác nhau | Trang "Cần cập nhật dịch vụ nền" |
| `NOT_AUTHORIZED` | Peer không thuộc nhóm được phép | Trang lỗi + `sudo usermod -aG ghostline $USER` |
| `NO_UI` | Cần hộp thoại khi không có GUI (CLI) | Trả về cho CLI |

Mỗi mã có chuỗi trong `vi.json` và `en.json`.

## 9. Kiểm thử

- **`internal/rpc`:** gọi/trả, event tới nhiều client, lỗi `AppError` (giữ code, params, chuỗi), `hello` lệch, peer bị từ chối (peercred giả), dòng quá 16 MiB, lời gọi ngược `ui` tới đúng client, client đóng giữa lời gọi ngược → `NO_UI`.
- **`tools/genrpc`:** file golden; ID sinh ra bằng đúng `fnv32a` của FQN `app.Service`. CI: `go generate ./... && git diff --exit-code`.
- **`internal/core`:** test của `shell` chuyển theo; thêm test `core.New` với `platform` giả.
- **Integration (không root, CI Linux):** `ghostlined --daemon --socket <tmp> --allow-uid <uid>` + `client.Service` thật: `GetSnapshot`, `SaveSettings` + event `state`, Connect dừng ở bước DNS và trả lỗi, `ExportSettings` gọi ngược `saveFile` đúng client.
- **Windows:** test `shell` và job CI Windows; tray chuyển sang `Service` là thay đổi thật trên Windows.
- **Thủ công (Linux):** tiêu chí 1–5.

## 10. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Tách `core` làm đổi thứ tự khởi động trên Windows | Giữ đúng thứ tự cũ (khôi phục trước, rồi orchestrator, rồi vòng lặp nền); review so từng đoạn; job CI Windows |
| `RegisterBindingMethodID` là API mới của Wails beta | Ghim Wails; test kiểm tra ID của proxy bằng đúng ID trình sinh binding tạo cho `app.Service` |
| Lỗi qua RPC khác lỗi Wails tạo trực tiếp | Test so JSON lỗi từ proxy với JSON Wails tạo cho cùng lỗi |
| Lời gọi ngược bị treo | Thời hạn 10 phút; đóng kết nối huỷ mọi lời gọi ngược đang chờ |
| Socket `0666` bị lạm dụng | Kiểm tra peer trước mọi thông điệp; không có method chạy lệnh tuỳ ý hay nhận đường dẫn file |
