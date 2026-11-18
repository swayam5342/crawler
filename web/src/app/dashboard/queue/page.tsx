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
  // Treat a missing list as empty, so an odd API response can't crash the page.
  const queued = data?.queued ?? [];
  const inFlight = data?.in_flight ?? [];

  async function remove(url: string) {
    await run(() => crawler.removeQueued(url), (r) => (r.removed ? "Removed from the queue." : "That URL was already picked up."));
    refresh();
  }

  return (
    <>
      <PageHead
        title="Queue"
        description="URLs waiting to be fetched, in the order they will be fetched. Removing a URL skips it; it won't be added again automatically."
      />

      <div className={s.grid}>
        <Panel title="In progress" meta={data ? `${inFlight.length}, fetching or waiting for their domain's turn` : undefined} flush>
          {inFlight.length > 0 ? (
            <table className={s.table}>
              <tbody>
                {inFlight.map((it) => (
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
            <Empty title="Nothing is in progress right now." />
          )}
        </Panel>

        <Panel
          title="Waiting in queue"
          meta={data ? (data.total_queued > LIMIT ? `showing ${LIMIT} of ${num(data.total_queued)}` : num(data.total_queued)) : undefined}
          flush
        >
          {queued.length > 0 ? (
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
                  {queued.map((it) => (
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
