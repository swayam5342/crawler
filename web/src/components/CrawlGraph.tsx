// A breadth-first crawl drawn as depth rings around a seed URL. Purely
// presentational and deterministic (same output on server and client);
// the reveal is CSS-only and is skipped under reduced motion.
import styles from "./CrawlGraph.module.css";

type Outcome = "seed" | "ok" | "fail" | "blocked" | "scope";

interface GNode {
  id: number;
  parent: number | null;
  depth: number;
  angle: number;
  x: number;
  y: number;
  outcome: Outcome;
  order: number;
}

const CX = 320;
const CY = 300;
const RADII = [0, 104, 186, 262];

// Small seeded PRNG so the layout is stable across renders.
function mulberry32(seed: number) {
  return () => {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function buildGraph(): GNode[] {
  const rand = mulberry32(20261118);
  const nodes: GNode[] = [
    { id: 0, parent: null, depth: 0, angle: 0, x: CX, y: CY, outcome: "seed", order: 0 },
  ];
  const fanout = [7, 3, 2];
  let frontier = [nodes[0]];
  for (let depth = 1; depth <= 3; depth++) {
    const next: GNode[] = [];
    const span = depth === 1 ? (Math.PI * 2) / fanout[0] : (Math.PI * 2) / (frontier.length * 2.6);
    for (const parent of frontier) {
      if (parent.outcome !== "ok" && parent.outcome !== "seed") continue; // failures don't expand
      const count = depth === 1 ? fanout[0] : Math.max(1, Math.round(fanout[depth - 1] * (0.55 + rand() * 0.7)));
      for (let i = 0; i < count; i++) {
        const angle =
          depth === 1
            ? -Math.PI / 2 + i * span + rand() * 0.2
            : parent.angle + (i - (count - 1) / 2) * span * 1.15 + (rand() - 0.5) * 0.12;
        const r = RADII[depth] + (rand() - 0.5) * 14;
        const roll = rand();
        const outcome: Outcome =
          depth === 1
            ? roll < 0.86 ? "ok" : "blocked"
            : roll < 0.74 ? "ok" : roll < 0.84 ? "fail" : roll < 0.93 ? "blocked" : "scope";
        const node: GNode = {
          id: nodes.length,
          parent: parent.id,
          depth,
          angle,
          x: CX + Math.cos(angle) * r,
          y: CY + Math.sin(angle) * r,
          outcome,
          order: nodes.length,
        };
        nodes.push(node);
        next.push(node);
      }
    }
    frontier = next;
  }
  return nodes;
}

const GRAPH = buildGraph();
const COUNTS = GRAPH.reduce(
  (acc, n) => {
    if (n.outcome !== "seed") acc[n.outcome]++;
    return acc;
  },
  { ok: 0, fail: 0, blocked: 0, scope: 0 } as Record<Exclude<Outcome, "seed">, number>,
);

const STEP_MS = 55;

export function CrawlGraph() {
  return (
    <figure className={styles.figure}>
      <svg
        className={styles.svg}
        viewBox="0 0 640 600"
        role="img"
        aria-labelledby="crawl-graph-title crawl-graph-desc"
      >
        <title id="crawl-graph-title">A crawl spreading out from one seed URL</title>
        <desc id="crawl-graph-desc">
          {`Starting from a seed page, the crawler discovered ${GRAPH.length - 1} links over three levels of depth: ${COUNTS.ok} fetched, ${COUNTS.fail} failed after retries, ${COUNTS.blocked} skipped because robots.txt disallows them, and ${COUNTS.scope} left alone because they point outside the allowed domains.`}
        </desc>

        {RADII.slice(1).map((r, i) => (
          <g key={r}>
            <circle cx={CX} cy={CY} r={r} className={styles.ring} />
            <text x={CX + 6} y={CY - r - 6} className={styles.ringLabel}>
              depth {i + 1}
            </text>
          </g>
        ))}

        {GRAPH.filter((n) => n.parent !== null).map((n) => {
          const p = GRAPH[n.parent!];
          const len = Math.hypot(n.x - p.x, n.y - p.y);
          return (
            <line
              key={`e${n.id}`}
              x1={p.x}
              y1={p.y}
              x2={n.x}
              y2={n.y}
              className={`${styles.edge} ${n.outcome === "scope" ? styles.edgeScope : ""}`}
              style={
                {
                  "--len": len,
                  animationDelay: `${n.order * STEP_MS}ms`,
                } as React.CSSProperties
              }
            />
          );
        })}

        {GRAPH.map((n) => (
          <circle
            key={`n${n.id}`}
            cx={n.x}
            cy={n.y}
            r={n.outcome === "seed" ? 11 : n.depth === 1 ? 7 : 5.5}
            className={`${styles.node} ${styles[n.outcome]}`}
            style={{ animationDelay: `${n.order * STEP_MS + 120}ms` }}
          />
        ))}

        <text x={CX} y={CY + 30} textAnchor="middle" className={styles.seedLabel}>
          seed
        </text>
      </svg>

      <figcaption className={styles.legend}>
        <span>
          <i className={`${styles.swatch} ${styles.ok}`} /> {COUNTS.ok} fetched
        </span>
        <span>
          <i className={`${styles.swatch} ${styles.fail}`} /> {COUNTS.fail} failed after retries
        </span>
        <span>
          <i className={`${styles.swatch} ${styles.blocked}`} /> {COUNTS.blocked} disallowed by robots.txt
        </span>
        <span>
          <i className={`${styles.swatch} ${styles.scope}`} /> {COUNTS.scope} outside allowed domains
        </span>
      </figcaption>
    </figure>
  );
}
