import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { Dpi } from "./pages/Dpi";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";

const svc = vi.hoisted(() => ({
  SaveSettings: vi.fn(() => Promise.resolve()),
  SetDPIEnabled: vi.fn(() => Promise.resolve()),
  StartAutotune: vi.fn(() => Promise.resolve()),
  CancelAutotune: vi.fn(() => Promise.resolve()),
  ProbeNow: vi.fn(() => Promise.resolve([])),
  PreviewDPIArgs: vi.fn(() => Promise.resolve([])),
  GetDPIBlacklist: vi.fn(() => Promise.resolve("youtube.com\n")),
  SaveDPIBlacklist: vi.fn(() => Promise.resolve()),
  DPIStrategies: vi.fn(),
  GetDPIAutoHostlist: vi.fn(() => Promise.resolve(["a.com", "b.com"])),
  SaveDPIAutoHostlist: vi.fn(() => Promise.resolve()),
  RetryZapret2: vi.fn(() => Promise.resolve()),
  DPIEngineDir: vi.fn(() => Promise.resolve("/var/lib/ghostline/bin/zapret2")),
  DPIInfo: vi.fn(() => Promise.resolve({ engines: [{ id: "zapret2", exe: "nfqws2" }], mechanism: "nftables inet ghostline, queue 200", avExclusions: false })),
}));
vi.mock("../../app/api", () => ({ Service: svc }));
const strategiesFor = (engine: string) =>
  Promise.resolve(
    engine === "zapret2"
      ? [
          { id: "z-split", name: { vi: "Nhẹ", en: "Light" } },
          { id: "z-fake", name: { vi: "Gói giả", en: "Fake" } },
        ]
      : [{ id: "light", name: { vi: "Nhẹ", en: "Light" } }],
  );

const base = {
  version: 3, language: "vi", mode: "full", probeSites: ["youtube.com"],
  dpi: {
    enabled: true, engine: "zapret2", preset: "light", customArgs: "", scope: "all",
    zapret2: { strategy: "z-split", customArgs: "", autoHostlist: false }, hideEngineHint: false,
  },
  fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
};
const withDPI = (dpi: Record<string, unknown>) =>
  useGhost.getState().setSettings({ ...structuredClone(base), dpi: { ...base.dpi, ...dpi } } as any);
const snap = (dpi: Record<string, unknown>) =>
  useGhost.getState().setSnapshot({ status: "protected", warnings: [], servers: [], dpi: { enabled: true, running: true, engine: "zapret2", preset: "z-split", fallback: false, ...dpi } } as any);

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  // tests may swap these for never-settling promises; start each one fresh
  svc.DPIStrategies.mockImplementation(strategiesFor as any);
  svc.PreviewDPIArgs.mockImplementation((() => Promise.resolve([])) as any);
  useGhost.getState().reset();
  withDPI({});
  snap({});
});

test("linux: only zapret2, nfqws2 in the preview, capture shown", async () => {
  svc.PreviewDPIArgs.mockImplementation((() => Promise.resolve(["--qnum=200"])) as any);
  render(<Dpi />);
  expect(await screen.findByText("nftables inet ghostline, queue 200", { exact: false })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /zapret2/ })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "GoodbyeDPI" })).toBeNull();
  await waitFor(() => expect(screen.getByLabelText("dòng lệnh").textContent).toContain("nfqws2 --qnum=200"));
});

test("linux: settings naming GoodbyeDPI (a Windows backup) show zapret2", async () => {
  withDPI({ engine: "goodbyedpi" });
  render(<Dpi />);
  await waitFor(() => expect(svc.DPIStrategies).toHaveBeenCalledWith("zapret2"));
  expect(screen.queryByText("GoodbyeDPI", { exact: false })).toBeNull();
});
