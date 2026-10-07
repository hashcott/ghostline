import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { Settings } from "./Settings";
import { useGhost } from "../../../app/store";
import { initI18n } from "../../../i18n";

const svc = vi.hoisted(() => ({
  ListCerts: vi.fn(() => Promise.resolve([])),
  CheckUpdateNow: vi.fn(),
  ListAdapters: vi.fn(() => Promise.resolve([])),
  DNSInfo: vi.fn(),
  SaveSettings: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../../app/api", () => ({ Service: svc }));
vi.mock("@wailsio/runtime", () => ({ Browser: { OpenURL: vi.fn() } }));

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSettings({
    version: 5, language: "vi", adapters: "auto", adapterGuids: [], bootstrap: ["1.1.1.1:53"], testDomain: "www.google.com",
    maxUpstreams: 5, updates: { checkApp: true, updateServerList: true },
  } as any);
  useGhost.getState().setInfo({ version: "0.6.0", portable: false, updateTag: "", updateUrl: "" } as any);
});

test("linux shows the DNS chain instead of the adapter picker", async () => {
  svc.DNSInfo.mockResolvedValue({ backend: "networkmanager", chain: "NetworkManager → systemd-resolved", interfaces: [], adapterPick: false, adapters: [] });
  render(<Settings />);
  expect(await screen.findByText("NetworkManager → systemd-resolved")).toBeInTheDocument();
  expect(screen.getByText("DNS hệ thống")).toBeInTheDocument();
  expect(screen.queryByText("chọn tay")).not.toBeInTheDocument();
});

test("windows keeps the adapter picker", async () => {
  svc.DNSInfo.mockResolvedValue({ backend: "windows", chain: "Windows", interfaces: ["Wi-Fi"], adapterPick: true, adapters: [] });
  render(<Settings />);
  expect(await screen.findByText("chọn tay")).toBeInTheDocument();
  expect(screen.queryByText("DNS hệ thống")).not.toBeInTheDocument();
});
