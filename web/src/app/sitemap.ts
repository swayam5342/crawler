import type { MetadataRoute } from "next";

export const dynamic = "force-static";
import { SITE_URL } from "@/lib/site";

export default function sitemap(): MetadataRoute.Sitemap {
  // A sitemap needs absolute URLs; without a known address, list the root
  // relative so the file is still valid for local runs.
  return [{ url: SITE_URL ? `${SITE_URL}/` : "/", changeFrequency: "monthly", priority: 1 }];
}
