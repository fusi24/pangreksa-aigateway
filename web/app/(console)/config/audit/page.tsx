"use client";

import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import {
  DataTable, Table, TableHead, TableRow, TableHeader, TableBody, TableCell,
  TableContainer, TableToolbar, TableToolbarContent, TableToolbarSearch,
  Tag, InlineNotification, InlineLoading, Button,
} from "@carbon/react";
import { ChevronDown, ChevronUp } from "@carbon/icons-react";
import { CodeBlock } from "@/components/telemetry/CodeBlock";
import { fmtTs, truncate } from "@/lib/utils/format";
import type { AuditLogResponse, AuditLogEntry } from "@/types/api";

const HEADERS = [
  { key: "ts", header: "Timestamp" },
  { key: "user_email", header: "User" },
  { key: "event_type", header: "Event" },
  { key: "resource_type", header: "Resource type" },
  { key: "resource_id", header: "Resource ID" },
  { key: "summary", header: "Summary" },
  { key: "_expand", header: "" },
];

/**
 * Config Change Audit Trail — read-only, expandable diff view.
 */
export default function AuditPage() {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading, error } =
    useInfiniteQuery<AuditLogResponse>({
      queryKey: ["config-audit"],
      queryFn: async ({ pageParam }) => {
        const params = new URLSearchParams({ limit: "50" });
        if (pageParam) params.set("cursor", pageParam as string);
        const res = await fetch(`/api/config/audit?${params.toString()}`);
        if (!res.ok) throw new Error("Failed to load audit log");
        return res.json() as Promise<AuditLogResponse>;
      },
      initialPageParam: undefined,
      getNextPageParam: (last) => last.next_cursor ?? undefined,
    });

  const entries: AuditLogEntry[] = data?.pages.flatMap((p) => p.entries) ?? [];

  function toggleExpand(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  const rows = entries.map((e) => ({
    id: e.id,
    ts: fmtTs(e.ts),
    user_email: e.user_email,
    event_type: e.event_type,
    resource_type: e.resource_type,
    resource_id: truncate(e.resource_id, 20),
    summary: truncate(e.summary, 50),
    _expand: e.id,
    _entry: e,
  }));

  if (error) {
    return <InlineNotification kind="error" title="Failed to load audit log" subtitle={(error as Error).message} />;
  }

  return (
    <div>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>Config Audit Trail</h1>

      <DataTable rows={rows} headers={HEADERS} size="sm">
        {({ rows: tableRows, headers, getTableProps, getRowProps }) => (
          <TableContainer title="Audit Log" description="Config mutations by all admin users">
            <TableToolbar>
              <TableToolbarContent>
                <TableToolbarSearch />
              </TableToolbarContent>
            </TableToolbar>
            <Table {...getTableProps()}>
              <TableHead>
                <TableRow>{headers.map((h) => <TableHeader key={h.key}>{h.header}</TableHeader>)}</TableRow>
              </TableHead>
              <TableBody>
                {isLoading && (
                  <TableRow>
                    <TableCell colSpan={7}><InlineLoading description="Loading audit log…" /></TableCell>
                  </TableRow>
                )}
                {tableRows.map((row) => {
                  const { key: rk, ...rp } = getRowProps({ row });
                  const entry = (row as typeof row & { _entry?: AuditLogEntry })._entry;
                  const isExpanded = expanded.has(row.id);

                  return (
                    <>
                      <TableRow key={rk} {...rp}>
                        {row.cells.map((cell) => {
                          if (cell.info.header === "_expand") {
                            return (
                              <TableCell key={cell.id} style={{ width: 40 }}>
                                <Button kind="ghost" size="sm" hasIconOnly
                                  renderIcon={isExpanded ? ChevronUp : ChevronDown}
                                  iconDescription="Toggle diff"
                                  onClick={() => toggleExpand(row.id)} />
                              </TableCell>
                            );
                          }
                          if (cell.info.header === "event_type") return <TableCell key={cell.id}><Tag type="blue">{cell.value as string}</Tag></TableCell>;
                          if (cell.info.header === "resource_type") return <TableCell key={cell.id}><Tag type="gray">{cell.value as string}</Tag></TableCell>;
                          return <TableCell key={cell.id}>{cell.value as string}</TableCell>;
                        })}
                      </TableRow>
                      {isExpanded && entry && (entry.old_value || entry.new_value) && (
                        <TableRow key={`${rk}-diff`}>
                          <TableCell colSpan={7} style={{ padding: "0.5rem 1rem" }}>
                            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" }}>
                              <div>
                                <div style={{ fontWeight: 600, fontSize: "0.75rem", marginBottom: "0.25rem", color: "#da1e28" }}>Old value</div>
                                {entry.old_value
                                  ? <CodeBlock code={JSON.stringify(entry.old_value, null, 2)} language="json" maxLines={15} />
                                  : <span style={{ color: "#525252", fontSize: "0.75rem" }}>—</span>}
                              </div>
                              <div>
                                <div style={{ fontWeight: 600, fontSize: "0.75rem", marginBottom: "0.25rem", color: "#198038" }}>New value</div>
                                {entry.new_value
                                  ? <CodeBlock code={JSON.stringify(entry.new_value, null, 2)} language="json" maxLines={15} />
                                  : <span style={{ color: "#525252", fontSize: "0.75rem" }}>—</span>}
                              </div>
                            </div>
                          </TableCell>
                        </TableRow>
                      )}
                    </>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </DataTable>

      {hasNextPage && (
        <div style={{ textAlign: "center", marginTop: "1rem" }}>
          <Button kind="ghost" onClick={() => fetchNextPage()} disabled={isFetchingNextPage}>
            {isFetchingNextPage ? "Loading…" : "Load more"}
          </Button>
        </div>
      )}
    </div>
  );
}
