import { useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Application, Browser } from "@wailsio/runtime";
import { Service } from "../app/api";
import { describeError } from "../i18n";
import { Chip } from "./neon/Chip";
import css from "./Terms.module.css";

const DISCLAIMER_URL = "https://hashcott.github.io/ghostline/disclaimer.html";
const PARAGRAPHS = ["purpose", "responsible", "forbidden", "lists", "warranty", "license"] as const;

/**
 * Terms stands in for the whole window until the current terms of use are
 * accepted: read to the end, ticked, then accepted (recorded by Go with the
 * time) or declined (Ghostline quits).
 */
export function Terms({ onAccepted }: { onAccepted: () => void }) {
  const { t } = useTranslation();
  const box = useRef<HTMLDivElement>(null);
  const [read, setRead] = useState(false);
  const [ticked, setTicked] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const check = () => {
    const el = box.current;
    if (el && el.scrollTop + el.clientHeight >= el.scrollHeight - 2) setRead(true);
  };
  // Text shorter than the box: nothing to scroll.
  useLayoutEffect(() => {
    const el = box.current;
    if (el && el.scrollHeight > 0 && el.scrollHeight <= el.clientHeight) setRead(true);
  }, []);

  const accept = async () => {
    setBusy(true);
    setError("");
    try {
      await Service.AcceptTerms();
      onAccepted();
    } catch (e) {
      setError(describeError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className={css.page}>
      <div className={css.panel}>
        <div className={css.title}>{t("terms.title")}</div>
        <p className={css.intro}>{t("terms.intro")}</p>
        <div ref={box} className={css.text} data-testid="terms-text" onScroll={check}>
          {PARAGRAPHS.map((p) => (
            <p key={p} className={p === "forbidden" ? css.warn : undefined}>{t(`terms.${p}`)}</p>
          ))}
          <p>
            <button className={css.link} onClick={() => void Browser.OpenURL(DISCLAIMER_URL)}>{t("terms.full")}</button>
          </p>
        </div>
        <label className={css.tick}>
          <input type="checkbox" checked={ticked} onChange={(e) => setTicked(e.target.checked)} />
          <span>{t("terms.agree")}</span>
        </label>
        {!read && <div className={css.hint}>{t("terms.scroll")}</div>}
        {error && <div className={css.error}>{error}</div>}
        <div className={css.actions}>
          <Chip active disabled={!read || !ticked || busy} onClick={() => void accept()}>{t("terms.accept")}</Chip>
          <Chip onClick={() => void Application.Quit()}>{t("terms.decline")}</Chip>
        </div>
      </div>
    </div>
  );
}
