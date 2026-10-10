import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Browser } from "@wailsio/runtime";
import { Service } from "../app/api";
import { useGhost } from "../app/store";
import { useUpdate } from "../app/format";
import { describeError } from "../i18n";

/**
 * UpdateAction is the "new version" button. Builds from the Windows
 * installer download, check and install the release in one click (the app
 * closes and the installer opens it again); others, and a failed install,
 * open the release page.
 */
export function UpdateAction({ className, style }: { className?: string; style?: React.CSSProperties }) {
  const { t } = useTranslation();
  const update = useUpdate();
  const self = useGhost((s) => s.info?.selfUpdate);
  const [state, setState] = useState<{ busy?: boolean; error?: string } | null>(null);
  if (!update) return null;
  const openPage = () => void Browser.OpenURL(update.url);

  if (!self) {
    return (
      <button className={className} style={style} onClick={openPage}>
        {t("settings.update", { tag: update.tag })}
      </button>
    );
  }
  if (state?.error) {
    return (
      <button className={className} style={style} title={state.error} onClick={openPage}>
        {t("settings.installFailed", { tag: update.tag })}
      </button>
    );
  }
  const install = () => {
    if (!window.confirm(t("settings.installConfirm", { tag: update.tag }))) return;
    setState({ busy: true });
    // Success never resolves visibly: Ghostline quits for the installer.
    Service.InstallUpdate().catch((e) => setState({ error: describeError(e) }));
  };
  return (
    <button className={className} style={style} disabled={state?.busy} onClick={install}>
      {state?.busy ? t("settings.installing", { tag: update.tag }) : t("settings.installUpdate", { tag: update.tag })}
    </button>
  );
}
