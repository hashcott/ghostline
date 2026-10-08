import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { Proxy } from "./Proxy";
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
  SysProxyInfo: vi.fn(),
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

test("linux: names the desktop whose proxy is set", async () => {
  svc.SysProxyInfo.mockResolvedValue({ desktop: "KDE", supported: true });
  render(<Proxy />);
  expect(await screen.findByText("KDE", { exact: false })).toBeInTheDocument();
});

test("linux: a desktop Ghostline cannot set shows how to do it by hand", async () => {
  svc.SysProxyInfo.mockResolvedValue({ desktop: "XFCE", supported: false });
  render(<Proxy />);
  expect(await screen.findByText("127.0.0.1:8080", { exact: false })).toBeInTheDocument();
  expect(screen.getByText("XFCE", { exact: false })).toBeInTheDocument();
});

test("windows: no desktop line", async () => {
  svc.SysProxyInfo.mockResolvedValue({ desktop: "", supported: true });
  render(<Proxy />);
  await waitFor(() => expect(svc.SysProxyInfo).toHaveBeenCalled());
  expect(screen.queryByText("KDE", { exact: false })).toBeNull();
});
