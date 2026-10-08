import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type DPIInfo, type ProbeResult } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { saveSettings } from "../../../app/settings";
import { describeError, tCode } from "../../../i18n";
import { useStrategyName } from "../../../app/strategies";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import css from "../full.module.css";

// entries counts domains in blacklist text the way Go does: one per line,
// blank lines and # comments skipped.
const entries = (text: string) => text.split("\n").filter((l) => l.trim() && !l.trim().startsWith("#")).length;

// GoodbyeDPI's numbered modes are not autotune steps, so the engine does not
// list them; they stay selectable here.
const GOODBYE_MODES = ["mode1", "mode2", "mode3", "mode4", "mode5", "mode6"];
const ENGINE_NAME: Record<string, string> = { zapret2: "zapret2", goodbyedpi: "GoodbyeDPI" };
const ENGINE_EXE: Record<string, string> = { zapret2: "winws2.exe", goodbyedpi: "goodbyedpi.exe" };

type Strategy = { id: string; name: Record<string, string> | null };
type Zapret2 = { strategy: string; customArgs: string; autoHostlist: boolean };

export function Dpi() {
  const { t, i18n } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const snap = useGhost((s) => s.snapshot);
  const autotune = useGhost((s) => s.autotune);
  const dpi = settings?.dpi;
  // The engines this OS has come from the backend; the buttons wait for
  // them so no engine flashes up that this OS lacks.
  const [info, setInfo] = useState<DPIInfo | null>(null);
  const available = (info?.engines ?? []).map((e) => e.id);
  // Settings from before zapret2 have no engine: they are GoodbyeDPI's. A
  // backup from another OS may name an engine this one lacks: the backend
  // runs its first engine then, and so does the page.
  const wanted: string = dpi?.engine || "goodbyedpi";
  const engine: string = info && available.length > 0 && !available.includes(wanted) ? available[0] : wanted;
  const z: Zapret2 = dpi?.zapret2 ?? { strategy: "", customArgs: "", autoHostlist: false };
  const isZ = engine === "zapret2";
  const strategy = (isZ ? z.strategy : dpi?.preset) ?? "";
  const customArgs = (isZ ? z.customArgs : dpi?.customArgs) ?? "";
  const autoOn = isZ && dpi?.scope === "blacklist" && z.autoHostlist;
  const [custom, setCustom] = useState(customArgs);
  const [loaded, setLoaded] = useState<{ engine: string; list: Strategy[] }>({ engine: "", list: [] });
  const [autoSites, setAutoSites] = useState<string[]>([]);
  const [engineDir, setEngineDir] = useState("");
  const runningName = useStrategyName(snap.dpi?.engine || engine, snap.dpi?.preset);
  const tuneName = useStrategyName(autotune?.engine || engine, autotune?.preset);
  const [error, setError] = useState<string | null>(null);
  const [shown, setShown] = useState<{ engine: string; args: string[] }>({ engine: "", args: [] });
  const [probe, setProbe] = useState<ProbeResult[]>([]);
  // saved: the blacklist file as stored; draft: the editor text. newList is
  // true while a first list is being written: scope switches to blacklist
  // only once it is saved.
  const [saved, setSaved] = useState("");
  const [draft, setDraft] = useState("");
  const [newList, setNewList] = useState(false);
  const [listNote, setListNote] = useState<string | null>(null);
  const [sites, setSites] = useState((settings?.probeSites ?? []).join("\n"));

  useEffect(() => {
    if (!dpi) return;
    Service.PreviewDPIArgs(engine, strategy, customArgs, dpi.scope, autoOn)
      .then((a) => setShown({ engine, args: a ?? [] }))
      .catch(() => setShown({ engine, args: [] }));
  }, [engine, strategy, customArgs, dpi?.scope, autoOn]);
  useEffect(() => {
    setCustom(customArgs);
    Service.DPIStrategies(engine)
      .then((l) => setLoaded({ engine, list: (l ?? []) as Strategy[] }))
      .catch(() => setLoaded({ engine, list: [] }));
  }, [engine]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!autoOn) return;
    Service.GetDPIAutoHostlist()
      .then((l) => setAutoSites(l ?? []))
      .catch(() => setAutoSites([]));
  }, [autoOn, snap.dpi?.running]);
  useEffect(() => {
    if (!dpi) return;
    Service.DPIInfo()
      .then((i) => setInfo(i ?? null))
      .catch(() => setInfo(null));
  }, [!!dpi]); // eslint-disable-line react-hooks/exhaustive-deps
  const fallback = !!snap.dpi?.fallback;
  useEffect(() => {
    if (!fallback) return;
    Service.DPIEngineDir("zapret2")
      .then((d) => setEngineDir(d ?? ""))
      .catch(() => setEngineDir(""));
  }, [fallback]);
  useEffect(() => {
    Service.GetDPIBlacklist()
      .then((b) => {
        setSaved(b ?? "");
        setDraft((d) => d || (b ?? "")); // keep what was typed meanwhile
      })
      .catch(() => {});
  }, []);

  if (!settings || !dpi) return null;

  const save = async (patch: Parameters<typeof saveSettings>[0]) => setError(await saveSettings(patch));
  const running = snap.dpi?.running;
  const setEnabled = (on: boolean) => {
    const s = useGhost.getState().settings;
    if (s) useGhost.getState().setSettings({ ...s, dpi: { ...s.dpi, enabled: on } });
  };
  const setZ = (patch: Partial<Zapret2>) => save((s) => ({ ...s, dpi: { ...s.dpi, zapret2: { ...z, ...patch } } }));
  const setStrategy = (v: string) => (isZ ? void setZ({ strategy: v }) : void save((s) => ({ ...s, dpi: { ...s.dpi, preset: v } })));
  const saveCustom = () =>
    isZ ? void setZ({ customArgs: custom }) : void save((s) => ({ ...s, dpi: { ...s.dpi, customArgs: custom } }));
  const pickEngine = (e: string) => {
    setError(null);
    // Choosing by hand means the user knows both engines: the hint is done.
    if (e !== engine) void save((s) => ({ ...s, dpi: { ...s.dpi, engine: e, hideEngineHint: true } }));
  };
  const saveAutoSites = (list: string[]) => {
    setAutoSites(list);
    Service.SaveDPIAutoHostlist(list).catch((e) => setError(describeError(e)));
  };
  // Results for the engine just left are dropped, so a switch never shows
  // one engine's args or strategies under the other's name.
  const strategies = loaded.engine === engine ? loaded.list : [];
  const preview = shown.engine === engine ? shown.args : [];
  const nameOf = (st: Strategy) => st.name?.[i18n.language] || st.name?.en || st.id;
  const options: { id: string; label: string }[] = [
    ...strategies.map((st) => ({ id: st.id, label: nameOf(st) })),
    ...(isZ ? [] : GOODBYE_MODES.map((m) => ({ id: m, label: t(`dpi.presets.${m}`) }))),
    { id: "custom", label: t("dpi.presets.custom") },
  ];
  if (strategy && !options.some((o) => o.id === strategy))
    options.unshift({ id: strategy, label: loaded.engine === engine ? t(`dpi.presets.${strategy}`, strategy) : "…" });
  const runningEngine = snap.dpi?.engine || engine;
  const suggested = () => (settings.probeSites ?? []).map((s) => s + "\n").join("");
  const showList = dpi.scope === "blacklist" || newList;
  const pickScope = (sc: "all" | "blacklist") => {
    setError(null);
    setListNote(null);
    if (sc === "all") {
      setNewList(false);
      if (dpi.scope !== "all") void save((s) => ({ ...s, dpi: { ...s.dpi, scope: "all" } }));
      return;
    }
    if (dpi.scope === "blacklist") return;
    if (entries(saved) > 0) {
      void save((s) => ({ ...s, dpi: { ...s.dpi, scope: "blacklist" } }));
      return;
    }
    if (entries(draft) === 0) setDraft(suggested());
    setNewList(true);
  };
  const saveList = async () => {
    setListNote(null);
    if (entries(draft) === 0) {
      setError(tCode("errors.DPI_BLACKLIST_EMPTY.message"));
      return;
    }
    try {
      await Service.SaveDPIBlacklist(draft);
    } catch (e) {
      setError(describeError(e));
      return;
    }
    setError(null);
    setSaved(draft);
    if (newList) {
      setNewList(false);
      await save((s) => ({ ...s, dpi: { ...s.dpi, scope: "blacklist" } }));
    }
    setListNote(
      !running
        ? t("dpi.blacklistSaved")
        : runningEngine === "zapret2"
          ? t("dpi.blacklistSavedReload")
          : t("dpi.blacklistSavedRestart", { engine: ENGINE_NAME[runningEngine] }),
    );
  };
  const cancelList = () => {
    setError(null);
    setListNote(null);
    setNewList(false);
    setDraft(entries(saved) > 0 ? saved : suggested());
  };
  const toggleDPI = (on: boolean) => {
    setError(null);
    setEnabled(on);
    Service.SetDPIEnabled(on).catch((e) => {
      setEnabled(!on);
      setError(describeError(e));
    });
  };

  return (
    <div className={css.page}>
      <div className={css.head}>{t("dpi.title")}</div>
      <div className={css.dim}>{t("dpi.legalNote")}</div>

      <div className={css.panel}>
        <div className={css.panelTitle}>
          <span>{t("dpi.engineTitle")}</span>
          <Toggle label={t("dpi.engineTitle")} checked={dpi.enabled} onChange={toggleDPI} />
        </div>
        {fallback && info?.avExclusions !== false && (
          <div className={css.bad}>
            <div>⚠ {t("dpi.fallback.text", { dir: engineDir })}</div>
            <Chip onClick={() => void Service.RetryZapret2().catch((e) => setError(describeError(e)))}>{t("dpi.fallback.retry")}</Chip>
          </div>
        )}
        {!isZ && !dpi.hideEngineHint && (
          <div className={css.row}>
            <span className={css.dim}>ⓘ {t("dpi.engineHint.text")}</span>
            <Chip onClick={() => pickEngine("zapret2")}>{t("dpi.engineHint.try")}</Chip>
            <Chip onClick={() => void save((s) => ({ ...s, dpi: { ...s.dpi, hideEngineHint: true } }))}>{t("dpi.engineHint.dismiss")}</Chip>
          </div>
        )}
        <div className={css.setting}>
          <span>{t("dpi.engine.label")}</span>
          <span className={css.row}>
            {available.map((e) => (
              <Chip key={e} active={engine === e} onClick={() => pickEngine(e)} title={t(`dpi.engine.${e}Desc`)}>
                {t(`dpi.engine.${e}`)}
              </Chip>
            ))}
          </span>
        </div>
        {info?.mechanism && <div className={css.dim}>{t("dpi.mechanism", { mechanism: info.mechanism })}</div>}
        <div className={css.setting}>
          <span>{t("dpi.preset")}</span>
          <span className={css.row}>
            <select aria-label="preset" value={strategy} onChange={(e) => setStrategy(e.target.value)}>
              {options.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.label}
                </option>
              ))}
            </select>
            {autotune?.running ? (
              <Chip onClick={() => void Service.CancelAutotune()}>{t("simple.autotuning", { preset: tuneName, index: autotune.index, total: autotune.total })}</Chip>
            ) : (
              <Chip onClick={() => void Service.StartAutotune()}>{t("dpi.autotune")}</Chip>
            )}
          </span>
        </div>
        {strategy === "custom" && (
          <div className={css.setting}>
            <span>{t("dpi.customArgs")}</span>
            <input
              aria-label={t("dpi.customArgs")}
              style={{ flex: 1 }}
              value={custom}
              onChange={(e) => setCustom(e.target.value)}
              onBlur={saveCustom}
            />
          </div>
        )}
        <div className={css.setting}>
          <span>{t("dpi.scope")}</span>
          <span className={css.row}>
            <Chip active={!showList} onClick={() => pickScope("all")}>
              {t("dpi.scopeAll")}
            </Chip>
            <Chip active={showList} onClick={() => pickScope("blacklist")}>
              {entries(saved) > 0 ? `${t("dpi.scopeBlacklist")} (${entries(saved)})` : t("dpi.scopeBlacklist")}
            </Chip>
          </span>
        </div>
        {showList && (
          <div className={css.blacklist}>
            <textarea
              aria-label={t("dpi.blacklistTitle")}
              placeholder={t("dpi.blacklistTitle")}
              title={t("dpi.blacklistTitle")}
              style={{ width: "100%", minHeight: 96 }}
              value={draft}
              onChange={(e) => {
                setDraft(e.target.value);
                setListNote(null);
              }}
              spellCheck={false}
            />
            <div className={css.row}>
              <Chip onClick={() => void saveList()} disabled={!newList && draft === saved}>
                {t("common.save")}
              </Chip>
              <Chip onClick={cancelList} disabled={!newList && draft === saved}>
                {t("common.cancel")}
              </Chip>
              {newList && <span className={css.dim}>{t("dpi.blacklistSuggested")}</span>}
              {listNote && <span className={css.ok}>✓ {listNote}</span>}
            </div>
          </div>
        )}
        {isZ && showList && !newList && (
          <div className={css.blacklist}>
            <div className={css.setting}>
              <span>{t("dpi.autoHostlist.label")}</span>
              <Toggle label={t("dpi.autoHostlist.label")} checked={z.autoHostlist} onChange={(v) => void setZ({ autoHostlist: v })} />
            </div>
            {autoOn &&
              (autoSites.length === 0 ? (
                <div className={css.dim}>{t("dpi.autoHostlist.empty")}</div>
              ) : (
                <>
                  {autoSites.map((d) => (
                    <div key={d} className={css.setting}>
                      <span>{d}</span>
                      <Chip label={t("dpi.autoHostlist.removeOne", { domain: d })} onClick={() => saveAutoSites(autoSites.filter((x) => x !== d))}>
                        ✕
                      </Chip>
                    </div>
                  ))}
                  <Chip onClick={() => saveAutoSites([])}>{t("dpi.autoHostlist.clear")}</Chip>
                </>
              ))}
          </div>
        )}
        {error && <div className={css.bad}>{error}</div>}
        {autotune && !autotune.running && autotune.error && <div className={css.bad}>{tCode(`errors.${autotune.error.code}.message`)}</div>}
        {autotune && !autotune.running && !autotune.error && autotune.preset && (
          <div className={css.ok}>{t("dpi.autotuneDone", { preset: tuneName, engine: ENGINE_NAME[autotune.engine] ?? autotune.engine })}</div>
        )}
        <div className={css.code} aria-label={t("dpi.preview")}>
          {info?.engines?.find((e) => e.id === engine)?.exe ?? ENGINE_EXE[engine]} {preview.join(" ")}
        </div>
        {running ? (
          <div className={css.ok}>● {t("log.DPI_STARTED", { engine: ENGINE_NAME[runningEngine], preset: runningName })}</div>
        ) : dpi.enabled ? (
          <div className={css.dim}>○ {t(snap.status === "protected" || snap.status === "degraded" ? "dpi.starting" : "dpi.waiting")}</div>
        ) : null}
      </div>

      <div className={css.grid2}>
        <div className={css.panel}>
          <div className={css.panelTitle}>
            <span>{t("dpi.fragment")}</span>
            <Toggle label={t("dpi.fragment")} checked={settings.fragmentDns.enabled} onChange={(v) => void save((s) => ({ ...s, fragmentDns: { ...s.fragmentDns, enabled: v } }))} />
          </div>
          <div className={css.setting}>
            <span>{t("dpi.chunks")}</span>
            <input type="number" min={2} max={20} style={{ width: 70 }} value={settings.fragmentDns.chunks}
              onChange={(e) => void save((s) => ({ ...s, fragmentDns: { ...s.fragmentDns, chunks: Number(e.target.value) } }))} />
          </div>
          <div className={css.setting}>
            <span>{t("dpi.delay")}</span>
            <input type="number" min={0} max={200} style={{ width: 70 }} value={settings.fragmentDns.delayMs}
              onChange={(e) => void save((s) => ({ ...s, fragmentDns: { ...s.fragmentDns, delayMs: Number(e.target.value) } }))} />
          </div>
          {dpi.enabled && <div className={css.dim}>{t("dpi.redundant")}</div>}
          <div className={css.dim}>{t("dpi.webFragmentNote")}</div>
        </div>

        <div className={css.panel}>
          <div className={css.panelTitle}>
            <span>{t("dpi.probeSites")}</span>
            <Chip onClick={() => void Service.ProbeNow().then((r) => setProbe(r ?? []))}>{t("dpi.probeAgain")}</Chip>
          </div>
          {(settings.probeSites ?? []).map((site) => {
            const r = probe.find((p) => p.site === site);
            return (
              <div key={site} className={css.setting}>
                <span>{site}</span>
                <span className={r ? (r.stage === "ok" ? css.ok : css.bad) : css.dim}>
                  {r ? t(`dpi.stage.${r.stage}`) : "·"}
                </span>
              </div>
            );
          })}
          <textarea
            aria-label={t("dpi.probeSites")}
            style={{ width: "100%", minHeight: 60, marginTop: 6 }}
            value={sites}
            onChange={(e) => setSites(e.target.value)}
            onBlur={() => void save((s) => ({ ...s, probeSites: sites.split(/\s+/).filter(Boolean) }))}
          />
        </div>
      </div>
    </div>
  );
}
