import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { Settings } from "./Settings";
import { useGhost } from "../../../app/store";
import { initI18n } from "../../../i18n";

const svc = vi.hoisted(() => ({
  ListCerts: vi.fn(() => Promise.resolve([])),
  CheckUpdateNow: vi.fn(),
  InstallUpdate: vi.fn(),
  ListAdapters: vi.fn(() => Promise.resolve([])),
  DNSInfo: vi.fn(() => Promise.resolve({ backend: "windows", chain: "Windows", interfaces: [], adapterPick: true, adapters: [] })),
  SaveSettings: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../../app/api", () => ({ Service: svc }));
const browser = vi.hoisted(() => ({ OpenURL: vi.fn() }));
vi.mock("@wailsio/runtime", () => ({ Browser: browser }));

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSettings({
    version: 2, language: "vi", adapters: "auto", adapterGuids: [], bootstrap: ["1.1.1.1:53"], testDomain: "www.google.com",
    maxUpstreams: 5, updates: { checkApp: true, updateServerList: true },
  } as any);
  useGhost.getState().setInfo({ version: "0.2.1", portable: false, updateTag: "", updateUrl: "" } as any);
});

test("check for updates: already up to date", async () => {
  svc.CheckUpdateNow.mockResolvedValueOnce({ current: "0.2.1", latest: "v0.2.1", url: "u", newer: false });
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "kiểm tra cập nhật" }));
  expect(await screen.findByText("đã là bản mới nhất (0.2.1)")).toBeInTheDocument();
});

test("check for updates: a newer release opens its page and is remembered", async () => {
  svc.CheckUpdateNow.mockResolvedValueOnce({ current: "0.2.1", latest: "v0.2.2", url: "https://github.com/x/releases/tag/v0.2.2", newer: true });
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "kiểm tra cập nhật" }));
  const link = await screen.findByRole("button", { name: "có bản mới v0.2.2 ↗" });
  fireEvent.click(link);
  expect(browser.OpenURL).toHaveBeenCalledWith("https://github.com/x/releases/tag/v0.2.2");
  expect(useGhost.getState().update).toEqual({ tag: "v0.2.2", url: "https://github.com/x/releases/tag/v0.2.2" });
});

test("check for updates: shows the error", async () => {
  svc.CheckUpdateNow.mockRejectedValueOnce(new Error("UPDATE_CHECK_FAILED: offline"));
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "kiểm tra cập nhật" }));
  expect(await screen.findByText(/không kiểm tra được bản mới/)).toBeInTheDocument();
});

test("about: shows the author and opens the GitHub repository", async () => {
  useGhost.getState().setInfo({ version: "0.4.0", portable: false, updateTag: "", updateUrl: "",
    author: "Harry Nguyen", repoUrl: "https://github.com/hashcott/ghostline" } as any);
  render(<Settings />);
  expect(screen.getByText("Ghostline 0.4.0 · tác giả Harry Nguyen")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "GitHub ↗" }));
  expect(browser.OpenURL).toHaveBeenCalledWith("https://github.com/hashcott/ghostline");
});

test("one-click update: confirms, installs, and shows the download", async () => {
  useGhost.getState().setInfo({ version: "0.2.1", portable: false, updateTag: "v0.2.2", updateUrl: "https://r/v0.2.2", selfUpdate: true } as any);
  svc.InstallUpdate.mockReturnValueOnce(new Promise(() => {})); // the app closes before it resolves
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "cập nhật lên v0.2.2" }));
  expect(confirm).toHaveBeenCalled();
  expect(svc.InstallUpdate).toHaveBeenCalled();
  expect(await screen.findByRole("button", { name: "đang tải v0.2.2…" })).toBeDisabled();
});

test("one-click update: cancelled confirm does nothing", () => {
  useGhost.getState().setInfo({ version: "0.2.1", portable: false, updateTag: "v0.2.2", updateUrl: "u", selfUpdate: true } as any);
  vi.spyOn(window, "confirm").mockReturnValue(false);
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "cập nhật lên v0.2.2" }));
  expect(svc.InstallUpdate).not.toHaveBeenCalled();
});

test("one-click update: a failure falls back to the release page", async () => {
  useGhost.getState().setInfo({ version: "0.2.1", portable: false, updateTag: "v0.2.2", updateUrl: "https://r/v0.2.2", selfUpdate: true } as any);
  svc.InstallUpdate.mockRejectedValueOnce(new Error("UPDATE_INSTALL_FAILED: checksum"));
  vi.spyOn(window, "confirm").mockReturnValue(true);
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "cập nhật lên v0.2.2" }));
  const page = await screen.findByRole("button", { name: "không cài được, mở trang v0.2.2 ↗" });
  expect(page).toHaveAttribute("title", "không cài được bản mới (checksum)");
  fireEvent.click(page);
  expect(browser.OpenURL).toHaveBeenCalledWith("https://r/v0.2.2");
});

test("portable builds keep the release page link", () => {
  useGhost.getState().setInfo({ version: "0.2.1", portable: true, updateTag: "v0.2.2", updateUrl: "https://r/v0.2.2", selfUpdate: false } as any);
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "có bản mới v0.2.2 ↗" }));
  expect(browser.OpenURL).toHaveBeenCalledWith("https://r/v0.2.2");
});

test("a withdrawn notice (channel switched to stable) hides the update", () => {
  useGhost.getState().setInfo({ version: "0.2.1", portable: false, updateTag: "v0.3.0-beta.1", updateUrl: "b" } as any);
  useGhost.getState().setUpdate({ tag: "", url: "" });
  render(<Settings />);
  expect(screen.queryByRole("button", { name: /v0.3.0-beta.1/ })).toBeNull();
});

test("a beta build shows the beta switch on and locked", async () => {
  useGhost.getState().setInfo({ version: "0.7.0-beta.1", portable: false } as any);
  render(<Settings />);
  const sw = await screen.findByRole("switch", { name: "nhận bản beta (thử nghiệm)" });
  expect(sw.getAttribute("aria-checked")).toBe("true");
  expect((sw as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText(/luôn bật khi đang dùng bản beta/)).toBeTruthy();
});

test("beta toggle saves updates.beta", async () => {
  render(<Settings />);
  fireEvent.click(screen.getByRole("switch", { name: "nhận bản beta (thử nghiệm)" }));
  await vi.waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect((svc.SaveSettings.mock.calls[0] as any[])[0].updates.beta).toBe(true);
});
