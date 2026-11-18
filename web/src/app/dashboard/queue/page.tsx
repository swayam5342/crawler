"use client";

import Link from "next/link";
import { useDashboard } from "@/components/dashboard/Shell";
import { Empty, PageHead, Panel, URLText, styles as s } from "@/components/dashboard/ui";
import { crawler } from "@/lib/api";
import { num } from "@/lib/format";
import { usePoll } from "@/lib/usePoll";

const LIMIT = 200;

export default function Queue() {
  const { run } = useDashboard();
  const { data, refresh } = usePoll(() => crawler.queue(LIMIT), 2000);

  async function remove(url: string) {
    await run(() => crawler.removeQueued(url), (r) => (r.removed ? "Removed from the queue." : "That URL was already picked up."));
    refresh();
  }

  return (
    <>
      <PageHead
        title="Queue"
        description="URLs waiting to be fetched, in the order they'll be picked up. Removing one skips it for good: it stays marked as seen."
      />

      <div className={s.grid}>
        <Panel title="Fetching now" meta={data ? `${data.in_flight.length}` : undefined} flush>
          {data && data.in_flight.length > 0 ? (
            <table className={s.table}>
              <tbody>
                {data.in_flight.map((it) => (
                  <tr key={it.url}>
                    <td>
                      <URLText url={it.url} />
                    </td>
                    <td className={`${s.num} ${s.muted}`}>depth {it.depth}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <Empty title="Nothing in flight" />
          )}
        </Panel>

        <Panel
          title="Waiting"
          meta={data ? (data.total_queued > LIMIT ? `showing ${LIMIT} of ${num(data.total_queued)}` : num(data.total_queued)) : undefined}
          flush
        >
          {data && data.queued.length > 0 ? (
            <div className={s.tableWrap}>
              <table className={s.table}>
                <thead>
                  <tr>
                    <th>URL</th>
                    <th className={s.num}>Depth</th>
                    <th>
                      <span className="visually-hidden">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.queued.map((it) => (
                    <tr key={it.url}>
                      <td>
                        <URLText url={it.url} />
                      </td>
                      <td className={s.num}>{it.depth}</td>
                      <td className={s.rowActions}>
                        <button className="btn btn-sm btn-quiet" onClick={() => remove(it.url)} aria-label={`Remove ${it.url} from the queue`}>
                          Remove
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty title="The queue is empty">
              {data ? "Everything discovered so far has been fetched." : "Loading…"}
              <br />
              <Link href="/dashboard/add" className="btn btn-primary">
                Add URLs
              </Link>
            </Empty>
          )}
        </Panel>
      </div>
    </>
  );
}
