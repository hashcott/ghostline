import type { ReactNode } from "react";
import css from "./neon.module.css";

export type BannerAction = { label: string; onClick: () => void; primary?: boolean; disabled?: boolean };

type Props = { tone: "ok" | "warn" | "err" | "violet"; children: ReactNode; actions?: BannerAction[] };

export function Banner({ tone, children, actions = [] }: Props) {
  return (
    <div className={css.banner} data-tone={tone} role="alert">
      <div>
        {tone === "warn" ? "⚠ " : tone === "err" ? "✕ " : ""}
        {children}
      </div>
      {actions.length > 0 && (
        <div className={css.actions}>
          {actions.map((a) => (
            <button key={a.label} className={`${css.btn} ${a.primary ? css.primary : ""}`} onClick={a.onClick} disabled={a.disabled}>
              <span>{a.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
