import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import type { BudgetSummaryResponse } from "@/types/api";

/**
 * GET /api/budget
 * Proxies to Central Server GET /admin/budget/summary.
 */
export async function GET(req: NextRequest) {
  const session = await getSession();
  if (!session)
    return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const jwt = getSessionJwt(req);

  try {
    const data = await centralFetch<BudgetSummaryResponse>(
      `/admin/budget/summary?org_id=${session.org_id}`,
      {},
      jwt
    );
    return NextResponse.json(data);
  } catch (e) {
    if (e instanceof ApiError)
      return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}
