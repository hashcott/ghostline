import { describe, expect, test } from "vitest";
import vi from "./vi.json";
import en from "./en.json";
import { initI18n, tCode } from "./index";

const keys = (o: Record<string, unknown>, p = ""): string[] =>
  Object.entries(o).flatMap(([k, v]) =>
    v && typeof v === "object" ? keys(v as Record<string, unknown>, p + k + ".") : [p + k],
  );

const goCodes = [
  "NOT_ADMIN", "PORT53_BUSY", "NO_SERVERS", "ENGINE_SELFTEST_FAILED", "DNS_INTERCEPTED", "SET_DNS_FAILED", "VERIFY_LEAK",
  "RESTORE_FAILED", "DPI_START_FAILED", "DPI_BLOCKED_BY_AV", "DPI_HASH_MISMATCH", "SERVERLIST_BAD_SIGNATURE",
  "UPDATE_CHECK_FAILED", "UPDATE_INSTALL_FAILED", "AUTOTUNE_NO_PRESET", "INTERNAL", "SETTINGS_RESET", "STATE_RESET", "NOT_CONNECTED",
  // Phase 3.
  "TOOL_BUSY", "LOOKUP_NOT_CONNECTED", "LOOKUP_BAD_NAME", "SCAN_TOO_MANY", "CFSCAN_NO_NETWORK", "CFSCAN_HOST_INVALID",
  "STAMP_INVALID", "IMPORT_INVALID", "IMPORT_WHILE_CONNECTED", "IMPORT_EXPIRED", "IMPORT_WRITE_FAILED", "EXPORT_WRITE_FAILED",
  "TEST_DOMAIN_BROKEN", "TEST_DOMAIN_NO_ADDRESS",
];

describe("i18n", () => {
  test("vi and en have identical keys", () => {
    expect(keys(en).sort()).toEqual(keys(vi).sort());
  });

  test("every DNS interceptor in internal/app/intercept.go has a hint", () => {
    for (const id of ["adguard", "avast", "avg", "yogadns", "portmaster"]) {
      expect((vi as any).errors.DNS_INTERCEPTED.hint[id], id).toBeTruthy();
    }
  });

  test("every Go error code has a message", () => {
    for (const c of goCodes) expect((vi as any).errors[c]?.message, c).toBeTruthy();
  });

  test("fixed copy from the spec", () => {
    expect(vi.simple.errorUnchanged).toBe("DNS của máy vẫn như cũ, không có gì bị thay đổi");
    expect(vi.simple.tapToConnect).toBe("bấm để kết nối");
  });

  test("tCode translates codes with params and falls back to the code", async () => {
    await initI18n("vi");
    expect(tCode("errors.NO_SERVERS.message", { checked: 142, elapsed: 20 })).toContain("142");
    expect(tCode("errors.UNKNOWN_CODE.message")).toBe("UNKNOWN_CODE");
  });
});

describe("tCode fallback", () => {
  test("a non-code error (HTTP 404) is shown as is, not as the key's last part", async () => {
    await initI18n("vi");
    expect(tCode("errors.HTTP 404.message")).toBe("HTTP 404");
    expect(tCode("errors.UNKNOWN_CODE.message")).toBe("UNKNOWN_CODE");
    expect(tCode("log.SOME_CODE")).toBe("SOME_CODE");
  });
});
