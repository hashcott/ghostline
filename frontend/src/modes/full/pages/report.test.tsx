import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Settings } from "./Settings";
import { Logs } from "./Logs";
import { useGhost } from "../../../app/store";
import { initI18n } from "../../../i18n";

const FORM = "https://github.com/hashcott/ghostline/issues/new?template=bug.yml&version=0.7.0";
const svc = vi.hoisted(() => ({
  ListCerts: vi.fn(() => Promise.resolve([])),
  ListAdapters: vi.fn(() => Promise.resolve([])),
  DNSInfo: vi.fn(() => Promise.resolve({ backend: "windows", chain: "Windows", interfaces: [], adapterPick: true, adapters: [] })),
  SaveSettings: vi.fn(() => Promise.resolve()),
  SetQueryLog: vi.fn(() => Promise.resolve()),
  ReportIssueURL: vi.fn(() => Promise.resolve("https://github.com/hashcott/ghostline/issues/new?template=bug.yml&version=0.7.0")),
}));
vi.mock("../../../app/api", () => ({ Service: svc }));
const browser = vi.hoisted(() => ({ OpenURL: vi.fn(() => Promise.resolve()) }));
vi.mock("@wailsio/runtime", () => ({ Browser: browser }));

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSettings({
    version: 2, language: "vi", adapters: "auto", adapterGuids: [], bootstrap: ["1.1.1.1:53"], testDomain: "www.google.com",
    maxUpstreams: 5, updates: { checkApp: true, updateServerList: true },
  } as any);
  useGhost.getState().setInfo({ version: "0.7.0", portable: false, author: "a", repoUrl: "https://github.com/hashcott/ghostline" } as any);
});

test("Settings: report a problem opens the prefilled bug form", async () => {
  render(<Settings />);
  fireEvent.click(screen.getByRole("button", { name: "báo lỗi ↗" }));
  await waitFor(() => expect(browser.OpenURL).toHaveBeenCalledWith(FORM));
});

test("Logs: copy and report puts the log on the clipboard, then opens the form", async () => {
  const writeText = vi.fn(() => Promise.resolve());
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  useGhost.getState().pushLog({ time: "2026-10-11T00:00:00Z", source: "system", code: "X_TEST", params: null } as any);
  render(<Logs />);
  fireEvent.click(screen.getByRole("button", { name: "copy và báo lỗi ↗" }));
  await waitFor(() => expect(browser.OpenURL).toHaveBeenCalledWith(FORM));
  expect(writeText).toHaveBeenCalledTimes(1);
  expect(screen.getByText(/đã copy nhật ký/)).toBeInTheDocument();
});
