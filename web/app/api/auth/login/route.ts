import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { decodeJwt } from "jose";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { signSession, sessionCookieOptions, COOKIE_NAME } from "@/lib/auth/session";
import type { ConsolePermission } from "@/types/rbac";

const loginBodySchema = z.object({
  email: z.string().email(),
  password: z.string().min(1),
});

/** Shape returned by the Central Server /auth/login endpoint. */
interface CentralLoginResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
}

/**
 * Payload embedded in the Central Server JWT.
 * Field names match what the Go server actually encodes (e.g. "perms" not "permissions").
 */
interface CentralJwtPayload {
  user_id: string;
  org_id: string;
  email: string;
  /** Central Server calls this field "perms", not "permissions". */
  perms?: string[];
}

/** All console permissions — granted when the user has no perms set (dev fallback). */
const ALL_CONSOLE_PERMISSIONS: ConsolePermission[] = [
  "console.observability.read",
  "console.monitor.read",
  "console.telemetry.read",
  "gateway.prompt_registry.read",
  "gateway.prompt_registry.write",
  "console.reports.read",
  "console.reports.generate",
  "console.admin.read",
  "console.admin.write",
];

/**
 * POST /api/auth/login
 * Exchanges email/password for a Central Server JWT, then creates a console
 * session JWT and sets it as an httpOnly cookie.
 *
 * User data (user_id, org_id, email, perms) is decoded from the Central Server
 * JWT — the response body only contains the token itself.
 */
export async function POST(req: NextRequest) {
  const body: unknown = await req.json();
  const parsed = loginBodySchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json({ error: "Invalid request body" }, { status: 400 });
  }

  try {
    const central = await centralFetch<CentralLoginResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify(parsed.data),
    });

    // Decode the Central Server JWT to extract user metadata.
    // jose's decodeJwt does NOT verify the signature — we trust the Central
    // Server issued it (it was just returned over HTTPS from a successful 200).
    const claims = decodeJwt(central.access_token) as unknown as CentralJwtPayload;

    const centralPerms: string[] = claims.perms ?? [];

    // In development, grant all console permissions when the user has none.
    // In production this should be an explicit assignment in the Central Server.
    const permissions: ConsolePermission[] =
      centralPerms.length > 0
        ? (centralPerms as ConsolePermission[])
        : process.env.NODE_ENV !== "production"
          ? ALL_CONSOLE_PERMISSIONS
          : [];

    const consoleJwt = await signSession({
      sub: claims.user_id,
      email: claims.email,
      org_id: claims.org_id,
      permissions,
      central_token: central.access_token,
    });

    const res = NextResponse.json({ ok: true });
    res.cookies.set(
      COOKIE_NAME,
      consoleJwt,
      sessionCookieOptions(process.env.NODE_ENV === "production")
    );

    return res;
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) {
      return NextResponse.json(
        { error: "Invalid email or password" },
        { status: 401 }
      );
    }
    return NextResponse.json({ error: "Login failed" }, { status: 500 });
  }
}
