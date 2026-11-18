import { NextResponse, type NextRequest } from "next/server";

// Optional HTTP basic auth in front of the dashboard and its API proxy.
// Enabled only when DASHBOARD_PASSWORD is set; the showcase pages stay
// public either way.
export function proxy(request: NextRequest) {
  const password = process.env.DASHBOARD_PASSWORD;
  if (!password) return NextResponse.next();

  const header = request.headers.get("authorization") ?? "";
  if (header.startsWith("Basic ")) {
    try {
      const [user, pass] = atob(header.slice(6)).split(/:(.*)/s);
      if (user === "admin" && timingSafeEqual(pass ?? "", password)) {
        return NextResponse.next();
      }
    } catch {
      // Malformed header: fall through to the challenge.
    }
  }

  return new NextResponse("Sign in to use the crawler dashboard.", {
    status: 401,
    headers: { "WWW-Authenticate": 'Basic realm="crawler dashboard", charset="UTF-8"' },
  });
}

function timingSafeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

export const config = {
  matcher: ["/dashboard/:path*", "/api/crawler/:path*"],
};
