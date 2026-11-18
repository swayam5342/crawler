import type { Metadata } from "next";
import { SiteHeader } from "@/components/SiteHeader";
import { asset, REPO_URL, SITE_NAME } from "@/lib/site";

export const metadata: Metadata = {
  title: `Page not found | ${SITE_NAME}`,
  robots: { index: false },
};

export default function NotFound() {
  return (
    <>
      <SiteHeader home="/" />
      <main id="main" className="lost">
      <div className="container lost-grid">
        <svg className="lost-mark" viewBox="0 0 120 120" role="img" aria-label="A crawl mark for a failed URL: a ring with a cross">
          <circle cx="60" cy="60" r="44" className="pl-hollow" />
          <path d="M33 33L87 87M87 33L33 87" className="pl-stroke" />
        </svg>
        <div>
          <h1>No page at this address</h1>
          <p className="lead">The crawler would log this one as a 404 and move on.</p>
          <div className="actions">
            <a href={asset("/")} className="btn btn--primary">
              Back to the home page
            </a>
            <a href={REPO_URL} className="btn btn--quiet">
              Read the source
            </a>
          </div>
        </div>
      </div>
      </main>
    </>
  );
}
