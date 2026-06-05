import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import type { ReportData, ReportType, ReportScope } from "@/types/api";

/**
 * GET /api/reports/data
 * Fetches aggregated data for client-side DOCX/XLSX generation.
 * The Central Server may implement a dedicated /admin/reports/data endpoint;
 * this falls back to composing data from existing admin endpoints.
 */
export async function GET(req: NextRequest) {
  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const { searchParams } = req.nextUrl;
  const type = (searchParams.get("type") ?? "usage_summary") as ReportType;
  const from = searchParams.get("from") ?? new Date(Date.now() - 86400_000).toISOString();
  const to = searchParams.get("to") ?? new Date().toISOString();
  const scope = (searchParams.get("scope") ?? "org") as ReportScope;
  const scopeId = searchParams.get("scope_id") ?? undefined;

  const jwt = getSessionJwt(req);

  try {
    const params = new URLSearchParams({
      from,
      to,
      org_id: session.org_id,
      ...(scopeId ? { scope_id: scopeId } : {}),
    });

    // Try the dedicated reports/data endpoint first, fall back to metrics
    let rows: Array<Record<string, string | number | boolean | null>> = [];
    let summary: Record<string, number | string> = {};

    try {
      const data = await centralFetch<ReportData>(
        `/admin/reports/data?type=${type}&${params.toString()}`,
        {},
        jwt
      );
      rows = data.rows;
      summary = data.summary;
    } catch {
      // Fallback: use metrics endpoint for summary
      const metrics = await centralFetch<{ summary: Record<string, number | string> }>(
        `/admin/metrics?${params.toString()}&interval=1h`,
        {},
        jwt
      );
      summary = metrics.summary ?? {};
    }

    const reportData: ReportData = {
      summary,
      rows,
      meta: {
        type,
        from,
        to,
        scope,
        ...(scopeId !== undefined ? { scope_id: scopeId } : {}),
        generated_at: new Date().toISOString(),
        generated_by: session.email,
      },
    };

    return NextResponse.json(reportData);
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}
