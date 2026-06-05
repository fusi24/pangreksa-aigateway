import type { NextRequest } from "next/server";

/**
 * Validates that the request Origin header matches the Host header.
 * Protects all mutating BFF routes against CSRF attacks.
 * Works in conjunction with SameSite=Strict cookie.
 *
 * @param req - Incoming NextRequest
 * @throws {Error} If Origin does not match Host
 */
export function validateCsrf(req: NextRequest): void {
  const origin = req.headers.get("origin");
  const host = req.headers.get("host");

  if (!origin || !host) {
    throw new Error("Missing Origin or Host header");
  }

  const originHost = new URL(origin).host;
  if (originHost !== host) {
    throw new Error(`CSRF validation failed: origin ${originHost} !== host ${host}`);
  }
}
