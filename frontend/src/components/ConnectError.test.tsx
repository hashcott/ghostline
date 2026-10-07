import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { initI18n } from "../i18n";
import { useGhost } from "../app/store";
import { ConnectError } from "./ConnectError";

const unreachable = {
  status: "error", step: 0, warnings: [], servers: [], since: "", latencyMs: 0, queries: 0,
  dpi: { enabled: false, running: false, preset: "light" }, blockedSites: [],
  error: { code: "DAEMON_UNREACHABLE" },
};

const svc = vi.hoisted(() => ({
  Connect: vi.fn(() => Promise.resolve()),
  GetSnapshot: vi.fn(() => Promise.resolve(null as unknown)),
  SaveSettings: vi.fn(() => Promise.resolve()),
}));
vi.mock("../app/api", () => ({ Service: svc }));

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  svc.Connect.mockClear();
  svc.GetSnapshot.mockClear();
  svc.GetSnapshot.mockImplementation(() => Promise.resolve(unreachable));
  useGhost.getState().setSnapshot(unreachable as any);
});

test("daemon unreachable shows its message and how to start the service", () => {
  render(<ConnectError onOpenServers={() => {}} onOpenLogs={() => {}} />);
  expect(screen.getByText("Dịch vụ nền của Ghostline chưa chạy")).toBeTruthy();
  expect(screen.getByText("Khởi động bằng lệnh: sudo systemctl start ghostline")).toBeTruthy();
});

// Everything the window loaded at start (settings, logs, app info) came
// from an unreachable daemon too: retry reloads the whole window.
test("retry reloads the window instead of connecting", () => {
  const reload = vi.fn();
  const real = window.location;
  Object.defineProperty(window, "location", { configurable: true, value: { ...real, reload } });
  try {
    render(<ConnectError onOpenServers={() => {}} onOpenLogs={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "thử lại" }));
    expect(reload).toHaveBeenCalledTimes(1);
    expect(svc.Connect).not.toHaveBeenCalled();
  } finally {
    Object.defineProperty(window, "location", { configurable: true, value: real });
  }
});
