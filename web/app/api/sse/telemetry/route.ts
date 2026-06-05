import { NextRequest } from "next/server";
import { getSessionJwt } from "@/lib/auth/session";

/**
 * GET /api/sse/telemetry
 * Proxies the SSE stream from Central Server to the browser.
 * The ADMIN_API_KEY is injected server-side — never exposed to the browser.
 */
export async function GET(req: NextRequest) {
  const jwt = getSessionJwt(req);

  const base = process.env.CENTRAL_SERVER_URL;
  const key = process.env.ADMIN_API_KEY;

  if (!base || !key) {
    return new Response("Server misconfigured", { status: 500 });
  }

  const headers: Record<string, string> = {
    Authorization: `Bearer ${key}`,
    Accept: "text/event-stream",
    "Cache-Control": "no-cache",
  };

  if (jwt) {
    headers["X-User-Token"] = jwt;
  }

  let upstream: Response;
  try {
    upstream = await fetch(`${base}/sse/telemetry`, { headers });
  } catch {
    return new Response('data: {"error":"Upstream unavailable"}\n\n', {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    });
  }

  if (!upstream.ok || !upstream.body) {
    return new Response('data: {"error":"SSE unavailable"}\n\n', {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    });
  }

  return new Response(upstream.body, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
