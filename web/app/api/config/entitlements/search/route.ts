import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";

interface UserSearchResult {
  user_id: string;
  email: string;
  name: string;
}

/**
 * GET /api/config/entitlements/search?q=email_or_id
 * Searches users for the entitlement editor.
 */
export async function GET(req: NextRequest) {
  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const q = req.nextUrl.searchParams.get("q") ?? "";
  const jwt = getSessionJwt(req);

  try {
    const data = await centralFetch<{ users: UserSearchResult[] }>(
      `/admin/users/search?q=${encodeURIComponent(q)}&org_id=${session.org_id}`,
      {},
      jwt
    );
    return NextResponse.json(data);
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}
