import { NextRequest, NextResponse } from "next/server";
import { centralFetch } from "@/lib/api/central-server";
import { getSessionJwt, COOKIE_NAME } from "@/lib/auth/session";

/**
 * POST /api/auth/logout
 * Revokes the Central Server session and clears the console session cookie.
 */
export async function POST(req: NextRequest) {
  const jwt = getSessionJwt(req);

  if (jwt) {
    try {
      await centralFetch("/auth/logout", { method: "POST" }, jwt);
    } catch {
      // Best-effort logout — clear cookie regardless
    }
  }

  const res = NextResponse.json({ ok: true });
  res.cookies.delete(COOKIE_NAME);
  return res;
}
