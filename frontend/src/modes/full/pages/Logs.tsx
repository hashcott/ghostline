import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type LogEvent } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { tCode } from "../../../i18n";
import { Chip } from "../../../components/neon/Chip";
import { Toggle } from "../../../components/neon/Toggle";
import { openIssueForm } from "../../../app/report";
import css from "../full.module.css";

const SOURCES = ["all", "engine", "dpi", "proxy", "rules", "system"] as const;

function message(e: LogEvent): string {
  const key = `log.${e.code}`;
  const text = tCode(key, e.params ?? undefined);
  return text === e.code ? tCode(`errors.${e.code}.message`, e.params ?? undefined) : text;
}

const hhmmss = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleTimeString("en-GB", { hour12: false });
};

export function Logs() {
  const { t } = useTranslation();
  const logs = useGhost((s) => s.logs);
  const queries = useGhost((s) => s.queries500);
  const [source, setSource] = useState<(typeof SOURCES)[number]>("all");
  const [paused, setPaused] = useState<LogEvent[] | null>(null);
  const showQueries = useGhost((s) => s.queryLog);
  const [reported, setReported] = useState(false);

  const shown = useMemo(() => {
    const base = paused ?? logs;
    return source === "all" ? base : base.filter((l) => (source === "system" ? l.source === "system" || l.source === "ok" : l.source === source));
  }, [logs, paused, source]);

  const asText = () => shown.map((l) => `${hhmmss(l.time)} [${l.source}] ${message(l)}`).join("\n");

  const save = () => {
    const url = URL.createObjectURL(new Blob([asText()], { type: "text/plain" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "ghostline-log.txt";
    a.click();
    URL.revokeObjectURL(url);
  };

  // The bug form has a log field: the log goes to the clipboard first.
  const report = async () => {
    try {
      await navigator.clipboard?.writeText(asText());
    } catch {
      // No clipboard: the form still opens; copy stays one press away.
    }
    setReported(true);
    await openIssueForm();
  };

  const toggleQueries = (on: boolean) => {
    useGhost.getState().setQueryLog(on);
    void Service.SetQueryLog(on);
    if (!on) useGhost.getState().clearQueries();
  };

  return (
    <div className={css.page}>
      <div className={css.head}>
        <span>{t("logs.title")}</span>
        <span className={css.tools}>
          <Chip onClick={() => setPaused(paused ? null : logs)}>{paused ? t("logs.resume") : t("logs.pause")}</Chip>
          <Chip onClick={() => void navigator.clipboard?.writeText(asText())}>{t("logs.copy")}</Chip>
          <Chip onClick={save}>{t("logs.save")}</Chip>
          <Chip onClick={() => void report()}>{t("logs.report")}</Chip>
        </span>
      </div>
      <div className={css.chips}>
        {SOURCES.map((s) => (
          <Chip key={s} active={source === s} onClick={() => setSource(s)}>
            {t(`logs.${s}`)}
          </Chip>
        ))}
        ·
        <Toggle showLabel label={t("logs.queries")} checked={showQueries} onChange={toggleQueries} />
      </div>
      <div className={css.dim}>ⓘ {t("logs.queriesNote")}</div>
      {reported && <div className={css.ok}>{t("logs.reportNote")}</div>}
      <div className={`${css.panel} ${css.log}`}>
        {shown.map((l, i) => (
          <div key={i}>
            <span className={css.dim}>{hhmmss(l.time)}</span>{" "}
            <span style={{ color: l.source === "dpi" ? "var(--amber)" : l.source === "ok" ? "var(--green)" : "var(--cyan)" }}>[{l.source}]</span>{" "}
            <span>{message(l)}</span>
          </div>
        ))}
        {showQueries &&
          queries.map((q, i) => (
            <div key={"q" + i} className={css.dim}>
              {q.Domain} {q.Type} → {q.Upstream || "cache"} {q.Err ? `✕ ${q.Err}` : ""}
            </div>
          ))}
      </div>
    </div>
  );
}
