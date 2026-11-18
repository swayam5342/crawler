const nf = new Intl.NumberFormat("en-US");

export const num = (n: number) => nf.format(n);

export function pct(n: number): string {
  if (!Number.isFinite(n) || n === 0) return "0%";
  if (n < 0.001) return "<0.1%";
  return `${(n * 100).toFixed(n < 0.1 ? 1 : 0)}%`;
}

export function rate(n: number): string {
  if (n === 0) return "0";
  return n < 10 ? n.toFixed(1) : Math.round(n).toString();
}

export function duration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  if (m < 60) return `${m}m ${seconds % 60}s`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h ${m % 60}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
}

export function ago(iso: string, now = Date.now()): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t) || t <= 0) return "never";
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 5) return "just now";
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h}h ago`;
  return new Date(t).toLocaleDateString();
}

/** Splits a URL into host and the rest, for compact two-tone display. */
export function splitURL(raw: string): { host: string; rest: string } {
  try {
    const u = new URL(raw);
    const rest = u.pathname + u.search;
    return { host: u.host, rest: rest === "/" ? "/" : rest };
  } catch {
    return { host: "", rest: raw };
  }
}
