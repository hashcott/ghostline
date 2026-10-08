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
  DPIStrategies: vi.fn(() => Promise.resolve([{ id: "light", name: { vi: "Nhẹ", en: "Light" } }, { id: "medium", name: { vi: "Vừa", en: "Medium" } }])),
  GetDPIAutoHostlist: vi.fn(() => Promise.resolve([])),
  DPIEngineDir: vi.fn(() => Promise.resolve("")),
  PreviewDPIArgs: vi.fn(() => Promise.resolve([])),
  GetDPIBlacklist: vi.fn(() => Promise.resolve("")),
  SaveDPIBlacklist: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../app/api", () => ({ Service: svc }));

const settings = {
  version: 1, language: "vi", mode: "full", probeSites: ["youtube.com", "discord.com"],
  dpi: { enabled: false, preset: "light", customArgs: "", scope: "all" },
  fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
};
const withScope = (scope: string) => useGhost.getState().setSettings({ ...structuredClone(settings), dpi: { ...settings.dpi, scope } } as any);
const editor = () => screen.queryByRole("textbox", { name: "Danh sách đen (mỗi dòng một domain)" }) as HTMLTextAreaElement | null;

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  svc.GetDPIBlacklist.mockResolvedValue("");
  useGhost.getState().reset();
  withScope("all");
  useGhost.getState().setSnapshot({ status: "protected", warnings: [], servers: [], dpi: { enabled: false, running: false, preset: "light" } } as any);
});

test("no separate edit button; the list shows only under blacklist scope", async () => {
  svc.GetDPIBlacklist.mockResolvedValue("youtube.com\n# note\ndiscord.com\n");
  render(<Dpi />);
  expect(screen.queryByRole("button", { name: "sửa ›" })).not.toBeInTheDocument();
  expect(editor()).toBeNull();
  // the saved list's entries are counted on the chip (comments skipped)
  const chip = await screen.findByRole("button", { name: "danh sách đen (2)" });
  fireEvent.click(chip);
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect((svc.SaveSettings.mock.calls[0] as any[])[0].dpi.scope).toBe("blacklist");
});

test("blacklist scope keeps the editor open inline; switching to all hides it", async () => {
  svc.GetDPIBlacklist.mockResolvedValue("youtube.com\n");
  withScope("blacklist");
  const { rerender } = render(<Dpi />);
  await waitFor(() => expect(editor()?.value).toBe("youtube.com\n"));
  fireEvent.click(screen.getByRole("button", { name: "mọi kết nối" }));
  withScope("all");
  rerender(<Dpi />);
  expect(editor()).toBeNull();
});

test("an empty list is pre-filled with the sample sites and scope waits for save", async () => {
  render(<Dpi />);
  fireEvent.click(await screen.findByRole("button", { name: "danh sách đen" }));
  await waitFor(() => expect(editor()?.value).toBe("youtube.com\ndiscord.com\n"));
  expect(svc.SaveSettings).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "lưu" }));
  await waitFor(() => expect(svc.SaveDPIBlacklist).toHaveBeenCalledWith("youtube.com\ndiscord.com\n"));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect((svc.SaveSettings.mock.calls[0] as any[])[0].dpi.scope).toBe("blacklist");
});

test("cancel on a new list leaves scope at all", async () => {
  render(<Dpi />);
  fireEvent.click(await screen.findByRole("button", { name: "danh sách đen" }));
  await waitFor(() => expect(editor()).not.toBeNull());
  fireEvent.click(screen.getByRole("button", { name: "huỷ" }));
  expect(editor()).toBeNull();
  expect(svc.SaveSettings).not.toHaveBeenCalled();
  expect(svc.SaveDPIBlacklist).not.toHaveBeenCalled();
});

test("an empty list cannot be saved", async () => {
  svc.GetDPIBlacklist.mockResolvedValue("youtube.com\n");
  withScope("blacklist");
  render(<Dpi />);
  await waitFor(() => expect(editor()).not.toBeNull());
  fireEvent.change(editor()!, { target: { value: "  \n# nothing\n" } });
  fireEvent.click(screen.getByRole("button", { name: "lưu" }));
  expect(await screen.findByText(/danh sách đen đang trống/)).toBeInTheDocument();
  expect(svc.SaveDPIBlacklist).not.toHaveBeenCalled();
});

test("saving while GoodbyeDPI runs says it restarted", async () => {
  svc.GetDPIBlacklist.mockResolvedValue("youtube.com\n");
  withScope("blacklist");
  useGhost.getState().setSnapshot({ status: "protected", warnings: [], servers: [], dpi: { enabled: true, running: true, preset: "light" } } as any);
  render(<Dpi />);
  await waitFor(() => expect(editor()).not.toBeNull());
  fireEvent.change(editor()!, { target: { value: "youtube.com\nx.com\n" } });
  fireEvent.click(screen.getByRole("button", { name: "lưu" }));
  await waitFor(() => expect(svc.SaveDPIBlacklist).toHaveBeenCalledWith("youtube.com\nx.com\n"));
  expect(await screen.findByText(/GoodbyeDPI đã khởi động lại/)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "danh sách đen (2)" })).toBeInTheDocument();
});

test("the page reminds the user they are responsible for lawful use", async () => {
  render(<Dpi />);
  expect(await screen.findByText(/tự chịu trách nhiệm tuân thủ pháp luật/)).toBeInTheDocument();
});
