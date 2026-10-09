import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type InstallInfo } from "../app/api";
import { describeError } from "../i18n";
import { Banner, type BannerAction } from "./neon/Banner";

// The window's version whose service update was turned down: not asked
// again at start, only offered by the button.
const DECLINED = "ghostline.serviceUpdateDeclined";

function declined(): string {
  try {
    return localStorage.getItem(DECLINED) ?? "";
  } catch {
    return "";
  }
}

function decline(version: string) {
  try {
    localStorage.setItem(DECLINED, version);
  } catch {
    // Asked again next start.
  }
}

const selfInstalled = (i: InstallInfo) => (i.kind === "appimage" || i.kind === "tarball") && !i.packaged;

/**
 * ServiceOutdated: the Linux background service is an older release than
 * this window (a newer AppImage or tar.gz replaced the window, not the
 * service). A self-installed service is updated as the window opens
 * (pkexec asks for the password) unless that was turned down for this
 * version; until then the banner offers the update.
 */
export function ServiceOutdated() {
  const { t } = useTranslation();
  const [info, setInfo] = useState<InstallInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const asked = useRef(false);

  const update = async (i: InstallInfo) => {
    setBusy(true);
    setFailure("");
    try {
      // Asked of the service: as the window opens, the store has not had
      // its first snapshot yet.
      const status = String((await Service.GetSnapshot().catch(() => null))?.status ?? "");
      const wasProtected = status === "protected" || status === "degraded";
      await Service.InstallService();
      // The service restarted disconnected; reconnect before the window
      // reloads with the new service's settings and version.
      if (wasProtected) await Service.Connect().catch(() => {});
      window.location.reload();
    } catch (e) {
      decline(i.appVersion);
      setFailure(describeError(e));
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    let live = true;
    void Service.ServiceInstall().then(
      (i) => {
        if (!live) return;
        setInfo(i);
        if (i.outdated && selfInstalled(i) && declined() !== i.appVersion && !asked.current) {
          asked.current = true;
          void update(i);
        }
      },
      () => {},
    );
    return () => {
      live = false;
    };
  }, []);

  if (!info?.outdated) return null;
  const self = selfInstalled(info);
  const actions: BannerAction[] = self
    ? [{ label: t("service.update"), onClick: () => void update(info), primary: true, disabled: busy }]
    : [];
  return (
    <Banner tone="warn" actions={actions}>
      <div>{t("service.outdated", { service: info.serviceVersion, app: info.appVersion })}</div>
      {!self && <div>{t("service.packageUpdate")}</div>}
      {busy && <div>{t("service.updating")}</div>}
      {self && info.steamos && <div>{t("service.steamosHint")}</div>}
      {failure && <div>{failure}</div>}
    </Banner>
  );
}
