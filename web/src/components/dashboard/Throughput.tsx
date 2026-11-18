import type { ThroughputPoint } from "@/lib/api";
import s from "./dashboard.module.css";

/** Stacked per-second bars for the last 60 seconds: fetched + failed. */
export function Throughput({ points }: { points: ThroughputPoint[] }) {
  const W = 600;
  const H = 140;
  const n = Math.max(points.length, 1);
  const max = Math.max(1, ...points.map((p) => p.ok + p.fail));
  const slot = W / n;
  const bar = Math.max(1, slot - 2);
  const total = points.reduce((a, p) => a + p.ok + p.fail, 0);
  const failed = points.reduce((a, p) => a + p.fail, 0);

  return (
    <div>
      <svg
        className={s.chart}
        viewBox={`0 0 ${W} ${H}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={`${total} requests in the last minute, ${failed} of them failed. Peak ${max} per second.`}
      >
        <line x1="0" x2={W} y1={H - 0.5} y2={H - 0.5} stroke="var(--line)" />
        <line x1="0" x2={W} y1={0.5} y2={0.5} stroke="var(--line-2)" strokeDasharray="3 4" />
        {points.map((p, i) => {
          const okH = (p.ok / max) * (H - 4);
          const failH = (p.fail / max) * (H - 4);
          const x = i * slot + 1;
          return (
            <g key={p.t}>
              {p.ok > 0 && <rect x={x} y={H - okH} width={bar} height={okH} rx="1.5" fill="var(--moss)" />}
              {p.fail > 0 && (
                <rect x={x} y={H - okH - failH - (p.ok > 0 ? 1 : 0)} width={bar} height={failH} rx="1.5" fill="var(--brick)" />
              )}
            </g>
          );
        })}
      </svg>
      <div className={s.chartAxis}>
        <span>60s ago</span>
        <span>peak {max}/s</span>
        <span>now</span>
      </div>
    </div>
  );
}
