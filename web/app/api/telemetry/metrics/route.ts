import { NextRequest, NextResponse } from "next/server";
import { getSession } from "@/lib/auth/session";

/**
 * GET /api/telemetry/metrics
 * Proxies PromQL queries to the Prometheus HTTP API.
 * Never exposes Prometheus directly to the browser.
 */
export async function GET(req: NextRequest) {
  const session = await getSession();
  if (!session)
    return NextResponse.json({ error: "Unauthenticated" }, { status: 401 });

  const prometheusUrl = process.env.PROMETHEUS_URL;
  if (!prometheusUrl) {
    return NextResponse.json({ error: "Prometheus not configured" }, { status: 503 });
  }

  const { searchParams } = req.nextUrl;
  const params = new URLSearchParams({
    query: searchParams.get("query") ?? "",
    start: searchParams.get("start") ?? "",
    end: searchParams.get("end") ?? "",
    step: searchParams.get("step") ?? "60",
  });

  try {
    const res = await fetch(
      `${prometheusUrl}/api/v1/query_range?${params.toString()}`,
      { cache: "no-store" }
    );
    const data: unknown = await res.json();
    return NextResponse.json(data);
  } catch {
    return NextResponse.json({ error: "Prometheus unavailable" }, { status: 503 });
  }
}
