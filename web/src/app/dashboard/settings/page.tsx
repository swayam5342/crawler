"use client";

import { useState } from "react";
import { useDashboard } from "@/components/dashboard/Shell";
import { PageHead, Panel, styles as s } from "@/components/dashboard/ui";
import { crawler, type ResetOptions } from "@/lib/api";
import { usePoll } from "@/lib/usePoll";

const RESET_ITEMS: { key: keyof ResetOptions; label: string; hint: string }[] = [
  { key: "queue", label: "Queue and history", hint: "Empties the queue and forgets which URLs were seen, so they can be crawled again." },
  { key: "dead_letters", label: "Failure log", hint: "Deletes every failed URL without retrying it." },
  { key: "pages", label: "Stored pages", hint: "Deletes all page records from the database. Raw page bodies on disk or S3 are kept." },
  { key: "stats", label: "Counters", hint: "Zeroes the numbers, chart and activity feed on the Overview." },
];

function toYAML(value: unknown, indent = 0): string {
  const pad = "  ".repeat(indent);
  if (Array.isArray(value)) {
    if (!value.length) return "[]";
    return "\n" + value.map((v) => `${pad}- ${toYAML(v, indent + 1).trimStart()}`).join("\n");
  }
  if (value && typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>);
    if (!entries.length) return "{}";
    return (
      "\n" +
      entries
        .map(([k, v]) => {
          const rendered = toYAML(v, indent + 1);
          return `${pad}${k}:${rendered.startsWith("\n") ? rendered : " " + rendered}`;
        })
        .join("\n")
    );
  }
  if (typeof value === "string") return value === "" ? '""' : JSON.stringify(value);
  return String(value);
}

export default function Settings() {
  const { status, run } = useDashboard();
  const { data: config } = usePoll(crawler.config, 0);
  const [domain, setDomain] = useState("");
  const [reset, setReset] = useState<ResetOptions>({ queue: false, dead_letters: false, pages: false, stats: false });
  const [confirmText, setConfirmText] = useState("");
  const [busy, setBusy] = useState(false);

  const domains = status?.allowed_domains ?? [];
  const selected = RESET_ITEMS.filter((i) => reset[i.key]);

  async function addDomain(e: React.FormEvent) {
    e.preventDefault();
    const d = domain.trim().replace(/^https?:\/\//, "").replace(/\/.*$/, "");
    if (!d) return;
    const r = await run(() => crawler.allowDomain(d), (r) => (r.added ? `${d} is now allowed.` : `${d} was already allowed.`));
    if (r) setDomain("");
  }

  async function removeDomain(d: string) {
    if (domains.length === 1 && !confirm(`Remove ${d}? With no domains on the list, every domain is allowed.`)) return;
    await run(() => crawler.disallowDomain(d), `${d} removed from the allow-list.`);
  }

  async function doReset(e: React.FormEvent) {
    e.preventDefault();
    if (!selected.length || confirmText !== "reset") return;
    setBusy(true);
    const r = await run(() => crawler.reset(reset), "Reset complete.");
    setBusy(false);
    if (r) {
      setReset({ queue: false, dead_letters: false, pages: false, stats: false });
      setConfirmText("");
    }
  }

  return (
    <>
      <PageHead title="Settings" description="Changes here apply immediately to the running crawl. They aren't written back to config.yaml." />

      <div className={`${s.grid} ${s.gridTwo}`}>
        <div className={s.grid}>
          <Panel title="Allowed domains">
            <p style={{ color: "var(--ink-2)", marginBottom: 14 }}>
              {domains.length
                ? "Only these domains and their subdomains are crawled. Links anywhere else are ignored."
                : "The list is empty, so links to any domain are followed. Add a domain to keep the crawl on specific sites."}
            </p>
            {domains.length > 0 && (
              <div className={s.chips} style={{ marginBottom: 16 }}>
                {domains.map((d) => (
                  <span key={d} className={s.chip}>
                    {d}
                    <button type="button" onClick={() => removeDomain(d)} aria-label={`Remove ${d}`}>
                      ×
                    </button>
                  </span>
                ))}
              </div>
            )}
            <form className={s.quickAdd} style={{ marginBottom: 0 }} onSubmit={addDomain}>
              <label htmlFor="domain" className="visually-hidden">
                Domain to allow
              </label>
              <input
                id="domain"
                className="input"
                placeholder="example.com"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                autoComplete="off"
              />
              <button className="btn" type="submit" disabled={!domain.trim()}>
                Allow domain
              </button>
            </form>
          </Panel>

          <Panel title="Reset" className={s.danger}>
            <form className={s.form} onSubmit={doReset}>
              {RESET_ITEMS.map((item) => (
                <label key={item.key} className="check">
                  <input
                    type="checkbox"
                    checked={reset[item.key]}
                    onChange={(e) => setReset({ ...reset, [item.key]: e.target.checked })}
                  />
                  <span>
                    {item.label}
                    <small>{item.hint}</small>
                  </span>
                </label>
              ))}
              {selected.length > 0 && (
                <div>
                  <label className={s.label} htmlFor="confirm-reset">
                    Type <code>reset</code> to clear {selected.map((i) => i.label.toLowerCase()).join(", ")}
                  </label>
                  <input
                    id="confirm-reset"
                    className="input"
                    value={confirmText}
                    onChange={(e) => setConfirmText(e.target.value)}
                    autoComplete="off"
                  />
                  <p className={s.hint}>This can&apos;t be undone.</p>
                </div>
              )}
              <div>
                <button className="btn btn-danger" type="submit" disabled={!selected.length || confirmText !== "reset" || busy}>
                  {busy ? "Resetting…" : "Reset selected"}
                </button>
              </div>
            </form>
          </Panel>
        </div>

        <Panel title="Running configuration" meta="read-only, secrets hidden" flush>
          <pre className={`${s.configPre} mono`}>{config ? toYAML(config).trimStart() : "Loading…"}</pre>
        </Panel>
      </div>
    </>
  );
}
