import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type InstallInfo } from "../app/api";
import { useGhost } from "../app/store";
import { describeError, tCode } from "../i18n";
import { Banner, type BannerAction } from "./neon/Banner";

const DAEMON_CODES = ["DAEMON_UNREACHABLE", "DAEMON_PROTOCOL_MISMATCH", "NOT_AUTHORIZED"];

type Props = { onOpenServers: () => void; onOpenLogs: () => void };

/** ConnectError shows the last connect error with the actions that fix it (both modes). */
export function ConnectError({ onOpenServers }: Props) {
  const { t } = useTranslation();
  const snap = useGhost((s) => s.snapshot);
  const settings = useGhost((s) => s.settings);
  if (String(snap.status) !== "error" || !snap.error) return null;
  const code = snap.error.code;

  const saveAndConnect = async (patch: (s: NonNullable<typeof settings>) => NonNullable<typeof settings>) => {
    if (!settings) return;
    const next = patch(structuredClone(settings));
    await Service.SaveSettings(next);
    useGhost.getState().setSettings(next);
    void Service.Connect();
  };

  // The Linux GUI cannot use its background service. Everything loaded at
  // start (settings, logs, app info) came back empty too, so retrying
  // reloads the window, which reconnects and loads it all again.
  if (DAEMON_CODES.includes(code)) return <DaemonError code={code} />;

  const actions: BannerAction[] = [{ label: t("common.retry"), onClick: () => void Service.Connect() }];
  if (code === "NO_SERVERS" && !settings?.fragmentDns?.enabled) {
    actions.push({ label: t("simple.enableFragment"), onClick: () => void saveAndConnect((s) => ({ ...s, fragmentDns: { ...s.fragmentDns, enabled: true } })) });
  }
  if (code === "NO_PINNED_SERVERS") {
    actions.unshift({ label: t("simple.openServers"), onClick: onOpenServers, primary: true });
    actions.push({ label: t("simple.disablePinnedOnly"), onClick: () => void saveAndConnect((s) => ({ ...s, pinnedOnly: false })) });
  }

  return (
    <Banner tone="err" actions={actions}>
      {tCode(`errors.${code}.message`, snap.error.params ?? undefined)}
      {code === "VERIFY_LEAK" && snap.error.params?.adapters ? (
        <div>{tCode("errors.VERIFY_LEAK.adapters", snap.error.params)}</div>
      ) : null}
    </Banner>
  );
}

/**
 * DaemonError: the Linux GUI cannot reach or use its background service.
 * Where this copy can, it offers to start, install or update the service
 * (pkexec, run by the GUI itself); on Windows ServiceInstall has no kind.
 */
function DaemonError({ code }: { code: string }) {
  const { t } = useTranslation();
  const [info, setInfo] = useState<InstallInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  useEffect(() => {
    void Service.ServiceInstall().then(setInfo, () => setInfo(null));
  }, []);

  const run = (action: () => Promise<void>) => async () => {
    setBusy(true);
    setFailure("");
    try {
      await action();
      window.location.reload();
    } catch (e) {
      setFailure(describeError(e));
    } finally {
      setBusy(false);
    }
  };
  const selfInstalled = info?.kind === "appimage" || info?.kind === "tarball";
  const actions: BannerAction[] = [{ label: t("common.retry"), onClick: () => window.location.reload() }];
  let note = "";
  if (info?.kind && code === "DAEMON_UNREACHABLE") {
    if (info.unit || info.kind === "package") {
      actions.unshift({ label: t("service.start"), onClick: run(Service.StartService), primary: true, disabled: busy });
    } else if (selfInstalled) {
      actions.unshift({ label: t("service.install"), onClick: run(Service.InstallService), primary: true, disabled: busy });
    }
  }
  if (info?.kind && code === "DAEMON_PROTOCOL_MISMATCH") {
    if (selfInstalled) {
      actions.unshift({ label: t("service.update"), onClick: run(Service.InstallService), primary: true, disabled: busy });
    } else {
      note = t("service.packageUpdate");
    }
  }
  const pkexec = actions.length > 1;
  return (
    <Banner tone="err" actions={actions}>
      <div>{tCode(`errors.${code}.message`)}</div>
      <div>{note || tCode(`errors.${code}.action`)}</div>
      {pkexec && info?.steamos && <div>{t("service.steamosHint")}</div>}
      {failure && <div>{failure}</div>}
    </Banner>
  );
}
