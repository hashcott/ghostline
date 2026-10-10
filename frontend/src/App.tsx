import { useEffect, useState } from "react";
import { startBridge } from "./app/bridge";
import { useGhost, type Mode } from "./app/store";
import { Service } from "./app/api";
import { TitleBar } from "./components/neon/TitleBar";
import { SimpleView } from "./modes/simple/SimpleView";
import { FullView } from "./modes/full/FullView";
import { FakeSniBanner } from "./components/FakeSniBanner";
import { ServiceOutdated } from "./components/ServiceOutdated";
import { Terms } from "./components/Terms";
import i18n, { initI18n, type Lang } from "./i18n";
import css from "./App.module.css";

function App() {
  const settings = useGhost((s) => s.settings);
  const setSettings = useGhost((s) => s.setSettings);
  useEffect(() => startBridge(), []);
  // Asked once settings are there: on Linux the service may not be running
  // yet, and its install banner must stay usable until then.
  const [terms, setTerms] = useState<"unknown" | "pending" | "accepted">("unknown");
  const ready = settings != null;
  useEffect(() => {
    if (!ready) return;
    void Service.TermsAccepted().then(
      (ok) => setTerms(ok ? "accepted" : "pending"),
      () => setTerms("pending"),
    );
  }, [ready]);

  const mode: Mode = settings?.mode === "full" ? "full" : "simple";
  const lang: Lang = (i18n.language as Lang) === "en" ? "en" : "vi";

  const onMode = (m: Mode) => {
    if (settings) setSettings({ ...settings, mode: m });
    void Service.SetMode(m);
  };
  const onLang = async (l: Lang) => {
    await initI18n(l);
    if (settings) {
      const next = { ...settings, language: l };
      setSettings(next);
      void Service.SaveSettings(next);
    }
  };

  return (
    <div className={css.app}>
      <TitleBar mode={mode} onMode={onMode} lang={lang} onLang={onLang} />
      <FakeSniBanner />
      <ServiceOutdated />
      <main className={css.main} data-mode={mode}>
        {terms !== "accepted" ? (
          terms === "pending" && <Terms onAccepted={() => setTerms("accepted")} />
        ) : mode === "simple" ? (
          <SimpleView
            onOpenLogs={() => {
              useGhost.getState().setPage("logs");
              onMode("full");
            }}
            onOpenServers={() => {
              useGhost.getState().setPage("servers");
              onMode("full");
            }}
            onOpenFull={() => onMode("full")}
          />
        ) : (
          <FullView />
        )}
      </main>
    </div>
  );
}

export default App;
