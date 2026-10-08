import { Events } from "@wailsio/runtime";
import { Service } from "./api";
import { useGhost } from "./store";
import { applyPlatform, initI18n } from "../i18n";

/** startBridge subscribes to Go events and loads the initial state. */
export function startBridge(): () => void {
  const s = useGhost.getState();
  const offs = [
    Events.On("state", (ev: any) => {
      applyPlatform(ev.data?.platform);
      useGhost.getState().setSnapshot(ev.data);
    }),
    Events.On("stats", (ev: any) => useGhost.getState().pushStats(ev.data)),
    Events.On("log", (ev: any) => useGhost.getState().pushLog(ev.data)),
    Events.On("query", (ev: any) => useGhost.getState().pushQuery(ev.data)),
    Events.On("scan:progress", (ev: any) => useGhost.getState().setScan(ev.data.running ? ev.data : null)),
    Events.On("dpi:autotune", (ev: any) => {
      useGhost.getState().setAutotune(ev.data);
      // Auto-tune saves the strategy it found: keep the UI's copy current,
      // or the next save from it would put the old one back.
      if (!ev.data?.running) void Service.GetSettings().then((st) => st && useGhost.getState().setSettings(st));
    }),
    Events.On("update", (ev: any) => useGhost.getState().setUpdate(ev.data)),
    Events.On("proxy:stats", (ev: any) => useGhost.getState().setProxyStats(ev.data)),
    Events.On("proxy:conn", (ev: any) => useGhost.getState().pushProxyConn(ev.data)),
    Events.On("rules:compiled", () => useGhost.getState().bumpRules()),
    Events.On("dnsserver:stats", (ev: any) => useGhost.getState().setDnsStats(ev.data)),
    Events.On("setup:countdown", (ev: any) => useGhost.getState().setSetup(ev.data)),
    Events.On("certs:changed", () => useGhost.getState().bumpCerts()),
    Events.On("lists:progress", () => useGhost.getState().bumpRules()),
    Events.On("tools:scan", (ev: any) => useGhost.getState().setAdvScan(ev.data)),
    Events.On("tools:cfscan", (ev: any) => useGhost.getState().setCfScan(ev.data)),
  ];
  void Service.GetSnapshot().then((snap) => {
    applyPlatform(snap?.platform);
    s.setSnapshot(snap);
  });
  void Service.GetSettings().then((st) => {
    s.setSettings(st);
    void initI18n(st.language === "en" ? "en" : "vi");
  });
  void Service.GetLogs().then((l) => s.setLogs(l ?? []));
  void Service.AppInfo().then(s.setInfo);
  return () => offs.forEach((off) => off());
}
