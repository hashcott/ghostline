import { useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service } from "../../app/api";
import { confirmDisconnect } from "../../app/disconnect";
import { useGhost } from "../../app/store";
import { isConnected, powerState, serverSummary, useUptime } from "../../app/format";
import { tCode } from "../../i18n";
import { useStrategyName } from "../../app/strategies";
import { PowerButton } from "../../components/neon/PowerButton";
import { TerminalPanel } from "../../components/neon/TerminalPanel";
import { Banner } from "../../components/neon/Banner";
import { UpdateAction } from "../../components/UpdateAction";
import { ConnectError } from "../../components/ConnectError";
import { Warnings } from "../../components/Warnings";
import { ProtectionLevels } from "./ProtectionLevels";
import { useFirstRunTune } from "./useFirstRunTune";
import css from "./SimpleView.module.css";

/**
 * useSmoothHeight follows an element's height so its wrapper can animate to
 * it: when the panel under the levels grows (connect steps) or shrinks, the
 * power button above glides instead of jumping.
 */
function useSmoothHeight<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [height, setHeight] = useState<number>();
  // Measured after every render, before paint (state changes here), and on
  // resize (parts that update by themselves, like the error banners).
  useLayoutEffect(() => {
    const h = ref.current?.offsetHeight;
    if (h !== undefined && h !== height) setHeight(h);
  });
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => setHeight(el.offsetHeight));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, height] as const;
}

export function SimpleView({
  onOpenLogs,
  onOpenServers = () => {},
  onOpenFull = () => {},
}: {
  onOpenLogs: () => void;
  onOpenServers?: () => void;
  onOpenFull?: () => void;
}) {
  const { t } = useTranslation();
  const snap = useGhost((s) => s.snapshot);
  const settings = useGhost((s) => s.settings);
  const latency = useGhost((s) => s.latency);
  const autotune = useGhost((s) => s.autotune);
  const bannerDismissed = useGhost((s) => s.bannerDismissed);
  const tuneName = useStrategyName(autotune?.engine || snap.dpi?.engine || settings?.dpi?.engine || "goodbyedpi", autotune?.preset);
  const strategyName = useStrategyName(snap.dpi?.engine || settings?.dpi?.engine || "goodbyedpi", snap.dpi?.preset);
  const dismissBanner = useGhost((s) => s.dismissBanner);
  const uptime = useUptime(snap.since);
  const status = String(snap.status);
  // The first LAN (non-loopback) DNS server address, without its port.
  const lanDNS = (snap.dnsServer?.running ? snap.dnsServer.addrs ?? [] : [])
    .map((x) => x.replace(/:\d+$/, "").replace(/^\[|\]$/g, ""))
    .find((h) => h !== "127.0.0.1" && h !== "::1");
  const label = `[ ${t(`status.${status}`)} ]`;
  const firstRun = useFirstRunTune(status);
  const [bottomRef, bottomHeight] = useSmoothHeight<HTMLDivElement>();
  // The first-run tune shows as one more connect step, not as banners.
  const tuneBanners = !firstRun;

  const onPower = () => {
    if (status === "connecting") void Service.CancelConnect();
    else if (isConnected(status)) { if (confirmDisconnect(t)) void Service.Disconnect(); }
    else if (status !== "disconnecting") void Service.Connect();
  };


  const lastLatency = latency.length ? latency[latency.length - 1] : snap.latencyMs;
  const blocked = snap.blockedSites ?? [];

  // The slow steps say why, in one dim line under them.
  const hintLine = (text: string) => (
    <span key="hint" data-testid="step-hint" className={css.stepHint}>
      {text}
    </span>
  );
  const stepLines = (current: number, extra?: string) => [
    ...[1, 2, 3, 4, 5, 6, 7].flatMap((n) => {
      const state = n < current ? "done" : n === current ? "current" : "todo";
      const scanning = n === 2 && state === "current" && !!snap.pickTotal;
      return [
        <span key={n} data-step={state} className={css[state]}>
          <span className={css.mark}>{state === "done" ? "✓ " : state === "current" ? "› " : "  "}</span>
          <span>
            {t(`step.${n}`)}
            {scanning ? ` ${snap.pickDone}/${snap.pickTotal}` : ""}
          </span>
        </span>,
        ...(scanning ? [hintLine(t("simple.stepHint.scan"))] : []),
      ];
    }),
    ...(extra
      ? [
          <span key="extra" data-step="current" className={css.current}>
            <span className={css.mark}>{"› "}</span>
            <span>{extra}</span>
          </span>,
          hintLine(t("simple.stepHint.firstRun")),
        ]
      : []),
  ];

  let below;
  if (firstRun) {
    below = (
      <TerminalPanel
        className={css.steps}
        lines={stepLines(
          8,
          firstRun === "tune" && autotune?.running
            ? t("simple.firstRun.tune", { index: autotune.index, total: autotune.total })
            : t("simple.firstRun.probe"),
        )}
      />
    );
  } else if (status === "connecting") {
    below = (
      <TerminalPanel className={css.steps} lines={stepLines(snap.step)} />
    );
  } else if (isConnected(status)) {
    below = (
      <TerminalPanel
        rows={[
          { k: t("simple.server"), v: serverSummary(snap.servers) },
          { k: t("simple.latency"), v: t("common.ms", { value: lastLatency }) },
          { k: t("simple.dpi"), v: snap.dpi.running ? `${strategyName} ✓` : t("common.off"), tone: snap.dpi.running ? "ok" : "dim" },
          { k: t("simple.uptime"), v: uptime },
          ...(snap.proxy?.running ? [{ k: t("simple.proxy"), v: snap.proxy.addr }] : []),
          ...(lanDNS ? [{ k: t("simple.dnsLan"), v: lanDNS }] : []),
        ]}
      />
    );
  } else if (status === "error") {
    below = (
      <TerminalPanel
        rows={[{ k: t("common.details"), v: <button className={css.link} onClick={onOpenLogs}>{t("common.openLogs")}</button> }]}
      />
    );
  } else {
    below = (
      <TerminalPanel
        rows={[
          { k: t("simple.server"), v: t("simple.serverAuto") },
          { k: t("simple.dpi"), v: settings?.dpi.enabled ? t("common.on") : t("common.off"), tone: settings?.dpi.enabled ? "ok" : "dim" },
        ]}
      />
    );
  }


  return (
    <section className={css.view}>
      <Warnings />
      <div className={css.hero}>
        <PowerButton state={powerState(status)} label={label} onClick={onPower} disabled={status === "disconnecting"} />
        <div className={css.status} data-status={status}>
          {label}
          {(status === "connecting" || isConnected(status)) && <span className={css.cursor}>_</span>}
        </div>
        <div className={css.sub}>
          {status === "disconnected" && t("simple.tapToConnect")}
          {status === "connecting" && t("simple.tapToCancel")}
          {isConnected(status) && t("simple.encrypted")}
          {status === "error" && snap.error?.code !== "RESTORE_FAILED" && t("simple.errorUnchanged")}
        </div>
        <ProtectionLevels onOpenFull={onOpenFull} disabled={status === "connecting" || status === "disconnecting"} />
      </div>
      {/* The wrapper animates to the panel's height (see useSmoothHeight). */}
      <div className={css.bottomWrap} style={bottomHeight === undefined ? undefined : { height: bottomHeight }}>
        <div ref={bottomRef} className={css.bottom}>
          <ConnectError onOpenServers={onOpenServers} onOpenLogs={onOpenLogs} />
          {tuneBanners && isConnected(status) && autotune?.running && (
            <Banner tone="warn">{t("simple.autotuning", { preset: tuneName, index: autotune.index, total: autotune.total })}</Banner>
          )}
          {tuneBanners && isConnected(status) && autotune && !autotune.running && !autotune.error && autotune.preset && (
            <Banner tone="ok">{t("dpi.autotuneDone", { preset: tuneName, engine: autotune.engine === "goodbyedpi" ? "GoodbyeDPI" : autotune.engine })}</Banner>
          )}
          {isConnected(status) && autotune && !autotune.running && autotune.error && (
            <Banner tone="err">{tCode(`errors.${autotune.error.code}.message`)}</Banner>
          )}
          {tuneBanners && isConnected(status) && blocked.length > 0 && !bannerDismissed && !autotune?.running && (
            <Banner
              tone="warn"
              actions={[
                { label: t("simple.autotune"), onClick: () => void Service.StartAutotune(), primary: true },
                { label: t("common.dismiss"), onClick: () => dismissBanner(true) },
              ]}
            >
              {t("simple.blocked", { count: blocked.length, total: settings?.probeSites?.length ?? blocked.length })}
            </Banner>
          )}
          {below}
          <UpdateAction className={css.update} />
        </div>
      </div>
    </section>
  );
}
