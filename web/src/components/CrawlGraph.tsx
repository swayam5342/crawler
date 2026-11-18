// A breadth-first crawl from one seed URL, drawn as a drafting plate: depth
// rings, white line work, and a key inside the frame. Outcomes are told
// apart by the shape of the mark, since the sheet has one ink. The layout
// is deterministic (same output on server and client). The plate draws
// itself once on load; under reduced motion it is shown finished.

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
  const nodes: GNode[] = [{ id: 0, parent: null, depth: 0, angle: 0, x: CX, y: CY, outcome: "seed", order: 0 }];
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
const STEP_MS = 22;
const r1 = (n: number) => Math.round(n * 10) / 10;

/** One mark per outcome, drawn around (x, y). */
function Mark({ outcome, x, y, size }: { outcome: Outcome; x: number; y: number; size: number }) {
  const d = size * 0.62;
  switch (outcome) {
    case "seed":
      return (
        <g>
          <circle cx={x} cy={y} r={size + 5} className="pl-ring" />
          <circle cx={x} cy={y} r={size} className="pl-fill" />
        </g>
      );
    case "ok":
      return <circle cx={x} cy={y} r={size} className="pl-fill" />;
    case "fail":
      return (
        <g>
          <circle cx={x} cy={y} r={size} className="pl-hollow" />
          <path d={`M${r1(x - d)} ${r1(y - d)}L${r1(x + d)} ${r1(y + d)}M${r1(x + d)} ${r1(y - d)}L${r1(x - d)} ${r1(y + d)}`} className="pl-stroke" />
        </g>
      );
    case "blocked":
      return (
        <g>
          <circle cx={x} cy={y} r={size} className="pl-hollow" />
          <path d={`M${r1(x - size)} ${r1(y)}L${r1(x + size)} ${r1(y)}`} className="pl-stroke" />
        </g>
      );
    case "scope":
      return <circle cx={x} cy={y} r={size} className="pl-dashed" />;
  }
}

const KEY: { outcome: Outcome; label: string }[] = [
  { outcome: "ok", label: "Fetched" },
  { outcome: "fail", label: "Failed after retries" },
  { outcome: "blocked", label: "Disallowed by robots.txt" },
  { outcome: "scope", label: "Outside allowed domains" },
];

export function CrawlGraph() {
  return (
    <figure className="plate">
      <svg
        className="plate-svg"
        viewBox="0 0 640 680"
        role="img"
        aria-labelledby="plate-title plate-desc"
      >
        <title id="plate-title">Crawl diagram: links found from one seed URL, three levels deep</title>
        <desc id="plate-desc">
          A seed page in the centre links out to pages on three depth rings. Filled marks were fetched, crossed marks
          failed after retries, barred marks were disallowed by robots.txt, and dashed marks point outside the allowed
          domains and were not followed. Failed and blocked pages are never expanded.
        </desc>

        <defs>
          <pattern id="pl-minor" width="24" height="24" patternUnits="userSpaceOnUse">
            <path d="M24 0H0V24" className="pl-grid" />
          </pattern>
          <pattern id="pl-major" width="120" height="120" patternUnits="userSpaceOnUse">
            <rect width="120" height="120" fill="url(#pl-minor)" />
            <path d="M120 0H0V120" className="pl-grid-2" />
          </pattern>
        </defs>
        <rect x="0" y="0" width="640" height="600" fill="url(#pl-major)" />

        {RADII.slice(1).map((r, i) => (
          <g key={r}>
            <circle cx={CX} cy={CY} r={r} className="pl-depth" />
            <text x={CX + 8} y={CY - r - 8} className="pl-label pl-halo">
              {`depth ${i + 1}`}
            </text>
          </g>
        ))}

        {GRAPH.filter((n) => n.parent !== null).map((n) => {
          const p = GRAPH[n.parent!];
          const len = Math.hypot(n.x - p.x, n.y - p.y);
          return (
            <line
              key={`e${n.id}`}
              x1={r1(p.x)}
              y1={r1(p.y)}
              x2={r1(n.x)}
              y2={r1(n.y)}
              className={`pl-edge${n.outcome === "scope" ? " pl-edge-scope" : ""}`}
              style={{ "--len": r1(len), animationDelay: `${n.order * STEP_MS}ms` } as React.CSSProperties}
            />
          );
        })}

        {GRAPH.map((n) => (
          <g key={`n${n.id}`} className="pl-node" style={{ animationDelay: `${n.order * STEP_MS + 120}ms` }}>
            <Mark outcome={n.outcome} x={r1(n.x)} y={r1(n.y)} size={n.outcome === "seed" ? 8 : n.depth === 1 ? 6.5 : 5} />
          </g>
        ))}

        <text x={CX} y={CY + 30} textAnchor="middle" className="pl-label pl-halo">
          seed URL
        </text>

        {/* The key, set in the plate's foot like a drawing's legend. */}
        <line x1="0" y1="604" x2="640" y2="604" className="pl-keyline" />
        {KEY.map((k, i) => {
          const x = 18 + (i % 2) * 300;
          const y = 632 + Math.floor(i / 2) * 30;
          return (
            <g key={k.outcome}>
              <Mark outcome={k.outcome} x={x} y={y - 5} size={5.5} />
              <text x={x + 16} y={y} className="pl-key">
                {k.label}
              </text>
            </g>
          );
        })}
      </svg>
    </figure>
  );
}
