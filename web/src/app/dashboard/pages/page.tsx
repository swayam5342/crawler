"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Badge, Empty, PageHead, Panel, URLText, statusTone, styles as s } from "@/components/dashboard/ui";
import { ApiError, crawler } from "@/lib/api";
import { ago, num } from "@/lib/format";
import { usePoll } from "@/lib/usePoll";

const PAGE_SIZE = 50;

export default function CrawledPages() {
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [domain, setDomain] = useState("");
  const [offset, setOffset] = useState(0);

  // Debounce typing so every keystroke isn't a database query.
  useEffect(() => {
    const t = setTimeout(() => {
      setQuery(search.trim());
      setOffset(0);
    }, 300);
    return () => clearTimeout(t);
  }, [search]);

  const { data, error, loading } = usePoll(
    () => crawler.pages({ q: query, domain, limit: PAGE_SIZE, offset }),
    5000,
    [query, domain, offset],
  );
  const { data: domains } = usePoll(crawler.pageDomains, 15000);

  const unsupported = error instanceof ApiError && error.status === 501;
  const total = data?.total ?? 0;
  const from = total ? offset + 1 : 0;
  const to = Math.min(offset + PAGE_SIZE, total);

  return (
    <>
      <PageHead
        title="Crawled pages"
        description="Every page stored so far, newest first. Pages marked noindex are not stored when honor_noindex is on."
      />

      <Panel flush>
        <div className={s.toolbar}>
          <label htmlFor="page-search" className="visually-hidden">
            Search pages
          </label>
          <input
            id="page-search"
            className="input"
            type="search"
            placeholder="Search by URL or title"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <label htmlFor="page-domain" className="visually-hidden">
            Domain
          </label>
          <select
            id="page-domain"
            className="select"
            value={domain}
            onChange={(e) => {
              setDomain(e.target.value);
              setOffset(0);
            }}
          >
            <option value="">All domains</option>
            {domains?.domains.map((d) => (
              <option key={d.domain} value={d.domain}>
                {d.domain} ({num(d.pages)})
              </option>
            ))}
          </select>
        </div>

        {unsupported ? (
          <Empty title="Browsing isn't available">The configured storage backend doesn't support listing pages.</Empty>
        ) : error && !data ? (
          <Empty title="Couldn't load pages">{error.message}</Empty>
        ) : data && data.pages.length > 0 ? (
          <>
            <div className={s.tableWrap}>
              <table className={s.table}>
                <thead>
                  <tr>
                    <th>Page</th>
                    <th>Status</th>
                    <th>Language</th>
                    <th>Fetched</th>
                  </tr>
                </thead>
                <tbody>
                  {data.pages.map((p) => (
                    <tr key={p.url}>
                      <td>
                        <span className={s.title}>{p.title || <span className={s.muted}>Untitled</span>}</span>
                        <URLText url={p.url} />
                      </td>
                      <td>
                        <Badge tone={statusTone(p.status_code)}>{p.status_code}</Badge>
                      </td>
                      <td className={s.muted}>{p.language || "–"}</td>
                      <td className={`${s.nowrap} ${s.muted}`} title={new Date(p.fetched_at).toLocaleString()}>
                        {ago(p.fetched_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className={s.pager}>
              <span>
                {num(from)}–{num(to)} of {num(total)}
              </span>
              <span className={s.actions}>
                <button className="btn btn-sm" onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))} disabled={offset === 0}>
                  Previous
                </button>
                <button className="btn btn-sm" onClick={() => setOffset(offset + PAGE_SIZE)} disabled={to >= total}>
                  Next
                </button>
              </span>
            </div>
          </>
        ) : loading ? (
          <Empty title="Loading pages…" />
        ) : query || domain ? (
          <Empty title="No pages match">Try a shorter search, or pick All domains.</Empty>
        ) : (
          <Empty title="No pages stored yet">
            Pages appear here as soon as they&apos;re fetched.
            <br />
            <Link href="/dashboard/add" className="btn btn-primary">
              Add URLs
            </Link>
          </Empty>
        )}
      </Panel>
    </>
  );
}
