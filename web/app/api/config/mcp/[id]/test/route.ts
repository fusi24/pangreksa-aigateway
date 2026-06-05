import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import { validateCsrf } from "@/lib/utils/csrf";
import type { McpTestResult } from "@/types/api";

/**
 * POST /api/config/mcp/[id]/test
 * Tests MCP server connectivity and returns available tools.
 */
export async function POST(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const { id } = await params;
  const jwt = getSessionJwt(req);

  try {
    const data = await centralFetch<McpTestResult>(
      `/admin/config/mcp/${id}/test`,
      { method: "POST" },
      jwt
    );
    return NextResponse.json(data);
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}
