import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Dpi } from "./pages/Dpi";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";

const svc = vi.hoisted(() => ({
  DPIInfo: vi.fn(() => Promise.resolve({ engines: [{ id: "zapret2", exe: "winws2.exe" }, { id: "goodbyedpi", exe: "goodbyedpi.exe" }], mechanism: "WinDivert", avExclusions: true })),
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
  DPIEngineDir: vi.fn(() => Promise.resolve("C:\\Data\\bin\\zapret2")),
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
const saved = () => { const c = svc.SaveSettings.mock.calls as any[][]; return c[c.length - 1][0]; };

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

test("switching the engine saves it", async () => {
  render(<Dpi />);
  fireEvent.click(await screen.findByRole("button", { name: "GoodbyeDPI" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect(saved().dpi.engine).toBe("goodbyedpi");
});

test("zapret2 strategies come from the engine, in the UI language", async () => {
  render(<Dpi />);
  await waitFor(() => expect(svc.DPIStrategies).toHaveBeenCalledWith("zapret2"));
  const sel = screen.getByRole("combobox", { name: "preset" }) as HTMLSelectElement;
  await waitFor(() => expect(screen.getByRole("option", { name: "Gói giả" })).toBeInTheDocument());
  fireEvent.change(sel, { target: { value: "z-fake" } });
  await waitFor(() => expect(saved().dpi.zapret2.strategy).toBe("z-fake"));
  expect(saved().dpi.preset).toBe("light");
});

test("auto-detect is offered only for zapret2 with the blacklist scope", async () => {
  const { rerender } = render(<Dpi />);
  expect(screen.queryByRole("switch", { name: "tự phát hiện trang bị can thiệp" })).toBeNull();
  withDPI({ scope: "blacklist" });
  rerender(<Dpi />);
  fireEvent.click(await screen.findByRole("switch", { name: "tự phát hiện trang bị can thiệp" }));
  await waitFor(() => expect(saved().dpi.zapret2.autoHostlist).toBe(true));
  withDPI({ scope: "blacklist", engine: "goodbyedpi" });
  rerender(<Dpi />);
  expect(screen.queryByRole("switch", { name: "tự phát hiện trang bị can thiệp" })).toBeNull();
});

test("auto-detected sites can be removed", async () => {
  withDPI({ scope: "blacklist", zapret2: { strategy: "z-split", customArgs: "", autoHostlist: true } });
  render(<Dpi />);
  expect(await screen.findByText("a.com")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "xoá a.com" }));
  await waitFor(() => expect(svc.SaveDPIAutoHostlist).toHaveBeenCalledWith(["b.com"]));
  fireEvent.click(screen.getByRole("button", { name: "xoá hết" }));
  await waitFor(() => expect(svc.SaveDPIAutoHostlist).toHaveBeenLastCalledWith([]));
});

test("fallback banner explains the exclusion and retries zapret2", async () => {
  snap({ engine: "goodbyedpi", preset: "light", fallback: true });
  render(<Dpi />);
  expect(await screen.findByText(/C:\\Data\\bin\\zapret2/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "thử lại zapret2" }));
  await waitFor(() => expect(svc.RetryZapret2).toHaveBeenCalled());
});

test("the engine hint shows for GoodbyeDPI users until dismissed", async () => {
  withDPI({ engine: "goodbyedpi" });
  const { rerender } = render(<Dpi />);
  expect(screen.getByText(/Thử engine mới zapret2/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "ẩn gợi ý" }));
  await waitFor(() => expect(saved().dpi.hideEngineHint).toBe(true));
  withDPI({ engine: "goodbyedpi", hideEngineHint: true });
  rerender(<Dpi />);
  expect(screen.queryByText(/Thử engine mới zapret2/)).toBeNull();
  withDPI({});
  rerender(<Dpi />);
  expect(screen.queryByText(/Thử engine mới zapret2/)).toBeNull();
});

test("zapret2 custom args are kept apart from GoodbyeDPI's", async () => {
  withDPI({ zapret2: { strategy: "custom", customArgs: "", autoHostlist: false } });
  render(<Dpi />);
  const box = screen.getByRole("textbox", { name: "tham số tự nhập" });
  fireEvent.change(box, { target: { value: "--lua-desync=multisplit:pos=2" } });
  fireEvent.blur(box);
  await waitFor(() => expect(saved().dpi.zapret2.customArgs).toBe("--lua-desync=multisplit:pos=2"));
  expect(saved().dpi.customArgs).toBe("");
});

test("preview names the running engine's program", async () => {
  render(<Dpi />);
  await waitFor(() => expect(svc.PreviewDPIArgs).toHaveBeenCalledWith("zapret2", "z-split", "", "all", false));
  expect(screen.getByLabelText("dòng lệnh").textContent).toMatch(/^winws2\.exe/);
});

test("the running line names the strategy, not its id", async () => {
  render(<Dpi />);
  expect(await screen.findByText(/zapret2 đã chạy \(preset Nhẹ\)/)).toBeInTheDocument();
});

test("switching engine never shows the old engine's args or a raw strategy id", async () => {
  withDPI({ engine: "goodbyedpi" });
  svc.PreviewDPIArgs.mockImplementation(((engine: string) =>
    engine === "zapret2" ? new Promise(() => {}) : Promise.resolve(["-p", "-r"])) as any);
  render(<Dpi />);
  await waitFor(() => expect(screen.getByLabelText("dòng lệnh").textContent).toContain("-p -r"));
  svc.DPIStrategies.mockImplementation((() => new Promise(() => {})) as any); // zapret2's list still loading
  fireEvent.click(screen.getByRole("button", { name: "zapret2 (khuyên dùng)" }));
  await waitFor(() => expect(screen.getByLabelText("dòng lệnh").textContent).toMatch(/^winws2\.exe/));
  expect(screen.getByLabelText("dòng lệnh").textContent).not.toContain("-p -r");
  const sel = screen.getByRole("combobox", { name: "preset" }) as HTMLSelectElement;
  expect(sel.options[sel.selectedIndex].text).not.toBe("z-split");
});

test("picking an engine by hand retires the 'try zapret2' hint", async () => {
  withDPI({ engine: "goodbyedpi" });
  render(<Dpi />);
  fireEvent.click(await screen.findByRole("button", { name: "zapret2 (khuyên dùng)" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect(saved().dpi.engine).toBe("zapret2");
  expect(saved().dpi.hideEngineHint).toBe(true);
});

test("auto-tune progress and result name the strategy", async () => {
  useGhost.getState().setAutotune({ running: true, engine: "zapret2", preset: "z-fake", index: 2, total: 4 } as any);
  const { rerender } = render(<Dpi />);
  expect(await screen.findByRole("button", { name: "đang dò: Gói giả (2/4)" })).toBeInTheDocument();
  useGhost.getState().setAutotune({ running: false, engine: "zapret2", preset: "z-fake", index: 0, total: 0 } as any);
  rerender(<Dpi />);
  expect(await screen.findByText("✓ tự dò đã chọn Gói giả (zapret2)")).toBeInTheDocument();
});
