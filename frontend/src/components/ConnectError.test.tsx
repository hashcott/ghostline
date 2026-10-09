import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { initI18n } from "../i18n";
import { useGhost } from "../app/store";
import { ConnectError } from "./ConnectError";

const unreachable = {
  status: "error", step: 0, warnings: [], servers: [], since: "", latencyMs: 0, queries: 0,
  dpi: { enabled: false, running: false, preset: "light" }, blockedSites: [],
  error: { code: "DAEMON_UNREACHABLE" },
};

const svc = vi.hoisted(() => ({
  Connect: vi.fn(() => Promise.resolve()),
  GetSnapshot: vi.fn(() => Promise.resolve(null as unknown)),
  SaveSettings: vi.fn(() => Promise.resolve()),
  ServiceInstall: vi.fn(() => Promise.resolve({ kind: "", unit: false, steamos: false })),
  InstallService: vi.fn(() => Promise.resolve()),
  StartService: vi.fn(() => Promise.resolve()),
}));
vi.mock("../app/api", () => ({ Service: svc }));

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  svc.Connect.mockClear();
  svc.InstallService.mockReset();
  svc.InstallService.mockImplementation(() => Promise.resolve());
  svc.StartService.mockReset();
  svc.StartService.mockImplementation(() => Promise.resolve());
  svc.ServiceInstall.mockImplementation(() => Promise.resolve({ kind: "", unit: false, steamos: false }));
  svc.GetSnapshot.mockClear();
  svc.GetSnapshot.mockImplementation(() => Promise.resolve(unreachable));
  useGhost.getState().setSnapshot(unreachable as any);
});

test("daemon unreachable shows its message and how to start the service", () => {
  render(<ConnectError onOpenServers={() => {}} onOpenLogs={() => {}} />);
  expect(screen.getByText("Dịch vụ nền của Ghostline chưa chạy")).toBeTruthy();
  expect(screen.getByText("Khởi động bằng lệnh: sudo systemctl start ghostline")).toBeTruthy();
});

// Everything the window loaded at start (settings, logs, app info) came
// from an unreachable daemon too: retry reloads the whole window.
test("retry reloads the window instead of connecting", () => {
  const reload = vi.fn();
  const real = window.location;
  Object.defineProperty(window, "location", { configurable: true, value: { ...real, reload } });
  try {
    render(<ConnectError onOpenServers={() => {}} onOpenLogs={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "thử lại" }));
    expect(reload).toHaveBeenCalledTimes(1);
    expect(svc.Connect).not.toHaveBeenCalled();
  } finally {
    Object.defineProperty(window, "location", { configurable: true, value: real });
  }
});

function withInstall(info: { kind: string; unit: boolean; steamos: boolean }, code = "DAEMON_UNREACHABLE") {
  svc.ServiceInstall.mockImplementation(() => Promise.resolve(info));
  useGhost.getState().setSnapshot({ ...unreachable, error: { code } } as any);
  render(<ConnectError onOpenServers={() => {}} onOpenLogs={() => {}} />);
}

function stubReload() {
  const reload = vi.fn();
  const real = window.location;
  Object.defineProperty(window, "location", { configurable: true, value: { ...real, reload } });
  return { reload, restore: () => Object.defineProperty(window, "location", { configurable: true, value: real }) };
}

test("a package with its service stopped offers to start it", async () => {
  const { reload, restore } = stubReload();
  try {
    withInstall({ kind: "package", unit: true, steamos: false });
    fireEvent.click(await screen.findByRole("button", { name: "khởi động dịch vụ" }));
    await waitFor(() => expect(reload).toHaveBeenCalled());
    expect(svc.StartService).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button", { name: "cài dịch vụ" })).toBeNull();
  } finally {
    restore();
  }
});

test("an AppImage without a service offers to install it", async () => {
  withInstall({ kind: "appimage", unit: false, steamos: false });
  fireEvent.click(await screen.findByRole("button", { name: "cài dịch vụ" }));
  await waitFor(() => expect(svc.InstallService).toHaveBeenCalledTimes(1));
});

test("an installed AppImage or tar.gz service is started, not installed again", async () => {
  for (const kind of ["appimage", "tarball"]) {
    cleanup();
    withInstall({ kind, unit: true, steamos: false });
    expect(await screen.findByRole("button", { name: "khởi động dịch vụ" })).toBeTruthy();
  }
});

test("a protocol mismatch updates a self-installed service, or points to the package manager", async () => {
  withInstall({ kind: "appimage", unit: true, steamos: false }, "DAEMON_PROTOCOL_MISMATCH");
  expect(await screen.findByRole("button", { name: "cập nhật dịch vụ" })).toBeTruthy();
  cleanup();
  withInstall({ kind: "package", unit: true, steamos: false }, "DAEMON_PROTOCOL_MISMATCH");
  expect(await screen.findByText("Cập nhật Ghostline bằng trình quản lý gói")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "cập nhật dịch vụ" })).toBeNull();
});

test("on a Steam Deck the deck user is reminded to set a password", async () => {
  withInstall({ kind: "appimage", unit: false, steamos: true });
  expect(await screen.findByText("Trên Steam Deck, đặt mật khẩu trước: mở Konsole và chạy passwd")).toBeTruthy();
});

test("a failed pkexec shows what to run in a terminal and keeps the buttons", async () => {
  svc.InstallService.mockImplementation(() => Promise.reject(new Error("PKEXEC_FAILED: sudo /run/user/1000/ghostline-install/ghostlined --install-system")));
  withInstall({ kind: "appimage", unit: false, steamos: false });
  fireEvent.click(await screen.findByRole("button", { name: "cài dịch vụ" }));
  expect(await screen.findByText("Không chạy được với quyền quản trị; chạy lệnh này trong terminal (sudo /run/user/1000/ghostline-install/ghostlined --install-system)")).toBeTruthy();
  expect(screen.getByRole("button", { name: "cài dịch vụ" })).toBeTruthy();
});

test("Windows (no install kind) shows only retry", async () => {
  withInstall({ kind: "", unit: false, steamos: false });
  await waitFor(() => expect(svc.ServiceInstall).toHaveBeenCalled());
  expect(screen.getAllByRole("button").map((b) => b.textContent)).toEqual(["thử lại"]);
});

test("DNS intercepted by a known program names it and says how to fix it", () => {
  useGhost.getState().setSnapshot({ ...unreachable, error: { code: "DNS_INTERCEPTED", params: { name: "AdGuard", hint: "adguard" } } } as any);
  render(<ConnectError onOpenServers={() => {}} onOpenLogs={() => {}} />);
  expect(screen.getByText("Có vẻ AdGuard đang chặn DNS của máy nên Ghostline không nhận được truy vấn")).toBeTruthy();
  expect(screen.getByText("Mở AdGuard → Cài đặt → Bảo vệ DNS → tắt, rồi bấm Thử lại")).toBeTruthy();
  expect(screen.getByText("thử lại")).toBeTruthy();
});

test("DNS intercepted by an unknown program still explains the cause", () => {
  useGhost.getState().setSnapshot({ ...unreachable, error: { code: "DNS_INTERCEPTED", params: { name: "", hint: "" } } } as any);
  render(<ConnectError onOpenServers={() => {}} onOpenLogs={() => {}} />);
  expect(screen.getByText(/trình chặn quảng cáo, antivirus hoặc VPN/)).toBeTruthy();
  expect(screen.getByText(/Tắt tính năng lọc DNS/)).toBeTruthy();
});
