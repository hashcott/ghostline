import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { Settings } from "./Settings";
import { useGhost } from "../../../app/store";
import { initI18n } from "../../../i18n";

const svc = vi.hoisted(() => ({
  ListCerts: vi.fn(() => Promise.resolve([])),
  CheckUpdateNow: vi.fn(),
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
