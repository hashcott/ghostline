import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import vi from "./vi.json";
import en from "./en.json";
import viLinux from "./vi.linux.json";
import enLinux from "./en.linux.json";

export type Lang = "vi" | "en";

export function initI18n(lang: Lang) {
  if (i18n.isInitialized) return i18n.changeLanguage(lang);
  return i18n.use(initReactI18next).init({
    resources: { vi: { translation: vi }, en: { translation: en } },
    lng: lang,
    fallbackLng: "en",
    interpolation: { escapeValue: false }, // React escapes
    returnNull: false,
  });
}

// Wording that differs by OS: the backend's platform picks a bundle that
// overrides the base strings; components never branch on the OS.
const platformBundles: Record<string, Record<Lang, object>> = {
  linux: { vi: viLinux, en: enLinux },
};
let platformApplied = "";

/** applyPlatform loads the wording for the backend's OS, once. */
export function applyPlatform(platform: string | undefined) {
  if (!platform || platform === platformApplied) return;
  platformApplied = platform;
  const b = platformBundles[platform];
  if (!b) return;
  for (const lang of ["vi", "en"] as Lang[]) i18n.addResourceBundle(lang, "translation", b[lang], true, true);
}

/**
 * tCode translates a key built from a Go code (errors.<CODE>.message,
 * log.<CODE>, step.<n>). A missing key falls back to the code itself.
 */
export function tCode(key: string, params?: Record<string, unknown>): string {
  if (i18n.exists(key)) return i18n.t(key, params as any) as string;
  // Untranslated: show the code (or free text such as "HTTP 404") between
  // the prefix and the suffix, never the suffix ("message") alone.
  const parts = key.split(".");
  if (parts.length > 2) return parts.slice(1, -1).join(".");
  return parts.length > 1 ? parts[1] : key;
}

export default i18n;

/**
 * describeError turns an error from a Go binding ("CODE" or "CODE: cause")
 * into the translated message; anything else is shown as-is.
 */
export function describeError(e: unknown): string {
  const msg = e instanceof Error ? e.message : String(e);
  const m = /^([A-Z][A-Z0-9_]{2,})(?::\s*([\s\S]*))?$/.exec(msg.trim());
  if (m && i18n.exists(`errors.${m[1]}.message`)) {
    const text = i18n.t(`errors.${m[1]}.message`) as string;
    return m[2] ? `${text} (${m[2]})` : text;
  }
  return msg;
}
