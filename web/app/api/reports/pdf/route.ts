import { NextRequest, NextResponse } from "next/server";
import { getSession } from "@/lib/auth/session";
import { validateCsrf } from "@/lib/utils/csrf";
import type { ReportParams } from "@/types/api";

/**
 * POST /api/reports/pdf
 * Proxies report parameters to the Central Server PDF generator,
 * streaming the PDF binary response back to the browser.
 */
export async function POST(req: NextRequest) {
  try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const params = (await req.json()) as ReportParams;
  const base = process.env.CENTRAL_SERVER_URL;
  const key = process.env.ADMIN_API_KEY;

  if (!base || !key) return NextResponse.json({ error: "Server misconfigured" }, { status: 500 });

  const dateStr = new Date().toISOString().split("T")[0] ?? "report";
  const filename = `pangreksa-report-${params.type}-${dateStr}.pdf`;

  try {
    const upstream = await fetch(`${base}/admin/reports/pdf`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${key}`,
        "X-User-Token": session.central_token,
      },
      body: JSON.stringify({ ...params, org_id: session.org_id }),
    });

    if (!upstream.ok || !upstream.body) {
      const text = await upstream.text().catch(() => "PDF generation failed");
      return NextResponse.json({ error: text }, { status: upstream.status });
    }

    return new Response(upstream.body, {
      headers: {
        "Content-Type": "application/pdf",
        "Content-Disposition": `attachment; filename="${filename}"`,
      },
    });
  } catch {
    return NextResponse.json({ error: "PDF service unavailable" }, { status: 503 });
  }
}
