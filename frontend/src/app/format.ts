import { useEffect, useState } from "react";
import type { PowerState } from "../components/neon/PowerButton";
import { useGhost } from "./store";

export function powerState(status: string): PowerState {
  switch (status) {
    case "protected":
      return "on";
    case "degraded":
      return "warn";
    case "connecting":
    case "disconnecting":
      return "busy";
    case "error":
      return "err";
  }
  return "off";
}

export function isConnected(status: string) {
  return status === "protected" || status === "degraded";
}

/** Uptime as HH:MM:SS since an ISO timestamp, ticking every second. */
export function useUptime(since: string | undefined): string {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const start = since ? Date.parse(since) : NaN;
  if (!since || Number.isNaN(start) || start <= 0) return "--:--:--";
  const s = Math.max(0, Math.floor((now - start) / 1000));
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(Math.floor(s / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`;
}

export function serverSummary(servers: string[] | null | undefined): string {
  if (!servers || servers.length === 0) return "—";
  return servers.length === 1 ? servers[0] : `${servers[0]} +${servers.length - 1}`;
}

type ServerLike = { name: string; address: string; source?: string; ips?: string[] | null };

/** DNSCrypt list ids like "a-and-a" become "A And A"; curated names stay. */
export function prettyServerName(s: ServerLike): string {
  if (s.source === "dnscrypt" && /^[a-z0-9-]+$/.test(s.name)) {
    return s.name
      .split("-")
      .filter(Boolean)
      .map((w) => w[0].toUpperCase() + w.slice(1))
      .join(" ");
  }
  return s.name;
}

/** The host (for URLs) or IP (for stamps) that tells same-named servers apart. */
export function serverDetail(s: ServerLike): string {
  const m = /^(?:https|tls|quic):\/\/([^/:]+)/.exec(s.address);
  if (m) return m[1];
  return s.ips?.[0] ?? "";
}

/**
 * useUpdate returns the newer release, either announced in this session or
 * remembered by Go from an earlier check (AppInfo), or null.
 */
export function useUpdate(): { tag: string; url: string } | null {
  const update = useGhost((s) => s.update);
  const info = useGhost((s) => s.info);
  // An event (even an empty one: the notice was withdrawn) wins over the
  // AppInfo read at start.
  const tag = update ? update.tag : info?.updateTag;
  const url = update ? update.url : info?.updateUrl;
  return tag && url ? { tag, url } : null;
}
