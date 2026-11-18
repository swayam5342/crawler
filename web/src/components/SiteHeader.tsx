import { asset, REPO_URL, SITE_NAME } from "@/lib/site";

const LINKS = [
  { id: "pipeline", label: "Pipeline" },
  { id: "dashboard", label: "Dashboard" },
  { id: "failures", label: "Failures" },
];

/** The site header. `home` is the path prefix for section links: "" on the
 *  home page itself, "/" anywhere else so the anchors still land. */
export function SiteHeader({ home = "" }: { home?: "" | "/" }) {
  const links = (
    <>
      {LINKS.map((l) => (
        <a key={l.id} href={home ? asset(`/#${l.id}`) : `#${l.id}`} className="line">
          {l.label}
        </a>
      ))}
      <a href={REPO_URL} className="line">
        Source
      </a>
    </>
  );
  return (
    <>
      <a className="skip" href="#main">
        Skip to content
      </a>
      <header className="nav">
        <div className="container nav-row">
          <a href={home ? asset("/") : "#main"} className="wordmark line" aria-label={`${SITE_NAME}, home`}>
            {SITE_NAME}
          </a>
          <nav aria-label="Site" className="nav-links">
            {links}
          </nav>
          <details className="menu">
            <summary className="btn btn--quiet btn--sm">Menu</summary>
            <nav aria-label="Site" className="menu-list">
              {links}
            </nav>
          </details>
          <a href={home ? asset("/#run") : "#run"} className="btn btn--quiet btn--sm nav-action">
            Run it
          </a>
        </div>
      </header>
    </>
  );
}
