import { beforeAll, expect, test, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { initI18n } from "./i18n";

const svc = vi.hoisted(() => ({
  Connect: vi.fn(() => Promise.resolve()),
  GetSnapshot: vi.fn(() =>
    Promise.resolve({ status: "disconnected", step: 0, warnings: [], servers: [], since: "", latencyMs: 0, queries: 0,
      dpi: { enabled: false, running: false, preset: "light" }, blockedSites: [] }),
  ),
  GetSettings: vi.fn(() => Promise.resolve({ language: "vi", mode: "simple", probeSites: [], dpi: {}, fragmentDns: {} })),
  GetLogs: vi.fn(() => Promise.resolve([])),
  AppInfo: vi.fn(() => Promise.resolve({ version: "test" })),
  ServiceInstall: vi.fn(() => Promise.resolve({ kind: "", outdated: false })),
  SetMode: vi.fn(() => Promise.resolve()),
  SaveSettings: vi.fn(() => Promise.resolve()),
  TermsAccepted: vi.fn(() => Promise.resolve(true)),
  InstallService: vi.fn(() => Promise.resolve()),
}));
vi.mock("./app/api", () => ({ Service: svc }));

beforeAll(() => initI18n("vi"));

test("mounting the app never connects on its own", async () => {
  const { default: App } = await import("./App");
  render(<App />);
  await act(async () => {
    await new Promise((r) => setTimeout(r, 50));
  });
  expect(svc.Connect).not.toHaveBeenCalled();
});

// First start of an AppImage: no service yet, so the terms cannot be
// recorded until it is installed. The install button must not hide
// behind the terms (issue #7).
test("terms not yet accepted and no service: the install button is there", async () => {
  cleanup();
  svc.GetSnapshot.mockImplementation(() =>
    Promise.resolve({ status: "error", step: 0, warnings: [], servers: [], since: "", latencyMs: 0, queries: 0,
      dpi: { enabled: false, running: false, preset: "light" }, blockedSites: [],
      error: { code: "DAEMON_UNREACHABLE" } } as any),
  );
  svc.ServiceInstall.mockImplementation(() => Promise.resolve({ kind: "appimage", outdated: false } as any));
  svc.TermsAccepted.mockImplementation(() => Promise.resolve(false));
  const { default: App } = await import("./App");
  render(<App />);
  expect(await screen.findByTestId("terms-text")).toBeTruthy();
  expect(await screen.findByRole("button", { name: "cài dịch vụ" })).toBeTruthy();
});
