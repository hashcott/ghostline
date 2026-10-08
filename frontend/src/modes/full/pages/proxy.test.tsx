import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Proxy } from "./Proxy";
import { Warnings } from "../../../components/Warnings";
import { SimpleView } from "../../simple/SimpleView";
import { useGhost } from "../../../app/store";
import { initI18n } from "../../../i18n";

const svc = vi.hoisted(() => ({
  SaveSettings: vi.fn(() => Promise.resolve()),
  GetLANInfo: vi.fn(() => Promise.resolve({ addrs: ["192.168.1.5:8080"], public: false })),
  GetQR: vi.fn(() => Promise.resolve([[true, false], [false, true]])),
  GetFragCache: vi.fn(() => Promise.resolve(["youtube.com"])),
  ClearFragCache: vi.fn(() => Promise.resolve()),
  GetProxyStats: vi.fn(() => Promise.resolve({ open: 0, lanClients: 0, bytesIn: 0, bytesOut: 0, byOutcome: {} })),
  SaveUpstreamProxy: vi.fn(() => Promise.resolve()),
  DeleteUpstreamProxy: vi.fn(() => Promise.resolve()),
  TestUpstreamProxy: vi.fn(() => Promise.resolve()),
  RetryProxy: vi.fn(() => Promise.resolve()),
  AnswerSysProxyOverride: vi.fn(() => Promise.resolve()),
  RestoreSystemProxy: vi.fn(() => Promise.resolve()),
  GetSettings: vi.fn(),
  DismissWarning: vi.fn(() => Promise.resolve()),
  RestoreDNSNow: vi.fn(() => Promise.resolve()),
  Connect: vi.fn(() => Promise.resolve()),
  Disconnect: vi.fn(() => Promise.resolve()),
  CancelConnect: vi.fn(() => Promise.resolve()),
  StartAutotune: vi.fn(() => Promise.resolve()),
  CancelAutotune: vi.fn(() => Promise.resolve()),
  SetMode: vi.fn(() => Promise.resolve()),
  SysProxyInfo: vi.fn(() => Promise.resolve({ desktop: "", supported: true })),
}));
vi.mock("../../../app/api", () => ({ Service: svc }));
vi.mock("@wailsio/runtime", () => ({ Browser: { OpenURL: vi.fn() } }));

const settings = {
  version: 2, language: "vi", mode: "full", probeSites: [], bootstrap: ["1.1.1.1:53"], pinned: [],
  dpi: { enabled: false, preset: "light", customArgs: "", scope: "all" },
  fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
  proxy: {
    enabled: true, port: 8080, systemProxy: true, shareLan: false,
    fragment: { mode: "auto", method: "both", chunks: 5, delayMs: 5, autoTimeoutMs: 3000, cacheDays: 7 },
    upstreams: [{ id: "tor", type: "socks5", addr: "127.0.0.1:9050", user: "", passEnc: "" }],
  },
  dnsBlockMode: "zero",
};

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSettings(structuredClone(settings) as any);
  useGhost.getState().setSnapshot({
    status: "protected", warnings: [], servers: ["Cloudflare"], blockedSites: [], reasons: [],
    dpi: { enabled: false, running: false, preset: "light" },
    proxy: { running: true, addr: "127.0.0.1:8080", systemProxy: true, shareLan: false },
  } as any);
});

test("share LAN shows the addresses and a QR code", async () => {
  render(<Proxy />);
  fireEvent.click(screen.getByRole("switch", { name: "chia sẻ LAN" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect((svc.SaveSettings.mock.calls[0] as any[])[0].proxy.shareLan).toBe(true);
  expect(await screen.findByText("192.168.1.5:8080")).toBeInTheDocument();
  expect(await screen.findByRole("img", { name: /192\.168\.1\.5:8080/ })).toBeInTheDocument();
  expect(svc.GetQR).toHaveBeenCalledWith("192.168.1.5:8080");
});

test("a Public network shows the warning", async () => {
  svc.GetLANInfo.mockResolvedValueOnce({ addrs: ["192.168.1.5:8080"], public: true });
  useGhost.getState().setSettings({ ...structuredClone(settings), proxy: { ...settings.proxy, shareLan: true } } as any);
  render(<Proxy />);
  expect(await screen.findByText(/Mạng đang là Public/)).toBeInTheDocument();
});

test("an invalid port is rejected before saving", async () => {
  render(<Proxy />);
  const port = screen.getByRole("spinbutton", { name: "cổng" });
  fireEvent.change(port, { target: { value: "53" } });
  fireEvent.blur(port);
  expect(await screen.findByText(/cổng phải từ 1024 đến 65535/)).toBeInTheDocument();
  expect(svc.SaveSettings).not.toHaveBeenCalled();
});

test("upstream test reports the result", async () => {
  svc.TestUpstreamProxy.mockRejectedValueOnce(new Error("proxy: upstream \"tor\": dial refused"));
  render(<Proxy />);
  fireEvent.click(screen.getByRole("button", { name: "kiểm tra tor" }));
  expect(svc.TestUpstreamProxy).toHaveBeenCalledWith("tor");
  expect(await screen.findByText(/dial refused/)).toBeInTheDocument();
});

test("the fragment cache lists hosts and can be cleared", async () => {
  render(<Proxy />);
  expect(await screen.findByText("youtube.com")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "xoá youtube.com" }));
  expect(svc.ClearFragCache).toHaveBeenCalledWith("youtube.com");
});

test("blocked-even-fragmented suggests GoodbyeDPI", async () => {
  render(<Proxy />);
  act(() => useGhost.getState().setProxyStats({ open: 1, lanClients: 0, bytesIn: 1, bytesOut: 1, byOutcome: { blockedEvenFragmented: 2 } } as any));
  expect(await screen.findByText(/thử bật vượt DPI/)).toBeInTheDocument();
});

test("SYSPROXY_EXISTING asks before replacing", async () => {
  useGhost.getState().setSnapshot({
    ...useGhost.getState().snapshot,
    warnings: [{ code: "SYSPROXY_EXISTING", params: { server: "10.0.0.1:3128", pac: "" } }],
  } as any);
  render(<Warnings />);
  expect(screen.getByText(/10\.0\.0\.1:3128/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "ghi đè" }));
  expect(svc.AnswerSysProxyOverride).toHaveBeenCalledWith(true);
});

test("simple mode shows the proxy address while it runs", () => {
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText("127.0.0.1:8080")).toBeInTheDocument();
});

test("SYSPROXY_RESTORE_FAILED offers a restore button and the manual steps", async () => {
  useGhost.getState().setSnapshot({ ...useGhost.getState().snapshot, warnings: [{ code: "SYSPROXY_RESTORE_FAILED" }] } as any);
  render(<Warnings />);
  expect(screen.getByText(/Cài đặt Windows → Proxy/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "khôi phục system proxy" }));
  expect(svc.RestoreSystemProxy).toHaveBeenCalled();
});

test("saving an upstream reloads settings from Go instead of faking passEnc", async () => {
  const stored = { ...structuredClone(settings), proxy: { ...settings.proxy, upstreams: [{ id: "tor", type: "socks5", addr: "127.0.0.1:9050", user: "", passEnc: "enc" }] } };
  svc.GetSettings.mockResolvedValue(stored as any);
  render(<Proxy />);
  fireEvent.click(screen.getByRole("button", { name: "sửa" }));
  fireEvent.change(screen.getByLabelText("mật khẩu"), { target: { value: "pw" } });
  fireEvent.click(screen.getByRole("button", { name: "lưu" }));
  await waitFor(() => expect(svc.GetSettings).toHaveBeenCalled());
  await waitFor(() => expect((useGhost.getState().settings as any).proxy.upstreams[0].passEnc).toBe("enc"));
});
