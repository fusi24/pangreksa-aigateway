import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import { validateCsrf } from "@/lib/utils/csrf";

/**
 * POST /api/admin/invalidate
 * Triggers Redis pub/sub invalidation on the Central Server,
 * causing all daemons to refresh their config caches.
 */
export async function POST(req: NextRequest) {
  try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const jwt = getSessionJwt(req);

  try {
    await centralFetch("/admin/invalidate", { method: "POST" }, jwt);
    return NextResponse.json({ ok: true });
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}
