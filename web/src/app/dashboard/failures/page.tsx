"use client";

import { useState } from "react";
import { useDashboard } from "@/components/dashboard/Shell";
import { Empty, PageHead, Panel, URLText, styles as s } from "@/components/dashboard/ui";
import { crawler } from "@/lib/api";
import { ago, num } from "@/lib/format";
import { usePoll } from "@/lib/usePoll";

export default function Failures() {
  const { run } = useDashboard();
  const { data, error, refresh } = usePoll(crawler.deadLetters, 5000);
  const [busy, setBusy] = useState(false);

  async function act(fn: () => Promise<unknown>, msg: (r: never) => string) {
    setBusy(true);
    await run(fn as () => Promise<never>, msg);
    await refresh();
    setBusy(false);
  }

  const retryAll = () =>
    act(
      () => crawler.retryDeadLetters(),
      (r: { requeued: number }) =>
        r.requeued ? `${r.requeued} URL${r.requeued === 1 ? "" : "s"} back in the queue.` : "Nothing to retry: those URLs are out of scope or already queued.",
    );

  function discardAll() {
    if (!confirm(`Discard all ${data?.total ?? 0} failed URLs? They won't be retried.`)) return;
    act(() => crawler.discardDeadLetters(), (r: { removed: number }) => `Discarded ${r.removed} failed URLs.`);
  }

  const entries = data?.entries ?? [];

  return (
    <>
      <PageHead
        title="Failures"
        description="URLs the crawler gave up on after every retry. Retrying puts them back in the queue at their original depth."
        actions={
          entries.length > 0 && (
            <>
              <button className="btn btn-primary" onClick={retryAll} disabled={busy}>
                Retry all
              </button>
              <button className="btn btn-danger" onClick={discardAll} disabled={busy}>
                Discard all
              </button>
            </>
          )
        }
      />

      <Panel flush meta={data && data.total > entries.length ? `showing ${entries.length} of ${num(data.total)}` : undefined}>
        {error && !data ? (
          <Empty title="Couldn't load failures">{error.message}</Empty>
        ) : !data ? (
          <Empty title="Loading failures…" />
        ) : entries.length > 0 ? (
          <div className={s.tableWrap}>
            <table className={s.table}>
              <thead>
                <tr>
                  <th>URL and error</th>
                  <th className={s.num}>Attempts</th>
                  <th className={s.num}>Depth</th>
                  <th>Failed</th>
                  <th>
                    <span className="visually-hidden">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e, i) => (
                  <tr key={`${e.url}-${e.failed_at}-${i}`}>
                    <td>
                      <URLText url={e.url} />
                      <span className={s.errorText}>{e.error}</span>
                    </td>
                    <td className={s.num}>{e.attempts}</td>
                    <td className={s.num}>{e.depth}</td>
                    <td className={`${s.nowrap} ${s.muted}`} title={new Date(e.failed_at).toLocaleString()}>
                      {ago(e.failed_at)}
                    </td>
                    <td>
                      <div className={s.rowActions}>
                        <button
                          className="btn btn-sm"
                          disabled={busy}
                          onClick={() =>
                            act(
                              () => crawler.retryDeadLetters([e.url]),
                              (r: { requeued: number }) => (r.requeued ? "Back in the queue." : "Not retried: it's out of scope or already queued."),
                            )
                          }
                        >
                          Retry
                        </button>
                        <button
                          className="btn btn-sm btn-quiet"
                          disabled={busy}
                          onClick={() => act(() => crawler.discardDeadLetters([e.url]), () => "Discarded.")}
                        >
                          Discard
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty title="No failures">Every URL so far was fetched, or is still being retried.</Empty>
        )}
      </Panel>
    </>
  );
}
