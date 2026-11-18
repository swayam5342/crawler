import Link from "next/link";
import { CrawlGraph } from "@/components/CrawlGraph";
import { REPO_URL, SITE_NAME } from "@/lib/site";
import styles from "./page.module.css";

const PIPELINE = [
  {
    title: "Normalize and deduplicate",
    body: "Fragments are stripped, query parameters sorted, default ports dropped. A URL that has been seen before, in any spelling, is skipped.",
  },
  {
    title: "Check scope",
    body: "Only hosts on the allow-list (and their subdomains) get through. Links to anywhere else are noted and left alone.",
  },
  {
    title: "Ask robots.txt",
    body: "Each site's robots.txt is fetched once, cached with a TTL, and consulted before every request. Disallowed paths are never fetched.",
  },
  {
    title: "Wait for a slot",
    body: "A per-domain concurrency cap and token-bucket rate limit keep any one site from being flooded. A Crawl-delay directive overrides the default pace.",
  },
  {
    title: "Fetch, carefully",
    body: "Separate timeouts for DNS, connect, TLS and the whole request. Redirect loops are caught, bodies are capped, gzip and brotli are decoded, and HTTP/2 falls back to HTTP/1.1.",
  },
  {
    title: "Extract",
    body: "Links, title and language are pulled from the HTML whatever its encoding. nofollow and noindex are honored.",
  },
  {
    title: "Store",
    body: "Page metadata goes to Postgres and the raw body to local disk or S3, keyed by content hash. New links go back into the queue one level deeper.",
  },
];

const FAILURES = [
  {
    when: "A server answers 503, times out, or resets the connection",
    then: "The request is retried with exponential backoff and jitter: up to 4 attempts, starting at 500 ms and never waiting more than 30 s.",
  },
  {
    when: "A whole domain keeps failing",
    then: "After 5 failures in a row its circuit breaker opens. That domain gets no requests for a minute, then a single trial request decides whether to resume.",
  },
  {
    when: "A URL still fails after every retry",
    then: "It's written to the dead-letter log with the error and attempt count, and you can retry it from the dashboard. Nothing is dropped silently.",
  },
  {
    when: "A page redirects in a circle, or ten times over",
    then: "Loops are detected and the hop count is capped, so the crawler moves on instead of spinning.",
  },
  {
    when: "A response is enormous or the HTML is broken",
    then: "Bodies are cut off at 10 MiB, and the parser takes malformed markup and non-UTF-8 text in stride.",
  },
  {
    when: "The process crashes or is stopped",
    then: "The queue and visited set are checkpointed every 10 seconds. On SIGTERM, new work stops, in-flight requests finish, and the next start resumes from there.",
  },
];

const CONTROLS = [
  { title: "Add URLs", body: "Paste one or a thousand. Each comes back as queued, already seen, or out of scope, with the reason." },
  { title: "Pause and resume", body: "Pausing lets in-flight requests finish and keeps the queue intact for later." },
  { title: "Browse what was crawled", body: "Search stored pages by URL or title, filtered by domain." },
  { title: "Deal with failures", body: "See why each URL failed, then retry it or discard it, one at a time or all together." },
  { title: "Change scope", body: "Add or remove allowed domains while the crawl runs." },
  { title: "Start over", body: "Clear the queue, failure log, stored pages, or counters, in any combination." },
];

export default function Home() {
  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <Link href="/" className={styles.brand}>
          <svg width="22" height="22" viewBox="0 0 22 22" aria-hidden="true">
            <circle cx="11" cy="11" r="3.5" fill="currentColor" />
            <circle cx="11" cy="11" r="9.5" fill="none" stroke="currentColor" strokeWidth="1.2" strokeDasharray="2 3" />
          </svg>
          {SITE_NAME}
        </Link>
        <nav className={styles.nav} aria-label="Site">
          <a href="#how-it-works">How it works</a>
          <a href="#failures">Failure handling</a>
          <a href="#run">Run it</a>
          <Link href="/dashboard" className="btn btn-sm">
            Open dashboard
          </Link>
        </nav>
      </header>

      <main>
        <section className={styles.hero}>
          <div className={styles.heroText}>
            <h1 className={styles.headline}>A web crawler that knows when to back off.</h1>
            <p className={styles.lede}>
              It reads robots.txt, paces itself per domain, stops calling hosts that keep failing, and
              resumes where it stopped after a crash. You steer it from a live dashboard while it runs.
            </p>
            <div className={styles.ctas}>
              <Link href="/dashboard" className="btn btn-primary">
                Open the dashboard
              </Link>
              <a href="#run" className="btn">
                Run it locally
              </a>
            </div>
            <p className={styles.stack}>Written in Go. Stores to Postgres and disk or S3. Exposes Prometheus metrics.</p>
          </div>
          <div className={styles.heroGraph}>
            <CrawlGraph />
          </div>
        </section>

        <section id="how-it-works" className={styles.section}>
          <div className={styles.sectionHead}>
            <h2>What happens to every URL</h2>
            <p>
              Every link the crawler finds takes the same path, in this order. Any step can stop it,
              and the step that did is recorded.
            </p>
          </div>
          <ol className={styles.pipeline}>
            {PIPELINE.map((step) => (
              <li key={step.title}>
                <h3>{step.title}</h3>
                <p>{step.body}</p>
              </li>
            ))}
          </ol>
        </section>

        <section id="failures" className={styles.section}>
          <div className={styles.sectionHead}>
            <h2>When things go wrong</h2>
            <p>
              Real sites time out, loop, lie about their encoding and fall over. Each of those cases has
              a defined response, and every limit below can be changed in the config.
            </p>
          </div>
          <dl className={styles.failures}>
            {FAILURES.map((f) => (
              <div key={f.when} className={styles.failureRow}>
                <dt>{f.when}</dt>
                <dd>{f.then}</dd>
              </div>
            ))}
          </dl>
        </section>

        <section className={`${styles.section} ${styles.control}`}>
          <div className={styles.sectionHead}>
            <h2>Steer it while it runs</h2>
            <p>
              Run the crawler as a service and it waits for work instead of exiting. The dashboard
              talks to it over a small JSON API, which you can lock with a token.
            </p>
            <Link href="/dashboard" className="btn btn-primary">
              Open the dashboard
            </Link>
          </div>
          <ul className={styles.controls}>
            {CONTROLS.map((c) => (
              <li key={c.title}>
                <h3>{c.title}</h3>
                <p>{c.body}</p>
              </li>
            ))}
          </ul>
        </section>

        <section id="run" className={styles.section}>
          <div className={styles.sectionHead}>
            <h2>Run it</h2>
            <p>
              Docker Compose starts Postgres and the crawler. The dashboard is a Next.js app that
              proxies to the crawler&apos;s API on port 9090.
            </p>
          </div>
          <div className={styles.run}>
            <div>
              <h3>Start the crawler and the dashboard</h3>
              <pre className={styles.code}>
                <code>{`docker compose up --build

# or, without Docker for the dashboard:
cd web
cp .env.example .env.local
npm install
npm run dev`}</code>
              </pre>
              <p className={styles.note}>
                Then open <code>localhost:3000/dashboard</code> and add a URL.
              </p>
            </div>
            <div>
              <h3>Tune it in configs/config.yaml</h3>
              <pre className={styles.code}>
                <code>{`crawl:
  allowed_domains: ["example.com"]
  max_depth: 5
  keep_alive: true      # wait for URLs from the dashboard

concurrency:
  global: 50
  per_domain: 2

rate_limit:
  requests_per_second: 1

server:
  addr: ":9090"
  api_token: ""         # or CRAWLER_API_TOKEN`}</code>
              </pre>
              <p className={styles.note}>Every setting can also be overridden with a CRAWLER_* environment variable.</p>
            </div>
          </div>
        </section>
      </main>

      <footer className={styles.footer}>
        <span>{SITE_NAME}</span>
        <a href={REPO_URL}>Source on GitHub</a>
        <Link href="/dashboard">Dashboard</Link>
      </footer>
    </div>
  );
}
