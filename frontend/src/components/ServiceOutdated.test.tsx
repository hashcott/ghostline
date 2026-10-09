import { afterEach, beforeAll, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { initI18n } from "../i18n";
import { useGhost } from "../app/store";
import { ServiceOutdated } from "./ServiceOutdated";

const svc = vi.hoisted(() => ({
  Connect: vi.fn(() => Promise.resolve()),
  ServiceInstall: vi.fn(() => Promise.resolve({})),
  InstallService: vi.fn(() => Promise.resolve()),
  GetSnapshot: vi.fn(() => Promise.resolve({ status: "disconnected" })),
}));
vi.mock("../app/api", () => ({ Service: svc }));

const appimage = {
  kind: "appimage", unit: true, steamos: false, packaged: false,
  serviceVersion: "0.6.0", appVersion: "0.6.2", outdated: true,
};
const reload = vi.fn();
const realLocation = window.location;

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  localStorage.clear();
  reload.mockClear();
  Object.defineProperty(window, "location", { configurable: true, value: { ...realLocation, reload } });
  svc.Connect.mockClear();
  svc.InstallService.mockReset();
  svc.InstallService.mockImplementation(() => Promise.resolve());
  svc.ServiceInstall.mockImplementation(() => Promise.resolve(appimage));
  svc.GetSnapshot.mockImplementation(() => Promise.resolve({ status: "disconnected" }));
  useGhost.getState().setSnapshot({ ...useGhost.getState().snapshot, status: "disconnected" } as any);
});
afterEach(() => {
  cleanup();
  Object.defineProperty(window, "location", { configurable: true, value: realLocation });
});

test("a current service shows nothing and is not updated", async () => {
  svc.ServiceInstall.mockImplementation(() => Promise.resolve({ ...appimage, serviceVersion: "0.6.2", outdated: false }));
  const { container } = render(<ServiceOutdated />);
  await waitFor(() => expect(svc.ServiceInstall).toHaveBeenCalled());
  expect(container.textContent).toBe("");
  expect(svc.InstallService).not.toHaveBeenCalled();
});

// Opening a newer AppImage is the update: pkexec asks for the password.
test("an older self-installed service is updated at once, then the window reloads", async () => {
  render(<ServiceOutdated />);
  expect(await screen.findByText(/bản 0\.6\.0, cũ hơn app \(0\.6\.2\)/)).toBeTruthy();
  await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
  expect(svc.InstallService).toHaveBeenCalledTimes(1);
  expect(svc.Connect).not.toHaveBeenCalled();
});

// Restarting the service drops protection; it is turned back on. The
// window has not had its first snapshot yet when it opens, so the
// service is asked.
test("protection that was on is reconnected after the update", async () => {
  svc.GetSnapshot.mockImplementation(() => Promise.resolve({ status: "protected" }));
  render(<ServiceOutdated />);
  await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
  expect(svc.Connect).toHaveBeenCalledTimes(1);
  expect(svc.InstallService.mock.invocationCallOrder[0]).toBeLessThan(svc.Connect.mock.invocationCallOrder[0]);
});

test("a cancelled update is not asked again for that version; the button stays", async () => {
  svc.InstallService.mockImplementation(() => Promise.reject(new Error("PKEXEC_FAILED: sudo /tmp/x/ghostlined --install-system")));
  render(<ServiceOutdated />);
  await waitFor(() => expect(svc.InstallService).toHaveBeenCalledTimes(1));
  expect(await screen.findByText(/sudo \/tmp\/x\/ghostlined --install-system/)).toBeTruthy();
  expect(reload).not.toHaveBeenCalled();
  cleanup();

  render(<ServiceOutdated />);
  const button = await screen.findByRole("button", { name: "cập nhật dịch vụ" });
  expect(svc.InstallService).toHaveBeenCalledTimes(1);
  svc.InstallService.mockImplementation(() => Promise.resolve());
  fireEvent.click(button);
  await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
  expect(svc.InstallService).toHaveBeenCalledTimes(2);
});

test("a newer release asks again after an earlier one was cancelled", async () => {
  localStorage.setItem("ghostline.serviceUpdateDeclined", "0.6.1");
  render(<ServiceOutdated />);
  await waitFor(() => expect(svc.InstallService).toHaveBeenCalledTimes(1));
});

// --install-system leaves a package's service alone.
test("a packaged service is not updated from the window", async () => {
  svc.ServiceInstall.mockImplementation(() => Promise.resolve({ ...appimage, packaged: true }));
  render(<ServiceOutdated />);
  expect(await screen.findByText("Cập nhật Ghostline bằng trình quản lý gói")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "cập nhật dịch vụ" })).toBeNull();
  expect(svc.InstallService).not.toHaveBeenCalled();
});
