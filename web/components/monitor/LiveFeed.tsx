"use client";

import { useState, useMemo } from "react";
import {
  DataTable,
  Table,
  TableHead,
  TableRow,
  TableHeader,
  TableBody,
  TableCell,
  TableContainer,
  TableToolbar,
  TableToolbarContent,
  Tag,
  Button,
  ComboBox,
  MultiSelect,
  InlineLoading,
} from "@carbon/react";
import { Pause, Play } from "@carbon/icons-react";
import { useLiveFeed } from "@/hooks/useLiveFeed";
import { fmtTs, fmtMs, fmtUsd, fmtTokens, truncate } from "@/lib/utils/format";

const STATUS_FILTER_OPTIONS = [
  { id: "success", label: "Success" },
  { id: "error", label: "Error" },
];

const TABLE_HEADERS = [
  { key: "ts", header: "Timestamp" },
  { key: "gateway_id", header: "Gateway" },
  { key: "user_id", header: "User" },
  { key: "provider", header: "Provider" },
  { key: "model", header: "Model" },
  { key: "latency_ms", header: "Latency" },
  { key: "tokens", header: "Tokens" },
  { key: "cost_usd", header: "Cost" },
  { key: "status", header: "Status" },
  { key: "guardrails", header: "Guardrails" },
];

/**
 * Real-time SSE telemetry feed rendered as a Carbon DataTable.
 * New rows animate in at the top. Supports pause/resume and filters.
 */
export function LiveFeed() {
  const { feed, paused, connected, togglePause } = useLiveFeed();
  const [gatewayFilter, setGatewayFilter] = useState<string>("");
  const [statusFilter, setStatusFilter] = useState<string[]>([]);

  const gatewayIds = useMemo(() => {
    const ids = [...new Set(feed.map((e) => e.gateway_id))];
    return ids.map((id) => ({ id, text: id }));
  }, [feed]);

  const filteredFeed = useMemo(() => {
    return feed.filter((e) => {
      if (gatewayFilter && e.gateway_id !== gatewayFilter) return false;
      if (statusFilter.length > 0 && !statusFilter.includes(e.status)) return false;
      return true;
    });
  }, [feed, gatewayFilter, statusFilter]);

  const rows = filteredFeed.slice(0, 100).map((e) => ({
    id: e.request_id,
    ts: fmtTs(e.created_at),
    gateway_id: truncate(e.gateway_id, 16),
    user_id: e.user_id ? truncate(e.user_id, 8) : "anon",
    provider: e.provider,
    model: truncate(e.model, 12),
    latency_ms: fmtMs(e.latency_ms),
    tokens: fmtTokens(e.input_tokens + e.output_tokens),
    cost_usd: fmtUsd(e.cost_usd),
    status: e.status,
    guardrails: String(e.guardrails_hit.length),
  }));

  return (
    <div>
      {/* Controls */}
      <div style={{ display: "flex", gap: "1rem", alignItems: "center", marginBottom: "1rem", flexWrap: "wrap" }}>
        <Button
          kind={paused ? "primary" : "secondary"}
          renderIcon={paused ? Play : Pause}
          onClick={togglePause}
          size="sm"
        >
          {paused ? "Resume" : "Pause"}
        </Button>

        <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
          <InlineLoading
            status={connected ? "active" : "error"}
            description={connected ? "Live" : "Disconnected"}
            style={{ width: "auto" }}
          />
        </div>

        <ComboBox
          id="gateway-filter"
          titleText=""
          placeholder="Filter by gateway"
          items={gatewayIds}
          itemToString={(item) => (item ? item.text : "")}
          onChange={({ selectedItem }) =>
            setGatewayFilter(selectedItem?.id ?? "")
          }
          size="sm"
          style={{ minWidth: 200 }}
        />

        <div style={{ minWidth: 180 }}>
          <MultiSelect
            id="status-filter"
            titleText=""
            label="Filter by status"
            items={STATUS_FILTER_OPTIONS}
            itemToString={(item) => item.label}
            onChange={({ selectedItems }) =>
              setStatusFilter((selectedItems ?? []).map((i) => i.id))
            }
            size="sm"
          />
        </div>

        <span style={{ color: "#525252", fontSize: "0.875rem" }}>
          {filteredFeed.length} events
        </span>
      </div>

      <DataTable rows={rows} headers={TABLE_HEADERS} size="sm">
        {({ rows: tableRows, headers, getTableProps, getRowProps }) => (
          <TableContainer>
            <TableToolbar>
              <TableToolbarContent>
                <span style={{ padding: "0 1rem", fontSize: "0.875rem", color: "#525252" }}>
                  Live Telemetry Feed
                </span>
              </TableToolbarContent>
            </TableToolbar>
            <Table {...getTableProps()}>
              <TableHead>
                <TableRow>
                  {headers.map((h) => (
                    <TableHeader key={h.key}>{h.header}</TableHeader>
                  ))}
                </TableRow>
              </TableHead>
              <TableBody>
                {tableRows.map((row) => {
                  const { key: rowKey, ...rowProps } = getRowProps({ row });
                  return (
                    <TableRow key={rowKey} {...rowProps}>
                      {row.cells.map((cell) => (
                        <TableCell key={cell.id}>
                          {cell.info.header === "status" ? (
                            <Tag type={cell.value === "success" ? "green" : "red"}>
                              {cell.value as string}
                            </Tag>
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
    </div>
  );
}
