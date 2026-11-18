// Product naming and links in one place.
export const SITE_NAME = "web_crawler";
export const REPO_URL = "https://github.com/swayam5342/crawler";
export const CLONE_DIR = "crawler";
// The deployed address, for canonical, sitemap and og:image URLs. Set
// NEXT_PUBLIC_SITE_URL when building for a real domain.
export const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL?.replace(/\/+$/, "") ?? "";
// The path the site is served under ("/crawler" on GitHub Pages, "" locally).
export const BASE_PATH = process.env.NEXT_PUBLIC_BASE_PATH?.replace(/\/+$/, "") ?? "";

/** Prefixes a public file path with the base path. Plain <img> and <a>
 *  tags don't get Next's basePath automatically. */
export const asset = (path: string) => `${BASE_PATH}${path}`;
