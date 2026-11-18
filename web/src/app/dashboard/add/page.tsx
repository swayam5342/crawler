"use client";

import { useState } from "react";
import { useDashboard } from "@/components/dashboard/Shell";
import { Badge, PageHead, Panel, URLText, styles as s } from "@/components/dashboard/ui";
import { crawler, type AddResult } from "@/lib/api";

const MAX = 1000;

const RESULT: Record<AddResult["status"], { tone: "ok" | "warn" | "bad" | "plain"; label: string; why: string }> = {
  queued: { tone: "ok", label: "Queued", why: "" },
  duplicate: { tone: "plain", label: "Already seen", why: "Crawled or queued before. Tick Recrawl to fetch it again." },
  out_of_scope: { tone: "warn", label: "Out of scope", why: "Its domain isn't on the allow-list." },
  invalid: { tone: "bad", label: "Invalid", why: "" },
  closed: { tone: "bad", label: "Not accepted", why: "The crawler is shutting down." },
};

function parse(text: string): string[] {
  const seen = new Set<string>();
  for (const part of text.split(/[\s,]+/)) {
    const v = part.trim();
    if (v) seen.add(v);
  }
  return [...seen];
}

export default function AddURLs() {
  const { run, status } = useDashboard();
  const [text, setText] = useState("");
  const [allowDomain, setAllowDomain] = useState(true);
  const [recrawl, setRecrawl] = useState(false);
  const [results, setResults] = useState<AddResult[] | null>(null);
  const [busy, setBusy] = useState(false);

  const urls = parse(text);
  const tooMany = urls.length > MAX;
  const scoped = (status?.allowed_domains.length ?? 0) > 0;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!urls.length || tooMany) return;
    setBusy(true);
    const res = await run(
      () => crawler.addURLs(urls, { allow_domain: allowDomain, recrawl }),
      (r) => {
        const n = r.results.filter((x) => x.status === "queued").length;
        return `${n} of ${r.results.length} URL${r.results.length === 1 ? "" : "s"} queued.`;
      },
    );
    setBusy(false);
    if (res) {
      setResults(res.results);
      if (res.results.every((r) => r.status === "queued")) setText("");
    }
  }

  return (
    <>
      <PageHead
        title="Add URLs"
        description="New URLs start at depth 0, so the crawler follows their links up to the configured max depth."
      />

      <div className={`${s.grid} ${s.gridTwo}`}>
        <Panel>
          <form className={s.form} onSubmit={submit}>
            <div>
              <label className={s.label} htmlFor="urls">
                URLs to crawl
              </label>
              <textarea
                id="urls"
                className="textarea"
                placeholder={"https://example.com/\nhttps://example.com/blog\nexample.org"}
                value={text}
                onChange={(e) => setText(e.target.value)}
                spellCheck={false}
              />
              <p className={s.hint} style={tooMany ? { color: "var(--brick)" } : undefined}>
                {tooMany
                  ? `${urls.length} URLs. Submit at most ${MAX} at a time.`
                  : `One per line, or separated by spaces or commas. URLs without a scheme get https://. ${urls.length ? `${urls.length} ready.` : ""}`}
              </p>
            </div>

            <label className="check">
              <input type="checkbox" checked={allowDomain} onChange={(e) => setAllowDomain(e.target.checked)} />
              <span>
                Allow these domains
                <small>
                  {scoped
                    ? "Adds each URL's domain to the allow-list so its pages are in scope."
                    : "No allow-list is set, so every domain is already in scope."}
                </small>
              </span>
            </label>

            <label className="check">
              <input type="checkbox" checked={recrawl} onChange={(e) => setRecrawl(e.target.checked)} />
              <span>
                Recrawl
                <small>Fetch URLs again even if they were crawled before.</small>
              </span>
            </label>

            <div>
              <button className="btn btn-primary" type="submit" disabled={!urls.length || tooMany || busy}>
                {busy ? "Adding…" : urls.length > 1 ? `Add ${urls.length} URLs` : "Add URL"}
              </button>
            </div>
          </form>
        </Panel>

        <Panel title="Result" meta={results ? `${results.length} submitted` : undefined} flush>
          {results ? (
            <ul className={s.resultList}>
              {results.map((r) => {
                const meta = RESULT[r.status];
                return (
                  <li key={r.url}>
                    <span style={{ minWidth: 0 }}>
                      <URLText url={r.url} link={r.status !== "invalid"} />
                      {(r.error || meta.why) && <span className={s.hint} style={{ display: "block", marginTop: 2 }}>{r.error ?? meta.why}</span>}
                    </span>
                    <Badge tone={meta.tone}>{meta.label}</Badge>
                  </li>
                );
              })}
            </ul>
          ) : (
            <p className={s.panelBody} style={{ color: "var(--ink-2)" }}>
              After you add URLs, each one is listed here with what happened to it.
            </p>
          )}
        </Panel>
      </div>
    </>
  );
}
