import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type LANInfo, type UpstreamProxy } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { saveSettings } from "../../../app/settings";
import { describeError, tCode } from "../../../i18n";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import { QRCode } from "../../../components/neon/QRCode";
import css from "../full.module.css";

const MODES = ["auto", "always", "never"] as const;
const METHODS = ["tcp", "record", "both"] as const;
const OUTCOMES = ["direct", "fragmented", "upstream", "blocked", "blockedEvenFragmented"] as const;

const emptyUpstream: UpstreamProxy = { id: "", type: "socks5", addr: "", user: "", passEnc: "" };

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

export function Proxy() {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const snap = useGhost((s) => s.snapshot);
  const stats = useGhost((s) => s.proxyStats);
  const [error, setError] = useState<string | null>(null);
  const [port, setPort] = useState(String(settings?.proxy?.port ?? 8080));
  const [lan, setLan] = useState<LANInfo | null>(null);
  const [qr, setQr] = useState<boolean[][] | null>(null);
  const [cache, setCache] = useState<string[]>([]);
  const [draft, setDraft] = useState<UpstreamProxy | null>(null);
  const [password, setPassword] = useState("");
  const [testResult, setTestResult] = useState<Record<string, string>>({});
  const [sysInfo, setSysInfo] = useState<{ desktop: string; supported: boolean } | null>(null);

  const proxy = settings?.proxy;
  const shareLan = !!proxy?.shareLan;

  useEffect(() => {
    void Service.GetFragCache().then((c) => setCache(c ?? []));
    Service.SysProxyInfo()
      .then((i) => setSysInfo(i ?? null))
      .catch(() => setSysInfo(null));
    // Live proxy:stats events win over this initial fetch.
    void Service.GetProxyStats().then((s) => s && !useGhost.getState().proxyStats && useGhost.getState().setProxyStats(s));
  }, []);

  useEffect(() => {
    if (!shareLan) {
      setLan(null);
      setQr(null);
      return;
    }
    void Service.GetLANInfo().then((info) => {
      setLan(info);
      const first = info?.addrs?.[0];
      if (first) void Service.GetQR(first).then((m) => setQr((m ?? []) as boolean[][]));
    });
  }, [shareLan, proxy?.port]);

  if (!settings || !proxy) return null;

  const save = async (patch: Parameters<typeof saveSettings>[0]) => setError(await saveSettings(patch));
  const setProxy = (p: Partial<typeof proxy>) => void save((s) => ({ ...s, proxy: { ...s.proxy, ...p } }));
  const setFrag = (f: Partial<typeof proxy.fragment>) =>
    void save((s) => ({ ...s, proxy: { ...s.proxy, fragment: { ...s.proxy.fragment, ...f } } }));

  const commitPort = () => {
    const n = Number(port);
    if (!Number.isInteger(n) || n < 1024 || n > 65535) {
      setError(t("proxy.portInvalid"));
      return;
    }
    if (n !== proxy.port) setProxy({ port: n });
  };

  const testUpstream = (id: string) => {
    setTestResult((r) => ({ ...r, [id]: "…" }));
    Service.TestUpstreamProxy(id)
      .then(() => setTestResult((r) => ({ ...r, [id]: t("proxy.upstreams.testOk") })))
      .catch((e) => setTestResult((r) => ({ ...r, [id]: describeError(e) })));
  };

  const saveDraft = () => {
    if (!draft) return;
    Service.SaveUpstreamProxy(draft, password)
      .then(() => Service.GetSettings())
      .then((fresh) => {
        // Go is the source of truth for upstreams (passwords stay encrypted there).
        if (fresh) useGhost.getState().setSettings(fresh);
        setDraft(null);
        setPassword("");
        setError(null);
      })
      .catch((e) => setError(describeError(e)));
  };

  const deleteUpstream = (id: string) =>
    Service.DeleteUpstreamProxy(id)
      .then(() => useGhost.getState().setSettings({ ...settings, proxy: { ...proxy, upstreams: (proxy.upstreams ?? []).filter((u) => u.id !== id) } }))
      .catch((e) => setError(describeError(e)));

  const clearCache = (host: string) =>
    void Service.ClearFragCache(host).then(() => setCache((c) => (host ? c.filter((h) => h !== host) : [])));

  const blockedEven = stats?.byOutcome?.blockedEvenFragmented ?? 0;
  const pxErr = snap.proxy?.error;

  return (
    <div className={css.page}>
      <div className={css.head}>
        <span>{t("proxy.title")}</span>
        <span className={css.count}>{snap.proxy?.running ? snap.proxy.addr : t("common.off")}</span>
      </div>

      {pxErr && (
        <div className={css.panel}>
          <div className={css.bad}>✕ {tCode(`errors.${pxErr.code}.message`, pxErr.params ?? undefined)}</div>
          <div className={css.row}>
            <Chip onClick={() => void Service.RetryProxy()}>{t("common.retry")}</Chip>
          </div>
        </div>
      )}

      <div className={css.panel}>
        <div className={css.setting}>
          <span>{t("proxy.enabled")}</span>
          <Toggle label={t("proxy.enabled")} checked={proxy.enabled} onChange={(v) => setProxy({ enabled: v })} />
        </div>
        <div className={css.setting}>
          <span>{t("proxy.systemProxy")}</span>
          <Toggle label={t("proxy.systemProxy")} checked={proxy.systemProxy} onChange={(v) => setProxy({ systemProxy: v })} />
        </div>
        {sysInfo?.desktop &&
          (sysInfo.supported ? (
            <div className={css.dim}>{t("proxy.systemProxyDesktop", { desktop: sysInfo.desktop })}</div>
          ) : (
            <div className={css.warn}>{t("proxy.systemProxyManual", { desktop: sysInfo.desktop, addr: `127.0.0.1:${proxy.port}` })}</div>
          ))}
        <div className={css.setting}>
          <span>{t("proxy.shareLan")}</span>
          <Toggle label={t("proxy.shareLan")} checked={proxy.shareLan} onChange={(v) => setProxy({ shareLan: v })} />
        </div>
        <div className={css.setting}>
          <span>{t("proxy.port")}</span>
          <input type="number" aria-label={t("proxy.port")} min={1024} max={65535} style={{ width: 90 }} value={port}
            onChange={(e) => setPort(e.target.value)} onBlur={commitPort} />
        </div>
        {error && <div className={css.bad}>{error}</div>}
        <div className={css.dim}>{t("proxy.note")}</div>
      </div>

      {shareLan && (
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("proxy.lan.title")}</div>
          {lan?.public && <div className={css.warn}>⚠ {t("proxy.lan.public")}</div>}
          <div className={css.row} style={{ alignItems: "flex-start", gap: 16 }}>
            {qr && lan?.addrs?.[0] && <QRCode matrix={qr} label={t("proxy.lan.qr", { addr: lan.addrs[0] })} />}
            <div>
              {(lan?.addrs ?? []).length === 0 && <div className={css.dim}>{t("proxy.lan.none")}</div>}
              {(lan?.addrs ?? []).map((a) => (
                <div key={a} className={css.big}>{a}</div>
              ))}
              <div className={css.dim}>{t("proxy.lan.howto")}</div>
            </div>
          </div>
        </div>
      )}

      <div className={css.grid2}>
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("proxy.fragment.title")}</div>
          <div className={css.setting}>
            <span>{t("proxy.fragment.mode")}</span>
            <span className={css.row}>
              {MODES.map((m) => (
                <Chip key={m} active={proxy.fragment.mode === m} onClick={() => setFrag({ mode: m })}>{t(`proxy.fragment.modes.${m}`)}</Chip>
              ))}
            </span>
          </div>
          <div className={css.setting}>
            <span>{t("proxy.fragment.method")}</span>
            <span className={css.row}>
              {METHODS.map((m) => (
                <Chip key={m} active={proxy.fragment.method === m} onClick={() => setFrag({ method: m })}>{t(`proxy.fragment.methods.${m}`)}</Chip>
              ))}
            </span>
          </div>
          <div className={css.setting}>
            <span>{t("dpi.chunks")}</span>
            <input type="number" aria-label={t("dpi.chunks")} min={2} max={64} style={{ width: 70 }} value={proxy.fragment.chunks}
              onChange={(e) => setFrag({ chunks: Number(e.target.value) })} />
          </div>
          <div className={css.setting}>
            <span>{t("dpi.delay")}</span>
            <input type="number" aria-label={t("dpi.delay")} min={0} max={100} style={{ width: 70 }} value={proxy.fragment.delayMs}
              onChange={(e) => setFrag({ delayMs: Number(e.target.value) })} />
          </div>
          <div className={css.setting}>
            <span>{t("proxy.fragment.autoTimeout")}</span>
            <input type="number" aria-label={t("proxy.fragment.autoTimeout")} min={1000} max={10000} step={500} style={{ width: 80 }}
              value={proxy.fragment.autoTimeoutMs} onChange={(e) => setFrag({ autoTimeoutMs: Number(e.target.value) })} />
          </div>
          {blockedEven > 0 && <div className={css.warn}>⚠ {t("proxy.hint.goodbyedpi", { count: blockedEven })}</div>}
        </div>

        <div className={css.panel}>
          <div className={css.panelTitle}>
            <span>{t("proxy.fragment.cache")}</span>
            {cache.length > 0 && <Chip onClick={() => clearCache("")}>{t("proxy.fragment.clearAll")}</Chip>}
          </div>
          {cache.length === 0 && <div className={css.dim}>{t("proxy.fragment.cacheEmpty")}</div>}
          {cache.map((h) => (
            <div key={h} className={css.setting}>
              <span>{h}</span>
              <Chip label={t("proxy.fragment.forget", { host: h })} onClick={() => clearCache(h)}>✕</Chip>
            </div>
          ))}
        </div>
      </div>

      <div className={css.panel}>
        <div className={css.panelTitle}>
          <span>{t("proxy.upstreams.title")}</span>
          <Chip onClick={() => setDraft({ ...emptyUpstream })}>{t("proxy.upstreams.add")}</Chip>
        </div>
        {(proxy.upstreams ?? []).length === 0 && <div className={css.dim}>{t("proxy.upstreams.empty")}</div>}
        {(proxy.upstreams ?? []).map((u) => (
          <div key={u.id} className={css.setting}>
            <span>
              <b>{u.id}</b> · {u.type} · {u.addr}
              {testResult[u.id] && <span className={css.dim}> — {testResult[u.id]}</span>}
            </span>
            <span className={css.row}>
              <Chip label={t("proxy.upstreams.testFor", { id: u.id })} onClick={() => testUpstream(u.id)}>{t("proxy.upstreams.test")}</Chip>
              <Chip onClick={() => setDraft({ ...u })}>{t("proxy.upstreams.edit")}</Chip>
              <Chip label={t("proxy.upstreams.deleteFor", { id: u.id })} onClick={() => void deleteUpstream(u.id)}>✕</Chip>
            </span>
          </div>
        ))}
        {draft && (
          <div className={css.row} style={{ flexWrap: "wrap", marginTop: 8 }}>
            <input aria-label={t("proxy.upstreams.id")} placeholder="tor" value={draft.id} onChange={(e) => setDraft({ ...draft, id: e.target.value })} style={{ width: 90 }} />
            <select aria-label={t("proxy.upstreams.type")} value={draft.type} onChange={(e) => setDraft({ ...draft, type: e.target.value })}>
              <option value="socks5">socks5</option>
              <option value="http">http</option>
            </select>
            <input aria-label={t("proxy.upstreams.addr")} placeholder="127.0.0.1:9050" value={draft.addr} onChange={(e) => setDraft({ ...draft, addr: e.target.value })} />
            <input aria-label={t("proxy.upstreams.user")} placeholder={t("proxy.upstreams.user")} value={draft.user} onChange={(e) => setDraft({ ...draft, user: e.target.value })} style={{ width: 100 }} />
            <input aria-label={t("proxy.upstreams.password")} type="password" placeholder={draft.passEnc ? "••••" : t("proxy.upstreams.password")}
              value={password} onChange={(e) => setPassword(e.target.value)} style={{ width: 110 }} />
            <Chip onClick={saveDraft}>{t("common.save")}</Chip>
            <Chip onClick={() => setDraft(null)}>{t("common.cancel")}</Chip>
          </div>
        )}
      </div>

      {stats && (
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("proxy.stats.title")}</div>
          <div className={css.row} style={{ flexWrap: "wrap", gap: 16 }}>
            <span>{t("proxy.stats.open", { count: stats.open })}</span>
            <span>{t("proxy.stats.lan", { count: stats.lanClients })}</span>
            <span>↑ {formatBytes(stats.bytesIn)}</span>
            <span>↓ {formatBytes(stats.bytesOut)}</span>
            {OUTCOMES.map((o) => (
              <span key={o} className={css.dim}>
                {t(`proxy.stats.${o}`)}: {stats.byOutcome?.[o] ?? 0}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
