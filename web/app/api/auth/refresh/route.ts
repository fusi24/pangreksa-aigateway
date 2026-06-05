import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, signSession, sessionCookieOptions, COOKIE_NAME, getSession } from "@/lib/auth/session";
import type { ConsolePermission } from "@/types/rbac";

interface RefreshResponse {
  access_token: string;
  expires_in: number;
  permissions?: string[];
}

/**
 * POST /api/auth/refresh
 * Obtains a new Central Server access token using the refresh cookie,
 * then re-signs the console session JWT with a fresh 15-minute expiry.
 */
export async function POST(req: NextRequest) {
  const currentJwt = getSessionJwt(req);
  const session = await getSession();

  if (!session || !currentJwt) {
    return NextResponse.json({ error: "No active session" }, { status: 401 });
  }

  try {
    const refreshed = await centralFetch<RefreshResponse>(
      "/auth/refresh",
      { method: "POST" },
      currentJwt
    );

    const newJwt = await signSession({
      sub: session.sub,
      email: session.email,
      org_id: session.org_id,
      permissions: (refreshed.permissions ?? session.permissions) as ConsolePermission[],
      central_token: refreshed.access_token,
    });

    const res = NextResponse.json({ ok: true });
    res.cookies.set(
      COOKIE_NAME,
      newJwt,
      sessionCookieOptions(process.env.NODE_ENV === "production")
    );

    return res;
  } catch (e) {
    if (e instanceof ApiError) {
      const res = NextResponse.json({ error: "Session expired" }, { status: 401 });
      res.cookies.delete(COOKIE_NAME);
      return res;
    }
    return NextResponse.json({ error: "Refresh failed" }, { status: 500 });
  }
}
