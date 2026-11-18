"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { createContext, useCallback, useContext, useMemo, useRef, useState } from "react";
import { ApiError, crawler, type Status } from "@/lib/api";
import { SITE_NAME } from "@/lib/site";
import { usePoll } from "@/lib/usePoll";
import s from "./dashboard.module.css";

interface Toast {
  id: number;
  text: string;
  error?: boolean;
}

interface DashboardCtx {
  status: Status | null;
  statusError: Error | null;
  refreshStatus: () => Promise<void>;
  notify: (text: string, error?: boolean) => void;
  /** Runs an API action, toasting success text or the error message. */
  run: <T>(action: () => Promise<T>, success?: string | ((r: T) => string)) => Promise<T | undefined>;
}

const Ctx = createContext<DashboardCtx | null>(null);

export function useDashboard(): DashboardCtx {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useDashboard must be used inside <Shell>");
  return ctx;
}

const TABS = [
  { href: "/dashboard", label: "Overview" },
  { href: "/dashboard/add", label: "Add URLs" },
  { href: "/dashboard/pages", label: "Crawled pages" },
  { href: "/dashboard/queue", label: "Queue" },
  { href: "/dashboard/failures", label: "Failures" },
  { href: "/dashboard/settings", label: "Settings" },
];

const STATE_LABEL: Record<string, string> = {
  running: "Crawling",
  idle: "Waiting for URLs",
  paused: "Paused",
  stopping: "Shutting down",
  finished: "Finished",
  offline: "Not connected",
  connecting: "Connecting…",
};

export function Shell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { data: status, error: statusError, refresh } = usePoll(crawler.status, 2000);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);
  const [busy, setBusy] = useState(false);

  const notify = useCallback((text: string, error = false) => {
    const id = nextId.current++;
    setToasts((t) => [...t.slice(-3), { id, text, error }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), error ? 7000 : 3500);
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
    await run(paused ? crawler.resume : crawler.pause, paused ? "Crawl resumed." : "Crawl paused. In-flight requests will finish.");
    setBusy(false);
  }

  return (
    <Ctx.Provider value={ctx}>
      <div className={s.shell}>
        <header className={s.topbar}>
          <div className={s.topbarInner}>
            <Link href="/" className={s.brand} aria-label={`${SITE_NAME} home`}>
              <svg width="20" height="20" viewBox="0 0 22 22" aria-hidden="true">
                <circle cx="11" cy="11" r="3.5" fill="currentColor" />
                <circle cx="11" cy="11" r="9.5" fill="none" stroke="currentColor" strokeWidth="1.2" strokeDasharray="2 3" />
              </svg>
              <span className={s.brandText}>{SITE_NAME}</span>
            </Link>
            <span className={`${s.state} ${s[`state_${state}`] ?? ""}`} role="status" aria-live="polite">
              <span className={s.dot} aria-hidden="true" />
              {STATE_LABEL[state] ?? state}
            </span>
            <span className={s.spacer} />
            <button
              type="button"
              className={`btn ${paused ? "btn-primary" : ""}`}
              onClick={togglePause}
              disabled={!canToggle || busy}
            >
              {paused ? "Resume crawl" : "Pause crawl"}
            </button>
          </div>
          <nav className={s.tabs} aria-label="Dashboard">
            {TABS.map((t) => {
              const active = t.href === "/dashboard" ? pathname === t.href : pathname.startsWith(t.href);
              return (
                <Link
                  key={t.href}
                  href={t.href}
                  className={`${s.tab} ${active ? s.tabActive : ""}`}
                  aria-current={active ? "page" : undefined}
                >
                  {t.label}
                  {t.href === "/dashboard/failures" && !!status?.dead_letters && (
                    <span className={s.tabCount}>{status.dead_letters}</span>
                  )}
                </Link>
              );
            })}
          </nav>
        </header>

        <main className={s.main}>
          {statusError && (
            <div className={s.banner} role="alert">
              <div>
                <strong>{offline ? "Can't reach the crawler" : "Couldn't load crawl status"}</strong>
                <p>
                  {statusError.message}
                  {offline && status && " Showing the last known numbers."}
                </p>
              </div>
            </div>
          )}
          {children}
        </main>

        <div className={s.toasts} aria-live="polite">
          {toasts.map((t) => (
            <div key={t.id} className={`${s.toast} ${t.error ? s.toastError : ""}`} role={t.error ? "alert" : "status"}>
              {t.text}
            </div>
          ))}
        </div>
      </div>
    </Ctx.Provider>
  );
}
