// Typed client for the crawler control API, via the /api/crawler proxy.

export type RunState = "running" | "idle" | "paused" | "stopping" | "finished";

export interface FrontierStats {
  queued: number;
  in_flight: number;
  visited: number;
  closed: boolean;
}

export interface Totals {
  success: number;
  failure: number;
  blocked: number;
  links_found: number;
}

export interface ThroughputPoint {
  t: number;
  ok: number;
  fail: number;
}

export interface DomainStats {
  domain: string;
  success: number;
  failure: number;
  blocked: number;
  avg_duration_ms: number;
  last_seen: string;
  last_error?: string;
  breaker?: "closed" | "open" | "half-open" | string;
}

export interface Status {
  state: RunState;
  keep_alive: boolean;
  started_at: string;
  stats_since: string;
  uptime_seconds: number;
  frontier: FrontierStats;
  totals: Totals;
  pages_per_second: number;
  error_rate: number;
  dead_letters: number;
  throughput: ThroughputPoint[];
  domains: DomainStats[];
  allowed_domains: string[];
}

export interface CrawlEvent {
  url: string;
  domain: string;
  depth: number;
  outcome: "success" | "failure" | "blocked";
  links: number;
  error?: string;
  duration_ms: number;
  at: string;
}

export interface QueueItem {
  url: string;
  depth: number;
}

export interface QueueView {
  queued: QueueItem[];
  in_flight: QueueItem[];
  total_queued: number;
}

export interface StoredPage {
  url: string;
  domain: string;
  status_code: number;
  title: string;
  language: string;
  no_index: boolean;
  content_hash: string;
  fetched_at: string;
}

export interface PageList {
  pages: StoredPage[];
  total: number;
}

export interface DeadLetter {
  url: string;
  domain: string;
  depth: number;
  attempts: number;
  error: string;
  failed_at: string;
}

export interface AddResult {
  url: string;
  status: "queued" | "duplicate" | "out_of_scope" | "invalid" | "closed";
  error?: string;
}

export interface ResetOptions {
  queue: boolean;
  dead_letters: boolean;
  pages: boolean;
  stats: boolean;
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly unreachable = false,
  ) {
    super(message);
  }
}

const BASE = "/api/crawler";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(BASE + path, { cache: "no-store", ...init });
  } catch {
    throw new ApiError("The dashboard server isn't responding.", 0, true);
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(data.error ?? `Request failed with status ${res.status}.`, res.status, !!data.unreachable);
  }
  return data as T;
}

function send<T>(method: "POST" | "DELETE", path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

const q = (params: Record<string, string | number | undefined>) => {
  const s = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") s.set(k, String(v));
  const str = s.toString();
  return str ? `?${str}` : "";
};

export const crawler = {
  status: () => request<Status>("/status"),
  events: (limit = 50) => request<{ events: CrawlEvent[] }>(`/events${q({ limit })}`),
  queue: (limit = 200) => request<QueueView>(`/queue${q({ limit })}`),
  removeQueued: (url: string) => send<{ removed: boolean }>("DELETE", `/queue${q({ url })}`),
  addURLs: (urls: string[], opts: { allow_domain: boolean; recrawl: boolean }) =>
    send<{ results: AddResult[] }>("POST", "/urls", { urls, ...opts }),
  pause: () => send<{ state: RunState }>("POST", "/pause", {}),
  resume: () => send<{ state: RunState }>("POST", "/resume", {}),
  pages: (p: { q?: string; domain?: string; limit?: number; offset?: number }) =>
    request<PageList>(`/pages${q(p)}`),
  pageDomains: () => request<{ domains: { domain: string; pages: number }[] }>("/pages/domains"),
  deadLetters: () => request<{ entries: DeadLetter[]; total: number }>("/deadletters"),
  retryDeadLetters: (urls: string[] = []) => send<{ requeued: number }>("POST", "/deadletters/retry", { urls }),
  discardDeadLetters: (urls: string[] = []) => send<{ removed: number }>("POST", "/deadletters/discard", { urls }),
  allowDomain: (domain: string) => send<{ added: boolean }>("POST", "/domains", { domain }),
  disallowDomain: (domain: string) => send<{ removed: boolean }>("DELETE", `/domains${q({ domain })}`),
  reset: (opts: ResetOptions) =>
    send<{ result: { queue_cleared: boolean; dead_letters_removed: number; pages_deleted: number; stats_cleared: boolean } }>(
      "POST",
      "/reset",
      opts,
    ),
  config: () => request<Record<string, unknown>>("/config"),
};
