import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { FakeSni } from "./FakeSni";
import { Overview } from "./Overview";
import { Settings } from "./Settings";
import { Lists } from "./Lists";
import { RulesTable } from "./RulesTable";
import { explainText } from "./rulesFormat";
import { FakeSniBanner } from "../../../components/FakeSniBanner";
import { useGhost } from "../../../app/store";
import { initI18n } from "../../../i18n";
import i18n from "../../../i18n";

const view = { ack: false, enabled: false, rules: [{ pattern: "youtube.com", sni: "www.google.com", connect: "www.google.com", enabled: true }], lists: [], stats: { open: 0, lanClients: 0, bytesIn: 0, bytesOut: 0, byOutcome: { fakesni: 3, fakesni_fallback: 1 } } };
const preset = { id: "fakesni-google", name: "Fake SNI: Google & YouTube", description: "", category: "fakesni", repo: "", license: "GPL-3.0",
  url: "https://raw.githubusercontent.com/hashcott/ghostline/main/lists/fakesni/google.txt", format: "ghostline", action: "perLine", signed: true, trustedForSNI: true };

const svc = vi.hoisted(() => ({
  GetFakeSNIView: vi.fn(),
  AckFakeSNIWarning: vi.fn(() => Promise.resolve()),
  SetFakeSNI: vi.fn(() => Promise.resolve()),
  SetProxyEnabled: vi.fn(() => Promise.resolve()),
  RetryFakeSNI: vi.fn(() => Promise.resolve()),
  Catalog: vi.fn(),
  AddList: vi.fn(() => Promise.resolve({})),
  DeleteList: vi.fn(() => Promise.resolve()),
  GetRules: vi.fn(() => Promise.resolve({ rules: [], lists: [] })),
  ListCerts: vi.fn(() => Promise.resolve([{ thumbprint: "aa11", subject: "Ghostline Fake SNI — phiên 2026-10-05", notAfter: "2026-11-04T00:00:00Z" }])),
  RemoveAllCerts: vi.fn(() => Promise.resolve()),
  RetryCertRemoval: vi.fn(() => Promise.resolve()),
  SetListTrustedForSNI: vi.fn(() => Promise.resolve()),
  UpdateList: vi.fn(() => Promise.resolve()),
  RefreshList: vi.fn(() => Promise.resolve()),
  MoveList: vi.fn(() => Promise.resolve()),
  GetSettings: vi.fn(),
  SaveSettings: vi.fn(() => Promise.resolve()),
  ListAdapters: vi.fn(() => Promise.resolve([])),
  DNSInfo: vi.fn(() => Promise.resolve({ backend: "windows", chain: "Windows", interfaces: [], adapterPick: true, adapters: [] })),
  GetProxyStats: vi.fn(() => Promise.resolve(null)),
  Connect: vi.fn(() => Promise.resolve()),
  Disconnect: vi.fn(() => Promise.resolve()),
  SetMode: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../../app/api", () => ({ Service: svc }));
vi.mock("@wailsio/runtime", () => ({ Browser: { OpenURL: vi.fn() } }));

const settings = {
  version: 4, language: "vi", mode: "full", probeSites: [], bootstrap: ["1.1.1.1:53"], pinned: [], adapters: "auto",
  dpi: { enabled: false, preset: "light", customArgs: "", scope: "all" },
  fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
  proxy: { enabled: true, port: 8080, systemProxy: true, shareLan: false, fragment: { mode: "auto", method: "both", chunks: 5, delayMs: 5, autoTimeoutMs: 3000, cacheDays: 7 }, upstreams: [] },
  dnsBlockMode: "zero",
  dnsServer: { enabled: false, shareLan: false, dohPort: 443, iosSsid: "" },
  fakeSni: { enabled: false, ackVersion: 0 },
  updates: { checkApp: true, updateServerList: true },
};
const snap = {
  status: "protected", warnings: [], servers: ["Cloudflare"], blockedSites: [], reasons: [],
  dpi: { enabled: false, running: false, preset: "light" },
  proxy: { running: true, addr: "127.0.0.1:8080", systemProxy: true, shareLan: false },
  dnsServer: { running: false, addrs: [] },
  fakeSni: { active: false, domains: 0, needsProxy: false, notAfter: "" },
};

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  svc.GetFakeSNIView.mockResolvedValue(structuredClone(view));
  svc.Catalog.mockResolvedValue([preset]);
  useGhost.getState().reset();
  useGhost.getState().setSettings(structuredClone(settings) as any);
  useGhost.getState().setSnapshot(structuredClone(snap) as any);
});

test("the warning must be scrolled to the end and ticked before continuing", async () => {
  render(<FakeSni />);
  const cont = await screen.findByRole("button", { name: "tiếp tục" });
  expect(cont).toBeDisabled();
  fireEvent.click(screen.getByRole("checkbox", { name: "tôi đã hiểu" }));
  expect(cont).toBeDisabled();
  fireEvent.scroll(screen.getByTestId("fakesni-warning"));
  expect(cont).toBeEnabled();
  expect(screen.getByText(/không dùng cho ngân hàng/i)).toBeInTheDocument();
  fireEvent.click(cont);
  await waitFor(() => expect(svc.AckFakeSNIWarning).toHaveBeenCalled());
});

test("after the warning: switch, presets and counters", async () => {
  svc.GetFakeSNIView.mockResolvedValue({ ...structuredClone(view), ack: true });
  render(<FakeSni />);
  fireEvent.click(await screen.findByRole("switch", { name: "bật Fake SNI" }));
  await waitFor(() => expect(svc.SetFakeSNI).toHaveBeenCalledWith(true));
  fireEvent.click(await screen.findByRole("switch", { name: "Fake SNI: Google & YouTube" }));
  await waitFor(() => expect(svc.AddList).toHaveBeenCalledWith(expect.objectContaining({ url: preset.url, signed: true, trustedForSNI: true, action: "perLine", format: "ghostline" })));
  expect(screen.getByText("youtube.com")).toBeInTheDocument();
  expect(screen.getByText(/đã giải mã: 3/)).toBeInTheDocument();
  expect(screen.getByText(/Firefox/)).toBeInTheDocument();
});

test("Fake SNI needs the proxy", async () => {
  svc.GetFakeSNIView.mockResolvedValue({ ...structuredClone(view), ack: true, enabled: true });
  useGhost.getState().setSettings({ ...structuredClone(settings), proxy: { ...settings.proxy, enabled: false } } as any);
  render(<FakeSni />);
  fireEvent.click(await screen.findByRole("button", { name: "bật proxy" }));
  await waitFor(() => expect(svc.SetProxyEnabled).toHaveBeenCalledWith(true));
});

test("the banner shows while active, cannot be closed and turns Fake SNI off", () => {
  const { container } = render(<FakeSniBanner />);
  expect(container).toBeEmptyDOMElement();
  act(() => useGhost.getState().setSnapshot({ ...structuredClone(snap), fakeSni: { active: true, domains: 4, needsProxy: false, notAfter: "" } } as any));
  expect(screen.getByText(/đang giải mã HTTPS của 4 tên miền/)).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /đóng/ })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "tắt Fake SNI" }));
  expect(svc.SetFakeSNI).toHaveBeenCalledWith(false);
});

test("rules show sni and connect; the tester explains Fake SNI", () => {
  render(<RulesTable rules={[{ pattern: "youtube.com", sni: "www.google.com", connect: "front.example", enabled: true } as any]} upstreams={[]} onSave={() => {}} />);
  expect(screen.getByText(/sni=www\.google\.com/)).toBeInTheDocument();
  expect(screen.getByText(/connect=front\.example/)).toBeInTheDocument();
  expect(screen.queryByText(/giai đoạn 2B/)).not.toBeInTheDocument();
  const t = i18n.t.bind(i18n) as any;
  expect(explainText({ sni: "www.google.com", connect: "x.com", source: { kind: "rule", index: 0 } } as any, [], t)).toMatch(/giải mã bằng Fake SNI \(sni=www\.google\.com, connect=x\.com\)/);
  expect(explainText({ sniIgnored: true, source: { kind: "list", listId: "u", line: 2 } } as any, [], t)).toMatch(/chưa được tin cho Fake SNI/);
});

test("trusting a list for Fake SNI asks first", async () => {
  const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
  const list = { id: "p", name: "P", source: "url", url: "https://x/p.txt", format: "auto", action: "perLine", enabled: true, updateHours: 24, lastUpdated: "", skipped: 0, counts: { domain: 12 }, trustedForSNI: false };
  render(<Lists lists={[list as any]} upstreams={[]} onChanged={() => {}} />);
  const sw = screen.getByRole("switch", { name: "tin cho Fake SNI: P" });
  fireEvent.click(sw);
  expect(confirm.mock.calls[0][0]).toMatch(/12/);
  expect(svc.SetListTrustedForSNI).not.toHaveBeenCalled();
  fireEvent.click(sw);
  await waitFor(() => expect(svc.SetListTrustedForSNI).toHaveBeenCalledWith("p", true));
  confirm.mockRestore();
});

test("settings list the certificates and remove them after confirming", async () => {
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  render(<Settings />);
  expect(await screen.findByText(/Ghostline Fake SNI — phiên/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "gỡ tất cả chứng chỉ Ghostline" }));
  await waitFor(() => expect(svc.RemoveAllCerts).toHaveBeenCalled());
  confirm.mockRestore();
});

test("overview shows the Fake SNI card", async () => {
  useGhost.getState().setSnapshot({ ...structuredClone(snap), fakeSni: { active: true, domains: 4, needsProxy: false, notAfter: "" } } as any);
  render(<Overview />);
  expect(await screen.findByText(/4 tên miền/)).toBeInTheDocument();
});

test("turning Fake SNI off from the banner refreshes the settings copy", async () => {
  svc.GetSettings.mockResolvedValue({ ...structuredClone(settings), fakeSni: { enabled: false, ackVersion: 1 } } as any);
  useGhost.getState().setSettings({ ...structuredClone(settings), fakeSni: { enabled: true, ackVersion: 1 } } as any);
  useGhost.getState().setSnapshot({ ...structuredClone(snap), fakeSni: { active: true, domains: 2, needsProxy: false, notAfter: "" } } as any);
  render(<FakeSniBanner />);
  fireEvent.click(screen.getByRole("button", { name: "tắt Fake SNI" }));
  await waitFor(() => expect((useGhost.getState().settings as any).fakeSni.enabled).toBe(false));
});

test("the page switch reflects the saved state after a change", async () => {
  svc.GetFakeSNIView.mockResolvedValue({ ...structuredClone(view), ack: true, enabled: true });
  svc.GetSettings.mockResolvedValue({ ...structuredClone(settings), fakeSni: { enabled: false, ackVersion: 1 } } as any);
  useGhost.getState().setSettings({ ...structuredClone(settings), fakeSni: { enabled: true, ackVersion: 1 } } as any);
  render(<FakeSni />);
  const sw = await screen.findByRole("switch", { name: "bật Fake SNI" });
  svc.GetFakeSNIView.mockResolvedValue({ ...structuredClone(view), ack: true, enabled: false });
  fireEvent.click(sw);
  await waitFor(() => expect(svc.SetFakeSNI).toHaveBeenCalledWith(false));
  await waitFor(() => expect((useGhost.getState().settings as any).fakeSni.enabled).toBe(false));
  await waitFor(() => expect(screen.getByRole("switch", { name: "bật Fake SNI" })).toHaveAttribute("aria-checked", "false"));
});
