"use client";

import Link from "next/link";
import { useState } from "react";
import { useDashboard } from "@/components/dashboard/Shell";
import { Badge, BreakerBadge, Empty, PageHead, Panel, URLText, styles as s } from "@/components/dashboard/ui";
import { crawler, type CrawlEvent } from "@/lib/api";
import { ago, duration, num, pct, rate } from "@/lib/format";
import { usePoll } from "@/lib/usePoll";

const RESULT: Record<CrawlEvent["outcome"], { tone: "ok" | "bad" | "warn"; label: string }> = {
  success: { tone: "ok", label: "Fetched" },
  failure: { tone: "bad", label: "Failed" },
  blocked: { tone: "warn", label: "Blocked by robots.txt" },
};

export default function Overview() {
  const { status, run } = useDashboard();
  const { data: feed } = usePoll(() => crawler.events(50), 2000);
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
        if (first.status === "queued") return `Added ${first.url} to the queue.`;
        if (first.status === "duplicate") return "That URL was already crawled or is in the queue. To fetch it again, use Add URLs and tick Recrawl.";
        if (first.status === "out_of_scope") return "That domain isn't allowed. Add it under Settings first.";
        return first.error ?? `Not added: ${first.status}`;
      },
    );
    if (res?.results[0]?.status === "queued") setUrl("");
  }

  const t = status?.totals;
  const fr = status?.frontier;
  const dash = "–";

  return (
    <>
      <PageHead
        title="Overview"
        description={
          status
            ? `Running for ${duration(status.uptime_seconds)}. ${status.keep_alive ? "The crawler keeps running when the queue is empty." : "The crawler stops when the queue is empty."}`
            : undefined
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
          placeholder="Enter a URL to crawl, e.g. https://example.com/"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          autoComplete="off"
        />
        <button className="btn btn-primary" type="submit" disabled={!url.trim()}>
          Add to queue
        </button>
      </form>

      <dl className={s.stats}>
        <div className={s.stat}>
          <dt>Pages crawled</dt>
          <dd>{t ? num(t.success) : dash}</dd>
        </div>
        <div className={s.stat}>
          <dt>Pages per second</dt>
          <dd>{status ? rate(status.pages_per_second) : dash}</dd>
        </div>
        <div className={s.stat}>
          <dt>In queue</dt>
          <dd>{fr ? num(fr.queued) : dash}</dd>
        </div>
        <div className={s.stat}>
          <dt>Fetching now</dt>
          <dd>{fr ? num(fr.in_flight) : dash}</dd>
        </div>
        <div className={s.stat}>
          <dt>Errors</dt>
          <dd>{t ? `${num(t.failure)} (${pct(status!.error_rate)})` : dash}</dd>
        </div>
        <div className={s.stat}>
          <dt>Blocked by robots.txt</dt>
          <dd>{t ? num(t.blocked) : dash}</dd>
        </div>
        <div className={`${s.stat} ${status?.dead_letters ? s.statBad : ""}`}>
          <dt>Failed URLs</dt>
          <dd>{status ? num(status.dead_letters) : dash}</dd>
        </div>
        <div className={s.stat}>
          <dt>URLs seen</dt>
          <dd>{fr ? num(fr.visited) : dash}</dd>
        </div>
      </dl>
      {!!status?.dead_letters && (
        <p className={s.hint} style={{ marginTop: -8, marginBottom: 16 }}>
          {status.dead_letters} URL{status.dead_letters === 1 ? "" : "s"} failed after all retries.{" "}
          <Link href="/dashboard/failures">Review failed URLs</Link>
        </p>
      )}

      <div className={s.grid}>
        <Panel title="Domains" meta={status ? `${status.domains.length} domain${status.domains.length === 1 ? "" : "s"}` : undefined} flush>
          {status && status.domains.length > 0 ? (
            <div className={s.tableWrap}>
              <table className={s.table}>
                <thead>
                  <tr>
                    <th>Domain</th>
                    <th className={s.num}>Crawled</th>
                    <th className={s.num}>Errors</th>
                    <th className={s.num}>Blocked</th>
                    <th className={s.num}>Avg response</th>
                    <th>State</th>
                    <th>Last error</th>
                  </tr>
                </thead>
                <tbody>
                  {status.domains.map((d) => (
                    <tr key={d.domain}>
                      <td className="mono">{d.domain}</td>
                      <td className={s.num}>{num(d.success)}</td>
                      <td className={s.num}>{num(d.failure)}</td>
                      <td className={s.num}>{num(d.blocked)}</td>
                      <td className={s.num}>{d.avg_duration_ms ? `${num(d.avg_duration_ms)} ms` : dash}</td>
                      <td>
                        <BreakerBadge state={d.breaker} />
                      </td>
                      <td>{d.last_error ? <span className={s.errorText}>{d.last_error}</span> : <span className={s.muted}>{dash}</span>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty title="No domains crawled yet." />
          )}
        </Panel>

        <Panel title="Recent activity" meta="Newest first, updates every 2 seconds" flush>
          {feed && feed.events.length > 0 ? (
            <div className={s.tableWrap}>
              <table className={s.table}>
                <thead>
                  <tr>
                    <th>Time</th>
                    <th>Result</th>
                    <th>URL</th>
                    <th className={s.num}>Links found</th>
                  </tr>
                </thead>
                <tbody>
                  {feed.events.map((e, i) => (
                    <tr key={`${e.at}-${e.url}-${i}`}>
                      <td className={`${s.nowrap} ${s.muted}`} title={new Date(e.at).toLocaleString()}>
                        {ago(e.at)}
                      </td>
                      <td>
                        <Badge tone={RESULT[e.outcome].tone}>{RESULT[e.outcome].label}</Badge>
                      </td>
                      <td>
                        <URLText url={e.url} />
                        {e.error && <span className={s.errorText}>{e.error}</span>}
                      </td>
                      <td className={s.num}>{e.outcome === "success" ? e.links : dash}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty title="Nothing crawled yet.">Enter a URL above to start.</Empty>
          )}
        </Panel>
      </div>
    </>
  );
}
