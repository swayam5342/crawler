"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { createContext, useCallback, useContext, useMemo, useRef, useState } from "react";
import { ApiError, crawler, type Status } from "@/lib/api";
import { usePoll } from "@/lib/usePoll";
import s from "./dashboard.module.css";

interface DashboardCtx {
  status: Status | null;
  statusError: Error | null;
  refreshStatus: () => Promise<void>;
  notify: (text: string, error?: boolean) => void;
  /** Runs an API action, then shows the success text or the error message. */
  run: <T>(action: () => Promise<T>, success?: string | ((r: T) => string)) => Promise<T | undefined>;
}

const Ctx = createContext<DashboardCtx | null>(null);

export function useDashboard(): DashboardCtx {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useDashboard must be used inside <Shell>");
  return ctx;
}

const NAV = [
  { href: "/dashboard", label: "Overview" },
  { href: "/dashboard/add", label: "Add URLs" },
  { href: "/dashboard/pages", label: "Crawled pages" },
  { href: "/dashboard/queue", label: "Queue" },
  { href: "/dashboard/failures", label: "Failed URLs" },
  { href: "/dashboard/settings", label: "Settings" },
];

const STATE_LABEL: Record<string, string> = {
  running: "Crawling",
  idle: "Idle, waiting for URLs",
  paused: "Paused",
  stopping: "Shutting down",
  finished: "Finished",
  offline: "Not connected",
  connecting: "Connecting…",
};

export function Shell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { data: status, error: statusError, refresh } = usePoll(crawler.status, 2000);
  const [message, setMessage] = useState<{ text: string; error: boolean } | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [busy, setBusy] = useState(false);

  const notify = useCallback((text: string, error = false) => {
    setMessage({ text, error });
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setMessage(null), error ? 8000 : 4000);
  }, []);

  const run = useCallback(
    async <T,>(action: () => Promise<T>, success?: string | ((r: T) => string)) => {
      try {
        const r = await action();
        if (success) notify(typeof success === "function" ? success(r) : success);
        refresh();
        return r;
      } catch (e) {
        notify(e instanceof Error ? e.message : String(e), true);
        return undefined;
      }
    },
    [notify, refresh],
  );

  const ctx = useMemo<DashboardCtx>(
    () => ({ status, statusError, refreshStatus: refresh, notify, run }),
    [status, statusError, refresh, notify, run],
  );

  const offline = statusError instanceof ApiError && (statusError.unreachable || statusError.status === 0);
  const state = offline || (!status && statusError) ? "offline" : (status?.state ?? "connecting");
  const paused = status?.state === "paused";
  const canToggle = !!status && !offline && status.state !== "stopping" && status.state !== "finished";

  async function togglePause() {
    setBusy(true);
    await run(paused ? crawler.resume : crawler.pause, paused ? "Crawl resumed." : "Crawl paused. Requests already in progress will finish.");
    setBusy(false);
  }

  return (
    <Ctx.Provider value={ctx}>
      <div className={s.shell}>
        <header className={s.header}>
          <div className={s.headerInner}>
            <span className={s.appName}>Crawler dashboard</span>
            <span className={s.statusText} role="status" aria-live="polite">
              Status: <strong>{STATE_LABEL[state] ?? state}</strong>
            </span>
            <span className={s.spacer} />
            <button type="button" className={`btn ${paused ? "btn-primary" : ""}`} onClick={togglePause} disabled={!canToggle || busy}>
              {paused ? "Resume crawl" : "Pause crawl"}
            </button>
          </div>
          <nav className={s.nav} aria-label="Dashboard">
            {NAV.map((t) => {
              const active = t.href === "/dashboard" ? pathname === t.href : pathname.startsWith(t.href);
              return (
                <Link
                  key={t.href}
                  href={t.href}
                  className={`${s.navLink} ${active ? s.navActive : ""}`}
                  aria-current={active ? "page" : undefined}
                >
                  {t.label}
                  {t.href === "/dashboard/failures" && !!status?.dead_letters && (
                    <span className={s.navCount}> ({status.dead_letters})</span>
                  )}
                </Link>
              );
            })}
          </nav>
        </header>

        <main className={s.main}>
          {statusError && (
            <div className={s.alert} role="alert">
              <strong>{offline ? "Can't reach the crawler." : "Couldn't load crawl status."}</strong> {statusError.message}
              {offline && status && " Showing the last known numbers."}
            </div>
          )}
          {children}
        </main>

        {message && (
          <div className={`${s.message} ${message.error ? s.messageError : ""}`} role={message.error ? "alert" : "status"}>
            {message.text}
          </div>
        )}
      </div>
    </Ctx.Provider>
  );
}
