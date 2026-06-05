import { NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import { validateCsrf } from "@/lib/utils/csrf";
import type { ApiKeyRecord, ApiKeyCreateResponse } from "@/types/api";

/**
 * GET /api/settings/api-keys
 * Lists API keys for the current user.
 */
export async function GET(req: NextRequest) {
  const jwt = getSessionJwt(req);
  try {
    const data = await centralFetch<{ keys: ApiKeyRecord[] }>(
      "/settings/api-keys",
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

/**
 * POST /api/settings/api-keys
 * Creates a new PAT. Returns the token once (not stored in plain text).
 */
export async function POST(req: NextRequest) {
  try {
    validateCsrf(req);
  } catch {
    return NextResponse.json({ error: "CSRF validation failed" }, { status: 403 });
  }

  const session = await getSession();
  if (!session)
    return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const body: unknown = await req.json();
  const jwt = getSessionJwt(req);

  try {
    const data = await centralFetch<ApiKeyCreateResponse>(
      "/settings/api-keys",
      { method: "POST", body: JSON.stringify(body) },
      jwt
    );
    return NextResponse.json(data, { status: 201 });
  } catch (e) {
    if (e instanceof ApiError)
      return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}

/**
 * DELETE /api/settings/api-keys
 * Revokes a key by ID (passed as ?id= query param).
 */
export async function DELETE(req: NextRequest) {
  try {
    validateCsrf(req);
  } catch {
    return NextResponse.json({ error: "CSRF validation failed" }, { status: 403 });
  }

  const id = req.nextUrl.searchParams.get("id");
  if (!id)
    return NextResponse.json({ error: "Missing id parameter" }, { status: 400 });

  const jwt = getSessionJwt(req);

  try {
    await centralFetch(`/settings/api-keys/${id}`, { method: "DELETE" }, jwt);
    return NextResponse.json({ ok: true });
  } catch (e) {
    if (e instanceof ApiError)
      return NextResponse.json({ error: e.body }, { status: e.status });
    return NextResponse.json({ error: "Internal error" }, { status: 500 });
  }
}
