import { notFound } from "next/navigation";
import { centralFetch, ApiError } from "@/lib/api/central-server";
import { getSession } from "@/lib/auth/session";
import { RequestDetailView } from "./RequestDetailView";
import type { RequestDetail } from "@/types/api";

/**
 * Request Drill-Down page — RSC that fetches data server-side
 * and passes it to the RequestDetailView client component.
 */
export default async function RequestDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const session = await getSession();
  if (!session) return notFound();

  let detail: RequestDetail;
  try {
    detail = await centralFetch<RequestDetail>(`/admin/request/${id}`, {}, session.central_token);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return notFound();
    throw e;
  }

  return (
    <RequestDetailView
      detail={detail}
      jaegerBaseUrl={process.env.NEXT_PUBLIC_JAEGER_EXTERNAL_URL ?? ""}
    />
  );
}
