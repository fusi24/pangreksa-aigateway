import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import { validateCsrf } from "@/lib/utils/csrf";
import type { PromptsResponse, PromptVersion } from "@/types/api";

type RouteParams = { slug?: string[] };

/**
 * Prompt Registry BFF routes.
 * GET /api/config/prompts                  → list all repos
 * GET /api/config/prompts/:repo/:name      → get prompt by FQN
 * POST /api/config/prompts/:repo/:name     → create new version
 * PUT /api/config/prompts/:repo/:name/active → set active version
 */
export async function GET(
  req: NextRequest,
  { params }: { params: Promise<RouteParams> }
) {
  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const { slug } = await params;
  const jwt = getSessionJwt(req);
  const path = slug ? `/admin/config/prompts/${slug.join("/")}` : `/admin/config/prompts?org_id=${session.org_id}`;

  try {
    const data = await centralFetch<PromptsResponse | PromptVersion>(path, {}, jwt);
    return NextResponse.json(data);
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}

export async function POST(
  req: NextRequest,
  { params }: { params: Promise<RouteParams> }
) {
  try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const { slug } = await params;
  if (!slug || slug.length < 2) return NextResponse.json({ error: "Missing repo/name" }, { status: 400 });

  const body: unknown = await req.json();
  const jwt = getSessionJwt(req);

  try {
    const data = await centralFetch<{ fqn: string; version: number }>(
      `/admin/config/prompts/${slug.join("/")}`,
      { method: "POST", body: JSON.stringify(body) },
      jwt
    );
    // Trigger daemon invalidation
    await centralFetch("/admin/invalidate", { method: "POST" }, jwt).catch(() => {});
    return NextResponse.json(data, { status: 201 });
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}

export async function PUT(
  req: NextRequest,
  { params }: { params: Promise<RouteParams> }
) {
  try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

  const session = await getSession();
  if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const { slug } = await params;
  const body: unknown = await req.json();
  const jwt = getSessionJwt(req);

  try {
    const data = await centralFetch<unknown>(
      `/admin/config/prompts/${(slug ?? []).join("/")}`,
      { method: "PUT", body: JSON.stringify(body) },
      jwt
    );
    await centralFetch("/admin/invalidate", { method: "POST" }, jwt).catch(() => {});
    return NextResponse.json(data);
  } catch (e) {
    if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}
