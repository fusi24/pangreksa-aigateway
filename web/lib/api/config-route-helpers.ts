import "server-only";

import { type NextRequest, NextResponse } from "next/server";
import { centralFetch, ApiError } from "./central-server";
import { getSessionJwt, getSession } from "@/lib/auth/session";
import { validateCsrf } from "@/lib/utils/csrf";

/**
 * Generic config CRUD route handler factory.
 * Used by all /api/config/{entity} routes to avoid repeating the same boilerplate.
 */
export function makeConfigHandlers(basePath: string) {
  async function GET(
    req: NextRequest,
    slug?: string[]
  ): Promise<NextResponse> {
    const session = await getSession();
    if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

    const jwt = getSessionJwt(req);
    const path = slug?.length
      ? `${basePath}/${slug.join("/")}`
      : `${basePath}?org_id=${session.org_id}`;

    try {
      const data = await centralFetch<unknown>(path, {}, jwt);
      return NextResponse.json(data);
    } catch (e) {
      if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
      return NextResponse.json({ error: "Internal error" }, { status: 500 });
    }
  }

  async function POST(req: NextRequest, slug?: string[]): Promise<NextResponse> {
    try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

    const session = await getSession();
    if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

    const body: unknown = await req.json();
    const jwt = getSessionJwt(req);
    const path = slug?.length ? `${basePath}/${slug.join("/")}` : basePath;

    try {
      const data = await centralFetch<unknown>(path, { method: "POST", body: JSON.stringify(body) }, jwt);
      await centralFetch("/admin/invalidate", { method: "POST" }, jwt).catch(() => {});
      return NextResponse.json(data, { status: 201 });
    } catch (e) {
      if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
      return NextResponse.json({ error: "Internal error" }, { status: 500 });
    }
  }

  async function PUT(req: NextRequest, slug?: string[]): Promise<NextResponse> {
    try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

    const session = await getSession();
    if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

    const body: unknown = await req.json();
    const jwt = getSessionJwt(req);
    const path = slug?.length ? `${basePath}/${slug.join("/")}` : basePath;

    try {
      const data = await centralFetch<unknown>(path, { method: "PUT", body: JSON.stringify(body) }, jwt);
      await centralFetch("/admin/invalidate", { method: "POST" }, jwt).catch(() => {});
      return NextResponse.json(data);
    } catch (e) {
      if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
      return NextResponse.json({ error: "Internal error" }, { status: 500 });
    }
  }

  async function DELETE(req: NextRequest, slug?: string[]): Promise<NextResponse> {
    try { validateCsrf(req); } catch { return NextResponse.json({ error: "CSRF" }, { status: 403 }); }

    const session = await getSession();
    if (!session) return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

    const jwt = getSessionJwt(req);
    const path = slug?.length ? `${basePath}/${slug.join("/")}` : basePath;

    try {
      await centralFetch<unknown>(path, { method: "DELETE" }, jwt);
      await centralFetch("/admin/invalidate", { method: "POST" }, jwt).catch(() => {});
      return NextResponse.json({ ok: true });
    } catch (e) {
      if (e instanceof ApiError) return NextResponse.json({ error: e.body }, { status: e.status });
      return NextResponse.json({ error: "Internal error" }, { status: 500 });
    }
  }

  return { GET, POST, PUT, DELETE };
}
