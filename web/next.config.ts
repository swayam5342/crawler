import type { NextConfig } from "next";

// Two build targets:
// - default: a self-contained server bundle for the Docker image;
// - STATIC_EXPORT=1: plain files in out/ for GitHub Pages, served under
//   NEXT_PUBLIC_BASE_PATH (e.g. "/crawler" for a project site).
const staticExport = !!process.env.STATIC_EXPORT;
const basePath = process.env.NEXT_PUBLIC_BASE_PATH?.replace(/\/+$/, "") || undefined;

const nextConfig: NextConfig = {
  output: staticExport ? "export" : "standalone",
  basePath,
  trailingSlash: staticExport,
  images: { unoptimized: true },
  poweredByHeader: false,
};

export default nextConfig;
