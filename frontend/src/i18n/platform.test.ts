import { beforeAll, expect, test } from "vitest";
import i18n, { applyPlatform, initI18n } from "./index";
import en from "./en.json";
import vi from "./vi.json";
import enLinux from "./en.linux.json";
import viLinux from "./vi.linux.json";

type Tree = { [k: string]: string | Tree };
const keys = (o: Tree, p = ""): string[] =>
  Object.entries(o).flatMap(([k, v]) => (typeof v === "string" ? [p + k] : keys(v, p + k + ".")));
const values = (o: Tree): string[] => Object.values(o).flatMap((v) => (typeof v === "string" ? [v] : values(v)));

beforeAll(() => initI18n("en"));

test("windows keeps base strings", () => {
  applyPlatform("windows");
  expect(i18n.t("errors.SYSPROXY_FAILED.message")).toBe("could not set the Windows system proxy");
});

test("linux overrides replace Windows wording", async () => {
  applyPlatform("linux");
  expect(i18n.t("errors.SYSPROXY_FAILED.message")).not.toMatch(/Windows/);
  expect(i18n.t("proxy.lan.public")).not.toMatch(/Windows/);
  await i18n.changeLanguage("vi");
  expect(i18n.t("errors.SYSPROXY_FAILED.message")).not.toMatch(/Windows/);
  await i18n.changeLanguage("en");
});

test("linux bundles have the same keys in vi and en", () => {
  expect(keys(viLinux as Tree).sort()).toEqual(keys(enLinux as Tree).sort());
});

test("every linux key exists in the base bundle", () => {
  const base = new Set(keys(en as Tree));
  const baseVi = new Set(keys(vi as Tree));
  for (const k of keys(enLinux as Tree)) {
    expect(base.has(k), k).toBe(true);
    expect(baseVi.has(k), k).toBe(true);
  }
});

test("no linux override mentions Windows", () => {
  for (const v of [...values(enLinux as Tree), ...values(viLinux as Tree)]) {
    expect(v).not.toMatch(/Windows|Defender|WinDivert|svchost|Hyper-V/);
  }
});
