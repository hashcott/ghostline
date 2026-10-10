import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { initI18n } from "../i18n";
import { useGhost } from "../app/store";

const svc = vi.hoisted(() => ({
  TermsAccepted: vi.fn(() => Promise.resolve(false)),
  AcceptTerms: vi.fn(() => Promise.resolve()),
  GetSnapshot: vi.fn(() => new Promise(() => {})),
  GetSettings: vi.fn(() => new Promise(() => {})),
  GetLogs: vi.fn(() => Promise.resolve([])),
  AppInfo: vi.fn(() => Promise.resolve({ version: "test" })),
  ServiceInstall: vi.fn(() => Promise.resolve({ kind: "", outdated: false })),
}));
vi.mock("../app/api", () => ({ Service: svc }));
const runtime = vi.hoisted(() => ({
  Application: { Quit: vi.fn(() => Promise.resolve()) },
  Browser: { OpenURL: vi.fn(() => Promise.resolve()) },
  Window: { Close: vi.fn(), Minimise: vi.fn(), ToggleMaximise: vi.fn() },
  Events: { On: vi.fn(() => () => {}) },
}));
vi.mock("@wailsio/runtime", () => runtime);
vi.mock("../app/bridge", () => ({ startBridge: () => () => {} }));

const settings = { language: "vi", mode: "simple", probeSites: [], dpi: {}, fragmentDns: {}, terms: { ackVersion: 0 } };

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
});

async function mountApp() {
  const { default: App } = await import("../App");
  render(<App />);
  await act(async () => {
    useGhost.getState().setSettings(structuredClone(settings) as any);
  });
}

test("the terms stand in for the app until they are read, ticked and accepted", async () => {
  await mountApp();
  const accept = await screen.findByRole("button", { name: "đồng ý và tiếp tục" });
  expect(screen.getByText(/hướng dẫn hay chia sẻ cho người khác/)).toBeInTheDocument();
  expect(accept).toBeDisabled();
  fireEvent.click(screen.getByRole("checkbox", { name: /tôi đã đọc, hiểu và đồng ý/i }));
  expect(accept).toBeDisabled();
  fireEvent.scroll(screen.getByTestId("terms-text"));
  expect(accept).toBeEnabled();
  fireEvent.click(accept);
  await waitFor(() => expect(svc.AcceptTerms).toHaveBeenCalled());
  await waitFor(() => expect(screen.queryByTestId("terms-text")).not.toBeInTheDocument());
});

test("declining quits Ghostline", async () => {
  await mountApp();
  fireEvent.click(await screen.findByRole("button", { name: "không đồng ý, thoát" }));
  expect(runtime.Application.Quit).toHaveBeenCalled();
  expect(svc.AcceptTerms).not.toHaveBeenCalled();
});

test("terms accepted before: the app opens straight away", async () => {
  svc.TermsAccepted.mockResolvedValueOnce(true);
  await mountApp();
  await waitFor(() => expect(svc.TermsAccepted).toHaveBeenCalled());
  await act(async () => {});
  expect(screen.queryByTestId("terms-text")).not.toBeInTheDocument();
});

test("a failed accept stays on the terms and says why", async () => {
  svc.AcceptTerms.mockRejectedValueOnce(new Error("disk full"));
  await mountApp();
  fireEvent.click(await screen.findByRole("checkbox", { name: /tôi đã đọc/i }));
  fireEvent.scroll(screen.getByTestId("terms-text"));
  fireEvent.click(screen.getByRole("button", { name: "đồng ý và tiếp tục" }));
  expect(await screen.findByText("disk full")).toBeInTheDocument();
  expect(screen.getByTestId("terms-text")).toBeInTheDocument();
});
