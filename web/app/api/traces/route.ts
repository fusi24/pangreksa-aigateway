import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import type { TracesResponse } from "@/types/api";

/**
 * GET /api/traces
 * Proxies to Central Server GET /admin/traces.
 */
export async function GET(req: NextRequest) {
  const session = await getSession();
  if (!session)
    return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const { searchParams } = req.nextUrl;
  const params = new URLSearchParams(searchParams);
  params.set("org_id", session.org_id);

  const jwt = getSessionJwt(req);

  try {
    const data = await centralFetch<TracesResponse>(
      `/admin/traces?${params.toString()}`,
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
