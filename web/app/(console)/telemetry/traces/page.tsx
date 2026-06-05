"use client";

import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  Table,
  TableHead,
  TableRow,
  TableHeader,
  TableBody,
  TableCell,
  TableContainer,
  Tag,
  Link,
  InlineNotification,
  SkeletonText,
} from "@carbon/react";
import { fmtTs, fmtMs, truncate } from "@/lib/utils/format";
import type { TracesResponse, TraceRecord } from "@/types/api";
import { useUIStore } from "@/store/ui";
import { formatISO, subHours } from "date-fns";

const HEADERS = [
  { key: "trace_id", header: "Trace ID" },
  { key: "request_id", header: "Request ID" },
  { key: "root_operation", header: "Operation" },
  { key: "root_latency_ms", header: "Duration" },
  { key: "span_count", header: "Spans" },
  { key: "status", header: "Status" },
  { key: "created_at", header: "Time" },
  { key: "jaeger", header: "" },
];

/**
 * Trace list page — searchable distributed trace metadata with Jaeger deep-links.
 */
export default function TracesPage() {
  const { timeRange } = useUIStore();
  const hours: Record<string, number> = { "1h": 1, "6h": 6, "24h": 24, "7d": 168, "30d": 720 };
  const from = formatISO(subHours(new Date(), hours[timeRange] ?? 24));
  const to = formatISO(new Date());

  const { data, isLoading, error } = useQuery<TracesResponse>({
    queryKey: ["traces", from, to],
    queryFn: async () => {
      const res = await fetch(`/api/traces?from=${from}&to=${to}&limit=50`);
      if (!res.ok) throw new Error("Failed to fetch traces");
      return res.json() as Promise<TracesResponse>;
    },
    refetchInterval: 30_000,
  });

  if (error) {
    return <InlineNotification kind="error" title="Failed to load traces" subtitle={(error as Error).message} />;
  }

  const traces = data?.traces ?? [];

  const rows = traces.map((t: TraceRecord) => ({
    id: t.trace_id,
    trace_id: truncate(t.trace_id, 16),
    request_id: truncate(t.request_id, 16),
    root_operation: t.root_operation,
    root_latency_ms: fmtMs(t.root_latency_ms),
    span_count: String(t.span_count),
    status: t.status,
    created_at: fmtTs(t.created_at),
    jaeger: t.jaeger_url,
  }));

  return (
    <div>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>Trace Explorer</h1>

      {isLoading ? (
        <SkeletonText paragraph />
      ) : (
        <DataTable rows={rows} headers={HEADERS} size="sm">
          {({ rows: tableRows, headers, getTableProps, getRowProps }) => (
            <TableContainer title="Distributed Traces" description={`${traces.length} traces in selected time range`}>
              <Table {...getTableProps()}>
                <TableHead>
                  <TableRow>
                    {headers.map((h) => <TableHeader key={h.key}>{h.header}</TableHeader>)}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {tableRows.map((row) => {
                    const { key: rk, ...rp } = getRowProps({ row });
                    return (
                      <TableRow key={rk} {...rp}>
                        {row.cells.map((cell) => (
                          <TableCell key={cell.id}>
                            {cell.info.header === "status" ? (
                              <Tag type={cell.value === "success" ? "green" : "red"}>{cell.value as string}</Tag>
                            ) : cell.info.header === "jaeger" && cell.value ? (
                              <Link href={cell.value as string} target="_blank" rel="noopener noreferrer">
                                View ↗
                              </Link>
                            ) : (
                              (cell.value as string)
                            )}
                          </TableCell>
                        ))}
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </DataTable>
      )}
    </div>
  );
}
