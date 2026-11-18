// Server-side proxy from the dashboard to the Go crawler's /api/. Keeps
// the crawler's API token out of the browser and avoids CORS entirely.
import type { NextRequest } from "next/server";

export const dynamic = "force-dynamic";

const API_URL = (process.env.CRAWLER_API_URL ?? "http://localhost:9090").replace(/\/+$/, "");
const TOKEN = process.env.CRAWLER_API_TOKEN ?? "";
const SAFE_SEGMENT = /^[a-z0-9_-]+$/i;

type Ctx = { params: Promise<{ path: string[] }> };

async function forward(req: NextRequest, ctx: Ctx): Promise<Response> {
  const { path } = await ctx.params;
  if (!path.length || !path.every((p) => SAFE_SEGMENT.test(p))) {
    return Response.json({ error: "unknown endpoint" }, { status: 404 });
  }

  const target = `${API_URL}/api/${path.join("/")}${req.nextUrl.search}`;
  const headers: Record<string, string> = { accept: "application/json" };
  const contentType = req.headers.get("content-type");
  if (contentType) headers["content-type"] = contentType;
  if (TOKEN) headers.authorization = `Bearer ${TOKEN}`;

  const hasBody = req.method !== "GET" && req.method !== "HEAD";
  try {
    const upstream = await fetch(target, {
      method: req.method,
      headers,
      body: hasBody ? await req.text() : undefined,
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
    return new Response(await upstream.text(), {
      status: upstream.status,
      headers: {
        "content-type": upstream.headers.get("content-type") ?? "application/json",
        "cache-control": "no-store",
      },
    });
  } catch {
    return Response.json(
      {
        error: `Can't reach the crawler at ${API_URL}. Start it, or point CRAWLER_API_URL at it.`,
        unreachable: true,
      },
      { status: 502 },
    );
  }
}

export const GET = forward;
export const POST = forward;
export const DELETE = forward;
