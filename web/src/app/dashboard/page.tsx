"use client";

import Link from "next/link";
import { useState } from "react";
import { useDashboard } from "@/components/dashboard/Shell";
import { Throughput } from "@/components/dashboard/Throughput";
import { BreakerBadge, Empty, PageHead, Panel, URLText, styles as s } from "@/components/dashboard/ui";
import { crawler, type CrawlEvent } from "@/lib/api";
import { ago, duration, num, pct, rate } from "@/lib/format";
import { usePoll } from "@/lib/usePoll";

const OUTCOME_COLOR: Record<CrawlEvent["outcome"], string> = {
  success: "var(--moss)",
  failure: "var(--brick)",
  blocked: "var(--amber)",
};

export default function Overview() {
  const { status, run } = useDashboard();
  const { data: feed } = usePoll(() => crawler.events(60), 2000);
  const [url, setUrl] = useState("");

  async function quickAdd(e: React.FormEvent) {
    e.preventDefault();
    const value = url.trim();
    if (!value) return;
    const res = await run(
      () => crawler.addURLs([value], { allow_domain: true, recrawl: false }),
      (r) => {
        const first = r.results[0];
        if (!first) return "Nothing to add.";
        if (first.status === "queued") return `Queued ${first.url}`;
        if (first.status === "duplicate") return "Already crawled or queued. Use Add URLs with Recrawl to fetch it again.";
        if (first.status === "out_of_scope") return "That domain isn't allowed. Add it under Settings first.";
        return first.error ?? `Not added: ${first.status}`;
      },
    );
    if (res?.results[0]?.status === "queued") setUrl("");
  }

  const t = status?.totals;
  const fr = status?.frontier;

  return (
    <>
      <PageHead
        title="Overview"
        description={
          status
            ? `Up ${duration(status.uptime_seconds)}. ${status.keep_alive ? "Runs as a service: it keeps waiting for new URLs." : "Batch mode: exits when the queue is empty."}`
            : "Connecting to the crawler…"
        }
      />

      <form className={s.quickAdd} onSubmit={quickAdd}>
        <label htmlFor="quick-url" className="visually-hidden">
          URL to crawl
        </label>
        <input
          id="quick-url"
          className="input"
          type="text"
          inputMode="url"
          placeholder="https://example.com/ (paste a URL to crawl)"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          autoComplete="off"
        />
        <button className="btn btn-primary" type="submit" disabled={!url.trim()}>
          Crawl this URL
        </button>
      </form>

      <dl className={`${s.panel} ${s.figures}`}>
        <div className={s.figure}>
          <dt>Pages fetched</dt>
          <dd>{t ? num(t.success) : "–"}</dd>
          <small>{t ? `${num(t.links_found)} links found` : " "}</small>
        </div>
        <div className={s.figure}>
          <dt>Pages per second</dt>
          <dd>{status ? rate(status.pages_per_second) : "–"}</dd>
          <small>last 10 seconds</small>
        </div>
        <div className={s.figure}>
          <dt>Queued</dt>
          <dd>{fr ? num(fr.queued) : "–"}</dd>
          <small>{fr ? `${num(fr.visited)} URLs seen` : " "}</small>
        </div>
        <div className={s.figure}>
          <dt>Fetching now</dt>
          <dd>{fr ? num(fr.in_flight) : "–"}</dd>
          <small>{t ? `${num(t.blocked)} blocked by robots.txt` : " "}</small>
        </div>
        <div className={`${s.figure} ${status && status.error_rate > 0.1 ? s.figureBad : ""}`}>
          <dt>Error rate</dt>
          <dd>{status ? pct(status.error_rate) : "–"}</dd>
          <small>{t ? `${num(t.failure)} failed requests` : " "}</small>
        </div>
        <div className={`${s.figure} ${status?.dead_letters ? s.figureBad : ""}`}>
          <dt>Gave up on</dt>
          <dd>{status ? num(status.dead_letters) : "–"}</dd>
          <small>
            {status?.dead_letters ? <Link href="/dashboard/failures">Review failures</Link> : "after all retries"}
          </small>
        </div>
      </dl>

      <div className={`${s.grid} ${s.gridOverview}`}>
        <div className={s.grid}>
          <Panel
            title="Requests, last 60 seconds"
            meta={
              <span className={s.chartLegend}>
                <span>
                  <i style={{ background: "var(--moss)" }} />
                  Fetched
                </span>
                <span>
                  <i style={{ background: "var(--brick)" }} />
                  Failed
                </span>
              </span>
            }
          >
            <Throughput points={status?.throughput ?? []} />
          </Panel>

          <Panel title="Domains" meta={status ? `${status.domains.length} seen` : undefined} flush>
            {status && status.domains.length > 0 ? (
              <div className={s.tableWrap}>
                <table className={s.table}>
                  <thead>
                    <tr>
                      <th>Domain</th>
                      <th className={s.num}>Fetched</th>
                      <th className={s.num}>Failed</th>
                      <th className={s.num}>Blocked</th>
                      <th className={s.num}>Avg time</th>
                      <th>Health</th>
                    </tr>
                  </thead>
                  <tbody>
                    {status.domains.map((d) => (
                      <tr key={d.domain}>
                        <td>
                          <span className="mono">{d.domain}</span>
                          {d.last_error && d.breaker !== "closed" && <span className={s.errorText}>{d.last_error}</span>}
                        </td>
                        <td className={s.num}>{num(d.success)}</td>
                        <td className={s.num}>{num(d.failure)}</td>
                        <td className={s.num}>{num(d.blocked)}</td>
                        <td className={s.num}>{d.avg_duration_ms ? `${num(d.avg_duration_ms)} ms` : "–"}</td>
                        <td>
                          <BreakerBadge state={d.breaker} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <Empty title="No domains crawled yet">Domains show up here once their first page is fetched.</Empty>
            )}
          </Panel>
        </div>

        <Panel title="Live activity" meta="newest first" flush>
          {feed && feed.events.length > 0 ? (
            <ul className={s.feed}>
              {feed.events.map((e, i) => (
                <li key={`${e.at}-${e.url}-${i}`}>
                  <span
                    className={s.feedDot}
                    style={{ background: OUTCOME_COLOR[e.outcome] }}
                    aria-label={e.outcome}
                    role="img"
                  />
                  <span>
                    <URLText url={e.url} />
                    {e.error && <span className={s.errorText}>{e.error}</span>}
                    {e.outcome === "blocked" && <span className={s.errorText} style={{ color: "var(--amber)" }}>Disallowed by robots.txt</span>}
                  </span>
                  <span className={s.feedMeta}>
                    {e.outcome === "success" ? `${e.links} links, ` : ""}
                    {ago(e.at)}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <Empty title="Nothing crawled yet">
              Paste a URL above to start. Activity appears here as pages are fetched.
            </Empty>
          )}
        </Panel>
      </div>
    </>
  );
}
