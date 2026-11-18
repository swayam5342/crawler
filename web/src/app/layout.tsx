import type { Metadata, Viewport } from "next";
import { Chakra_Petch, IBM_Plex_Mono, IBM_Plex_Sans } from "next/font/google";
import { SITE_NAME, SITE_URL } from "@/lib/site";
import "./site.css";

// Self-hosted at build time as subset woff2, so the page never depends on
// a third-party font request at runtime.
const display = Chakra_Petch({ subsets: ["latin"], weight: "600", variable: "--nf-display", display: "swap" });
const body = IBM_Plex_Sans({ subsets: ["latin"], weight: ["400", "500"], variable: "--nf-body", display: "swap" });
const mono = IBM_Plex_Mono({ subsets: ["latin"], weight: "500", variable: "--nf-mono", display: "swap" });

const TITLE = `${SITE_NAME}: a self-hosted web crawler that backs off`;
const DESCRIPTION =
  "A self-hosted web crawler written in Go. It reads robots.txt, paces each domain, retries with backoff and resumes after a crash, with a dashboard to steer it.";

export const metadata: Metadata = {
  // Absolute URLs (canonical, og:image) need the deployed address; set
  // NEXT_PUBLIC_SITE_URL at build time. Only the origin goes here: Next adds
  // the basePath to file-based metadata (og:image, icon) itself.
  metadataBase: SITE_URL ? new URL(new URL(SITE_URL).origin) : undefined,
  title: TITLE,
  description: DESCRIPTION,
  alternates: SITE_URL ? { canonical: `${SITE_URL}/` } : undefined,
  openGraph: { type: "website", title: TITLE, description: DESCRIPTION, siteName: SITE_NAME },
  twitter: { card: "summary_large_image", title: TITLE, description: DESCRIPTION },
  other: { "page-type": "software", "site-kind": "software" },
};

export const viewport: Viewport = {
  themeColor: "#1d3a6b",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" data-type="software" className={`${display.variable} ${body.variable} ${mono.variable}`}>
      <body>{children}</body>
    </html>
  );
}
