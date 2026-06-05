import { NextRequest } from "next/server";
import { makeConfigHandlers } from "@/lib/api/config-route-helpers";

const handlers = makeConfigHandlers("/admin/config/guardrails");
type Ctx = { params: Promise<{ slug?: string[] }> };

export async function GET(req: NextRequest, ctx: Ctx) {
  const { slug } = await ctx.params;
  return handlers.GET(req, slug);
}
export async function POST(req: NextRequest, ctx: Ctx) {
  const { slug } = await ctx.params;
  return handlers.POST(req, slug);
}
export async function PUT(req: NextRequest, ctx: Ctx) {
  const { slug } = await ctx.params;
  return handlers.PUT(req, slug);
}
export async function DELETE(req: NextRequest, ctx: Ctx) {
  const { slug } = await ctx.params;
  return handlers.DELETE(req, slug);
}
