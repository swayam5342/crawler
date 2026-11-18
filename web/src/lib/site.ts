// Product naming and links in one place.
export const SITE_NAME = "web_crawler";
export const REPO_URL = "https://github.com/swayam5342/crawler";
export const CLONE_DIR = "crawler";
// The deployed address, for canonical, sitemap and og:image URLs. Set
// NEXT_PUBLIC_SITE_URL when building for a real domain.
export const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL?.replace(/\/+$/, "") ?? "";
