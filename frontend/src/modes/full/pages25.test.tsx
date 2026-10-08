import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Dpi } from "./pages/Dpi";
import { Logs } from "./pages/Logs";
import { Settings } from "./pages/Settings";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";

const svc = vi.hoisted(() => ({
  DPIInfo: vi.fn(() => Promise.resolve({ engines: [{ id: "zapret2", exe: "winws2.exe" }, { id: "goodbyedpi", exe: "goodbyedpi.exe" }], mechanism: "WinDivert", avExclusions: true })),
  ListCerts: vi.fn(() => Promise.resolve([])),
  SaveSettings: vi.fn(() => Promise.resolve()),
  SetDPIEnabled: vi.fn(() => Promise.resolve()),
  StartAutotune: vi.fn(() => Promise.resolve()),
  CancelAutotune: vi.fn(() => Promise.resolve()),
  ProbeNow: vi.fn(() => Promise.resolve([{ site: "youtube.com", stage: "ok", latency: 2e8 }])),
  DPIStrategies: vi.fn(() => Promise.resolve([{ id: "light", name: { vi: "Nhẹ", en: "Light" } }, { id: "medium", name: { vi: "Vừa", en: "Medium" } }])),
  GetDPIAutoHostlist: vi.fn(() => Promise.resolve([])),
  DPIEngineDir: vi.fn(() => Promise.resolve("")),
  PreviewDPIArgs: vi.fn(() => Promise.resolve(["-p", "-r"])),
  GetDPIBlacklist: vi.fn(() => Promise.resolve("")),
  SaveDPIBlacklist: vi.fn(() => Promise.resolve()),
  SetQueryLog: vi.fn(() => Promise.resolve()),
  RestoreDNSNow: vi.fn(() => Promise.resolve()),
  StopConflictingService: vi.fn(() => Promise.resolve()),
  ListAdapters: vi.fn(() => Promise.resolve([])),
  DNSInfo: vi.fn(() => Promise.resolve({ backend: "windows", chain: "Windows", interfaces: [], adapterPick: true, adapters: [] })),
}));
vi.mock("../../app/api", () => ({ Service: svc }));

const settings = {
  version: 1, language: "vi", mode: "full", startWithWindows: false, autoConnect: false, closeToTray: true,
  adapters: "auto", adapterGuids: [], testDomain: "www.google.com", bootstrap: ["1.1.1.1:53", "8.8.8.8:53"],
  maxUpstreams: 5, includeTags: ["no-filter"], pinned: [], pinnedOnly: false,
  probeSites: ["youtube.com", "discord.com"],
  dpi: { enabled: false, preset: "light", customArgs: "", scope: "all" },
  fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
  updates: { checkApp: true, updateServerList: true },
  fullWindow: { width: 1000, height: 660 },
};

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSettings(structuredClone(settings) as any);
  useGhost.getState().setSnapshot({ status: "protected", warnings: [], servers: [], blockedSites: [], dpi: { enabled: false, running: false, preset: "light" } } as any);
});

test("custom args validation error from SaveSettings is shown inline", async () => {
  svc.SaveSettings.mockResolvedValueOnce(undefined as any).mockRejectedValueOnce(new Error('dpi: flag not allowed: "--dns-addr"'));
  render(<Dpi />);
  fireEvent.change(screen.getByRole("combobox", { name: "preset" }), { target: { value: "custom" } });
  fireEvent.change(screen.getByRole("textbox", { name: "tham số tự nhập" }), { target: { value: "--dns-addr 1.1.1.1" } });
  fireEvent.blur(screen.getByRole("textbox", { name: "tham số tự nhập" }));
  expect(await screen.findByText(/flag not allowed/)).toBeInTheDocument();
});

test("autotune progress renders preset steps", async () => {
  render(<Dpi />);
  fireEvent.click(screen.getByRole("button", { name: "⚡ tự dò" }));
  expect(svc.StartAutotune).toHaveBeenCalled();
  act(() => useGhost.getState().setAutotune({ preset: "medium", index: 2, total: 4, running: true } as any));
  expect(await screen.findByText(/đang dò: Vừa \(2\/4\)/)).toBeInTheDocument();
});

test("dpi toggle and probe", async () => {
  render(<Dpi />);
  fireEvent.click(screen.getByRole("switch", { name: "VƯỢT DPI CHO MỌI ỨNG DỤNG" }));
  expect(svc.SetDPIEnabled).toHaveBeenCalledWith(true);
  fireEvent.click(screen.getByRole("button", { name: "⟳ thử lại" }));
  expect(await screen.findByText("✓")).toBeInTheDocument();
  expect(await screen.findByText(/goodbyedpi\.exe -p -r/)).toBeInTheDocument();
});

test("query log toggle calls SetQueryLog and shows RAM-only note", () => {
  render(<Logs />);
  expect(screen.getByText(/chỉ giữ trong RAM/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("switch", { name: "hiện truy vấn" }));
  expect(svc.SetQueryLog).toHaveBeenCalledWith(true);
});

test("logs translate codes and filter by source", () => {
  useGhost.getState().setLogs([
    { time: "2026-10-04T14:02:11+07:00", source: "system", code: "CONNECTED", params: { servers: 5 } },
    { time: "2026-10-04T14:05:41+07:00", source: "dpi", code: "DPI_STARTED", params: { engine: "goodbyedpi", preset: "medium" } },
  ] as any);
  render(<Logs />);
  expect(screen.getByText("đã kết nối với 5 máy chủ")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "dpi" }));
  expect(screen.queryByText("đã kết nối với 5 máy chủ")).toBeNull();
  expect(screen.getByText("goodbyedpi đã chạy (preset medium)")).toBeInTheDocument();
});

test("settings: restore DNS now calls RestoreDNSNow", () => {
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: /KHÔI PHỤC DNS NGAY/ }));
  expect(svc.RestoreDNSNow).toHaveBeenCalled();
});

test("settings: toggling start with windows saves", () => {
  render(<Settings />);
  fireEvent.click(screen.getByRole("switch", { name: "khởi động cùng windows" }));
  expect(svc.SaveSettings).toHaveBeenCalledWith(expect.objectContaining({ startWithWindows: true }));
});

test("stopping a conflicting service requires in-page confirmation", () => {
  useGhost.getState().setSnapshot({ status: "error", error: { code: "PORT53_BUSY", params: { pid: 4, name: "svchost.exe", service: "SharedAccess" } }, warnings: [], dpi: {} } as any);
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "Tạm dừng dịch vụ SharedAccess" }));
  expect(svc.StopConflictingService).not.toHaveBeenCalled();
  expect(screen.getByText(/Mobile Hotspot/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "xác nhận" }));
  expect(svc.StopConflictingService).toHaveBeenCalledWith("SharedAccess");
});

test("settings: a failed restore is shown, not swallowed", async () => { // review minor
  svc.RestoreDNSNow.mockRejectedValueOnce(new Error("netsh failed"));
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: /KHÔI PHỤC DNS NGAY/ }));
  expect(await screen.findByText(/netsh failed/)).toBeInTheDocument();
});

test("settings: a failed service stop is shown", async () => {
  svc.StopConflictingService.mockRejectedValueOnce(new Error("access denied"));
  useGhost.getState().setSnapshot({ status: "error", error: { code: "PORT53_BUSY", params: { pid: 4, name: "svchost.exe", service: "SharedAccess" } }, warnings: [], dpi: {} } as any);
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "Tạm dừng dịch vụ SharedAccess" }));
  fireEvent.click(screen.getByRole("button", { name: "xác nhận" }));
  expect(await screen.findByText(/access denied/)).toBeInTheDocument();
});

test("query view toggle survives leaving the logs page", () => { // review minor
  const first = render(<Logs />);
  fireEvent.click(screen.getByRole("switch", { name: "hiện truy vấn" }));
  first.unmount();
  render(<Logs />);
  expect(screen.getByRole("switch", { name: "hiện truy vấn" })).toHaveAttribute("aria-checked", "true");
});

test("errors from Go are translated, not shown raw", async () => {
  svc.SetDPIEnabled.mockRejectedValueOnce(new Error("DPI_START_FAILED: dpi: GoodbyeDPI failed to start"));
  render(<Dpi />);
  fireEvent.click(screen.getByRole("switch", { name: "VƯỢT DPI CHO MỌI ỨNG DỤNG" }));
  expect(await screen.findByText(/Engine vượt DPI không chạy được/)).toBeInTheDocument();
  expect(screen.queryByText(/map\[/)).toBeNull();
});

test("dpi toggle reflects the new state and the snapshot from Go", async () => {
  render(<Dpi />);
  const sw = screen.getByRole("switch", { name: "VƯỢT DPI CHO MỌI ỨNG DỤNG" });
  fireEvent.click(sw);
  await waitFor(() => expect(sw).toHaveAttribute("aria-checked", "true"));
  // Turned off from the tray: the snapshot is the source of truth.
  act(() => useGhost.getState().setSnapshot({ ...useGhost.getState().snapshot, dpi: { enabled: false, running: false, preset: "light" } } as any));
  expect(sw).toHaveAttribute("aria-checked", "false");
  expect(useGhost.getState().settings?.dpi.enabled).toBe(false);
});

test("dpi shows that it waits for a connection", () => {
  useGhost.getState().setSnapshot({ status: "disconnected", warnings: [], servers: [], blockedSites: [], dpi: { enabled: true, running: false, preset: "light" } } as any);
  render(<Dpi />);
  expect(screen.getByText(/chạy khi kết nối/)).toBeInTheDocument();
});

test("dpi shows it is starting while connected", () => {
  useGhost.getState().setSnapshot({ status: "protected", warnings: [], servers: [], blockedSites: [], dpi: { enabled: true, running: false, preset: "light" } } as any);
  render(<Dpi />);
  expect(screen.getByText(/đang khởi động/)).toBeInTheDocument();
});

test("settings: several test domains are allowed, with a warning past two", () => {
  render(<Settings />);
  const box = screen.getByRole("textbox", { name: "tên miền thử" });
  fireEvent.change(box, { target: { value: "www.google.com\nyoutube.com" } });
  expect(screen.queryByRole("status")).toBeNull();
  fireEvent.change(box, { target: { value: "www.google.com\nyoutube.com\ndiscord.com" } });
  expect(screen.getByRole("status")).toHaveTextContent("Không nên dùng nhiều tên miền: 3 tên miền");
});
