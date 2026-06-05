import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import { validateCsrf } from "@/lib/utils/csrf";
import type { EntitlementRecord } from "@/types/api";

/**
 * GET /api/config/entitlements?user_id=...
 * POST /api/config/entitlements — update entitlement + invalidate
 */
export async function GET(req: NextRequest) {
  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const userId = req.nextUrl.searchParams.get("user_id");
  if (!userId) return NextResponse.json({ error: "user_id required" }, { status: 400 });

  const jwt = getSessionJwt(req);
  try {
    const data = await centralFetch<EntitlementRecord>(`/admin/entitlement/${userId}`, {}, jwt);
    return NextResponse.json(data);
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}

export async function POST(req: NextRequest) {
  try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const body: unknown = await req.json();
  const jwt = getSessionJwt(req);

  try {
    const data = await centralFetch<EntitlementRecord>(
      "/admin/entitlement",
      { method: "POST", body: JSON.stringify(body) },
      jwt
    );
    await centralFetch("/admin/invalidate", { method: "POST" }, jwt).catch(() => {});
    return NextResponse.json(data);
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}
