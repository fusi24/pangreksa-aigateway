import "server-only";

import { cookies } from "next/headers";
import { jwtVerify, SignJWT } from "jose";
import type { NextRequest } from "next/server";
import type { ConsolePermission } from "@/types/rbac";

const COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "pangreksa_session";

function getSecret(): Uint8Array {
  const secret = process.env.NEXTAUTH_SECRET;
  if (!secret) throw new Error("NEXTAUTH_SECRET must be set");
  return new TextEncoder().encode(secret);
}

/**
 * Payload stored in the console session JWT.
 * `central_token` is the original JWT from the Central Server,
 * forwarded server-side to API calls — never exposed to the browser.
 */
export interface SessionPayload {
  sub: string;
  email: string;
  org_id: string;
  permissions: ConsolePermission[];
  central_token: string;
  iat?: number;
  exp?: number;
}

/**
 * Signs a new console session JWT and returns the token string.
 *
 * @param payload - Session data to embed in the JWT
 * @returns Signed JWT string (15-minute expiry)
 */
export async function signSession(
  payload: Omit<SessionPayload, "iat" | "exp">
): Promise<string> {
  return new SignJWT(payload as Record<string, unknown>)
    .setProtectedHeader({ alg: "HS256" })
    .setIssuedAt()
    .setExpirationTime("15m")
    .sign(getSecret());
}

/**
 * Reads and verifies the console session from the httpOnly cookie.
 *
 * @returns Parsed SessionPayload, or null if absent/invalid/expired
 */
export async function getSession(): Promise<SessionPayload | null> {
  const cookieStore = await cookies();
  const token = cookieStore.get(COOKIE_NAME)?.value;
  if (!token) return null;

  try {
    const { payload } = await jwtVerify(token, getSecret());
    return payload as unknown as SessionPayload;
  } catch {
    return null;
  }
}

/**
 * Extracts the raw session JWT from a NextRequest cookie.
 * Used in BFF route handlers to forward the token to the Central Server.
 *
 * @param req - Incoming Next.js request
 * @returns Raw JWT string or undefined
 */
export function getSessionJwt(req: NextRequest): string | undefined {
  return req.cookies.get(COOKIE_NAME)?.value;
}

/**
 * Cookie options shared between login and refresh routes.
 */
export function sessionCookieOptions(secure: boolean) {
  return {
    httpOnly: true,
    secure,
    sameSite: "strict" as const,
    maxAge: 15 * 60,
    path: "/",
  };
}

export { COOKIE_NAME };
