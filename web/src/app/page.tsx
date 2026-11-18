import { CopyButton } from "@/components/CopyButton";
import { CrawlGraph } from "@/components/CrawlGraph";
import { DashboardRun, type Screen } from "@/components/DashboardRun";
import { SiteHeader } from "@/components/SiteHeader";
import { asset, CLONE_DIR, REPO_URL, SITE_NAME } from "@/lib/site";

// What happens to each URL, in order: the stage, what it does, and the
// package that does it.
const PIPELINE = [
  { step: "Normalize", does: "Fragments stripped, query sorted", pkg: "internal/urlutil" },
  { step: "Scope", does: "Allowed domains and their subdomains only", pkg: "internal/frontier" },
  { step: "Robots", does: "robots.txt fetched, cached and obeyed", pkg: "internal/robots" },
  { step: "Pace", does: "Token bucket per domain; Crawl-delay wins", pkg: "internal/ratelimit" },
  { step: "Fetch", does: "Timeouts per layer, retries, a circuit breaker", pkg: "internal/fetcher" },
  { step: "Extract", does: "Links, title and language; nofollow honored", pkg: "internal/extractor" },
  { step: "Store", does: "Metadata to Postgres, the body to disk or S3", pkg: "internal/storage" },
];

const SCREENS: Screen[] = [
  {
    id: "overview",
    title: "Watch it crawl",
    line: "Pages, errors and robots.txt blocks per domain, refreshed every two seconds.",
    src: asset("/screens/overview.png"),
    alt: "Dashboard overview: pages crawled, errors, blocked URLs and a table of recent activity.",
  },
  {
    id: "pages",
    title: "Search what it stored",
    line: "Every stored page with its status code, by URL, title or domain.",
    src: asset("/screens/pages.png"),
    alt: "Crawled pages: a searchable list of page titles, URLs, status codes and fetch times.",
  },
  {
    id: "failures",
    title: "Retry what failed",
    line: "A URL that ran out of tries waits here with the error that stopped it.",
    src: asset("/screens/failures.png"),
    alt: "Failed URLs: one URL with its error, attempt count, and Retry and Discard buttons.",
  },
  {
    id: "settings",
    title: "Change scope, or start over",
    line: "Edit the allowed domains; clear the queue, failures or stored pages.",
    src: asset("/screens/settings.png"),
    alt: "Settings: the allowed domains list, reset options and the running configuration.",
  },
];

const FAILURES = [
  { when: "503s, timeouts, connection resets", then: "Retried with exponential backoff and jitter." },
  { when: "A domain that keeps failing", then: "After 5 failures in a row it gets nothing for a minute." },
  { when: "A URL out of retries", then: "Kept with its error, one click from a retry." },
  { when: "Huge or broken pages", then: "Bodies stop at 10 MiB; bad HTML and odd encodings still parse." },
  { when: "A crash or a SIGTERM", then: "The queue is saved every 10 seconds; the next start resumes." },
];

// Default retry timing (500 ms base, doubling): tries at 0, 0.5, 1.5, 3.5 s.
const TRIES = [
  { label: "Try 1", at: "0 s", pos: 0 },
  { label: "Try 2", at: "0.5 s", pos: 0.5 / 3.5 },
  { label: "Try 3", at: "1.5 s", pos: 1.5 / 3.5 },
  { label: "Try 4", at: "3.5 s", pos: 1 },
];

const RUN = `git clone ${REPO_URL}.git
cd ${CLONE_DIR}
docker compose up --build`;

const CONFIG = `crawl:
  # an empty list allows every domain
  allowed_domains: []
  max_depth: 5
  keep_alive: true

rate_limit:
  requests_per_second: 1

retry:
  max_attempts: 4
  base_backoff: 500ms`;

export default function Home() {
  return (
    <>
      <SiteHeader />

      <main id="main">
        <section className="hero" aria-labelledby="hero-title">
          <div className="container hero-grid">
            <div className="hero-text">
              <h1 id="hero-title">A web crawler that backs off</h1>
              <p className="lead">It reads robots.txt, paces each domain and resumes after a crash.</p>
              <div className="actions">
                <a href="#run" className="btn btn--primary">
                  Run it locally
                </a>
                <a href={REPO_URL} className="btn btn--quiet">
                  Read the source
                </a>
              </div>
              <p className="hero-fact">
                Written in Go. One request per second per domain by default.
              </p>
            </div>
            <div className="hero-plate">
              <CrawlGraph />
            </div>
          </div>
        </section>

        <section id="pipeline" className="section band" aria-labelledby="pipeline-title">
          <div className="container">
            <div className="head head--split">
              <h2 id="pipeline-title">What happens to each URL</h2>
              <p className="lead">Any of these seven stages can stop a URL.</p>
            </div>
            <div className="register-wrap">
              <table className="register">
                <thead>
                  <tr>
                    <th scope="col">Stage</th>
                    <th scope="col">Step</th>
                    <th scope="col">What it does</th>
                    <th scope="col">Code</th>
                  </tr>
                </thead>
                <tbody>
                  {PIPELINE.map((row, i) => (
                    <tr key={row.step}>
                      <td className="num mono">{i + 1}</td>
                      <th scope="row">{row.step}</th>
                      <td>{row.does}</td>
                      <td className="mono">{row.pkg}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <section id="dashboard" className="section" aria-labelledby="dashboard-title">
          <div className="container">
            <div className="head">
              <h2 id="dashboard-title">Steer it from the dashboard</h2>
              <p className="lead">Run it as a service and it waits for new URLs instead of exiting.</p>
            </div>
            <DashboardRun screens={SCREENS} />
          </div>
        </section>

        <section id="failures" className="section section--rule" aria-labelledby="failures-title">
          <div className="container failures">
            <h2 id="failures-title">When a site misbehaves</h2>
            <dl className="rows">
              {FAILURES.map((f) => (
                <div key={f.when} className="row">
                  <dt>{f.when}</dt>
                  <dd>{f.then}</dd>
                </div>
              ))}
            </dl>
          </div>
        </section>

        <section className="section band band--deep" aria-labelledby="backoff-title">
          <div className="container">
            <div className="head head--split">
              <h2 id="backoff-title">Four tries in about 3.5 seconds</h2>
              <p className="lead">Each wait doubles, give or take 30%, and never passes 30 s.</p>
            </div>
            <div className="timeline" role="img" aria-label="Retry timeline with the default settings: try 1 at 0 seconds, try 2 at 0.5 seconds, try 3 at 1.5 seconds, try 4 at 3.5 seconds.">
              <div className="dim" aria-hidden="true">
                <span className="dim-line" />
                {TRIES.map((t) => (
                  <span key={t.label} className="tick" style={{ "--pos": t.pos } as React.CSSProperties}>
                    <span className="tick-at mono">{t.at}</span>
                    <span className="tick-label">{t.label}</span>
                  </span>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section id="run" className="section" aria-labelledby="run-title">
          <div className="container run-close">
            <div className="head">
              <h2 id="run-title">Run it on your machine</h2>
              <p className="lead">Docker starts Postgres, the crawler and the dashboard on localhost:8067.</p>
            </div>
            <div className="code-pair">
              <div className="code">
                <div className="code-head">
                  <span className="mono">terminal</span>
                  <CopyButton text={RUN} label="Copy the run commands" />
                </div>
                <pre>
                  <code>{RUN}</code>
                </pre>
              </div>
              <div className="code">
                <div className="code-head">
                  <span className="mono">configs/config.yaml</span>
                </div>
                <pre>
                  <code>{CONFIG}</code>
                </pre>
              </div>
            </div>
          </div>
        </section>
      </main>

      <footer className="footer">
        <div className="container footer-row">
          <p>
            <span className="wordmark">{SITE_NAME}</span>, a web crawler written in Go.
          </p>
          <a href={REPO_URL} className="line line--rest">Source on GitHub</a>
        </div>
      </footer>
    </>
  );
}
