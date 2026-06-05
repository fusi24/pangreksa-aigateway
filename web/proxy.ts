import { NextRequest, NextResponse } from "next/server";
import { jwtVerify } from "jose";
import { ROUTE_PERMISSION_MAP } from "@/types/rbac";

const COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "pangreksa_session";

function getSecret(): Uint8Array {
  const secret = process.env.NEXTAUTH_SECRET ?? "";
  return new TextEncoder().encode(secret);
}

/**
 * RBAC route guard — runs on every request before any page renders.
 * - Redirects to /login if no valid session cookie
 * - Rewrites to /403 if session lacks the required permission
 */
export async function proxy(req: NextRequest) {
  const { pathname } = req.nextUrl;

  // Pass through public routes and static assets
  if (
    pathname.startsWith("/login") ||
    pathname.startsWith("/api/auth") ||
    pathname.startsWith("/403") ||
    pathname.startsWith("/_next") ||
    pathname === "/favicon.ico"
  ) {
    return NextResponse.next();
  }

  const token = req.cookies.get(COOKIE_NAME)?.value;

  if (!token) {
    return NextResponse.redirect(new URL("/login", req.url));
  }

  let permissions: string[] = [];

  try {
    const { payload } = await jwtVerify(token, getSecret());
    permissions = (payload["permissions"] as string[] | undefined) ?? [];
  } catch {
    const res = NextResponse.redirect(new URL("/login", req.url));
    res.cookies.delete(COOKIE_NAME);
    return res;
  }

  for (const [prefix, required] of ROUTE_PERMISSION_MAP) {
    if (pathname.startsWith(prefix) && !permissions.includes(required)) {
      return NextResponse.rewrite(new URL("/403", req.url));
    }
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
