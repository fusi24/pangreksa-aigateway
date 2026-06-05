import { NextResponse } from "next/server";

/**
 * GET /api/health
 * Used by Docker healthcheck and load balancer probes.
 */
export function GET() {
  return NextResponse.json({ status: "ok", ts: new Date().toISOString() });
}
