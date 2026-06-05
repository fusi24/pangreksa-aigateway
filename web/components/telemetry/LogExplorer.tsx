"use client";

import { useState, useCallback, useMemo } from "react";
import { Virtuoso } from "react-virtuoso";
import { useInfiniteQuery } from "@tanstack/react-query";
import {
  Search,
  MultiSelect,
  DatePicker,
  DatePickerInput,
  Button,
  Tag,
  InlineNotification,
  InlineLoading,
} from "@carbon/react";
import { CodeBlock } from "./CodeBlock";
import { fmtTs, truncate } from "@/lib/utils/format";
import type { LogEntry, LogsResponse, LogLevel } from "@/types/api";

const LEVEL_OPTIONS: Array<{ id: LogLevel; label: string }> = [
  { id: "debug", label: "DEBUG" },
  { id: "info", label: "INFO" },
  { id: "warn", label: "WARN" },
  { id: "error", label: "ERROR" },
];

const LEVEL_TAG_TYPE: Record<LogLevel, string> = {
  debug: "gray",
  info: "blue",
  warn: "warm-gray",
  error: "red",
};

/**
 * Paginated, searchable structured log viewer.
 * Uses react-virtuoso for virtual scroll (handles 10k+ rows without lag).
 * Expandable rows show JSON fields via CodeBlock.
 */
export function LogExplorer() {
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [levels, setLevels] = useState<LogLevel[]>([]);
  const [gatewayId, setGatewayId] = useState("");
  const [dateRange, setDateRange] = useState<{ from: Date | null; to: Date | null }>({
    from: null,
    to: null,
  });
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  // Debounce search input
  const handleQueryChange = useCallback((value: string) => {
    setQuery(value);
    const timer = setTimeout(() => setDebouncedQuery(value), 300);
    return () => clearTimeout(timer);
  }, []);

  const buildParams = useCallback(() => {
    const p: Record<string, string> = {
      from: dateRange.from ? dateRange.from.toISOString() : new Date(Date.now() - 24 * 3600_000).toISOString(),
      to: dateRange.to ? dateRange.to.toISOString() : new Date().toISOString(),
      limit: "100",
    };
    if (debouncedQuery) p["q"] = debouncedQuery;
    if (levels.length > 0) p["level"] = levels.join(",");
    if (gatewayId) p["gateway_id"] = gatewayId;
    return p;
  }, [debouncedQuery, levels, gatewayId, dateRange]);

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading, error } =
    useInfiniteQuery<LogsResponse>({
      queryKey: ["logs", buildParams()],
      queryFn: async ({ pageParam }) => {
        const params = new URLSearchParams(buildParams());
        if (pageParam) params.set("cursor", pageParam as string);
        const res = await fetch(`/api/logs?${params.toString()}`);
        if (!res.ok) throw new Error("Failed to fetch logs");
        return res.json() as Promise<LogsResponse>;
      },
      initialPageParam: undefined,
      getNextPageParam: (last) => last.next_cursor ?? undefined,
    });

  const allLogs: LogEntry[] = useMemo(
    () => data?.pages.flatMap((p) => p.logs) ?? [],
    [data]
  );

  const totalMatched = data?.pages[0]?.total_matched ?? 0;
  const jaegerBaseUrl = process.env.NEXT_PUBLIC_JAEGER_EXTERNAL_URL ?? "";

  function toggleExpand(id: string) {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <div>
      {/* Filter bar */}
      <div style={{ display: "flex", gap: "1rem", flexWrap: "wrap", alignItems: "flex-end", marginBottom: "1rem" }}>
        <div style={{ flex: "1 1 250px" }}>
          <Search
            id="log-search"
            labelText="Search logs"
            placeholder="Search message, fields…"
            value={query}
            onChange={(e) => handleQueryChange(e.target.value)}
            size="md"
          />
        </div>

        <div style={{ minWidth: 200 }}>
          <MultiSelect
            id="level-filter"
            titleText="Level"
            label="All levels"
            items={LEVEL_OPTIONS}
            itemToString={(item) => item.label}
            onChange={({ selectedItems }) =>
              setLevels((selectedItems ?? []).map((i) => i.id))
            }
            size="md"
          />
        </div>

        <div style={{ minWidth: 200 }}>
          <DatePicker
            datePickerType="range"
            onChange={(dates: Date[]) => {
              setDateRange({
                from: dates[0] ?? null,
                to: dates[1] ?? null,
              });
            }}
          >
            <DatePickerInput
              id="log-date-from"
              placeholder="mm/dd/yyyy"
              labelText="From"
              size="md"
            />
            <DatePickerInput
              id="log-date-to"
              placeholder="mm/dd/yyyy"
              labelText="To"
              size="md"
            />
          </DatePicker>
        </div>
      </div>

      {/* Summary */}
      <div style={{ marginBottom: "0.5rem", color: "#525252", fontSize: "0.875rem" }}>
        {isLoading ? <InlineLoading description="Loading logs…" /> : `${totalMatched.toLocaleString()} matching log entries`}
      </div>

      {error && (
        <InlineNotification
          kind="error"
          title="Failed to load logs"
          subtitle={(error as Error).message}
        />
      )}

      {/* Virtual log list */}
      <div style={{ border: "1px solid #e0e0e0", borderRadius: 4, overflow: "hidden" }}>
        <Virtuoso
          style={{ height: 600 }}
          data={allLogs}
          endReached={() => { if (hasNextPage && !isFetchingNextPage) fetchNextPage(); }}
          itemContent={(_, log) => {
            const isExpanded = expandedIds.has(log.request_id);
            const hasFields = Object.keys(log.fields).length > 0;

            return (
              <div
                style={{
                  padding: "0.5rem 1rem",
                  borderBottom: "1px solid #e0e0e0",
                  cursor: hasFields ? "pointer" : "default",
                  background: isExpanded ? "#f4f4f4" : "transparent",
                }}
                onClick={() => hasFields && toggleExpand(log.request_id)}
              >
                <div style={{ display: "flex", gap: "0.75rem", alignItems: "center", flexWrap: "wrap" }}>
                  <span style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem", color: "#525252", whiteSpace: "nowrap" }}>
                    {fmtTs(log.ts)}
                  </span>

                  <Tag type={LEVEL_TAG_TYPE[log.level] ?? "gray"} size="sm">
                    {log.level.toUpperCase()}
                  </Tag>

                  <span style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem", color: "#0f62fe" }}>
                    {truncate(log.gateway_id, 18)}
                  </span>

                  {log.trace_id && jaegerBaseUrl ? (
                    <a
                      href={`${jaegerBaseUrl}/trace/${log.trace_id}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem", color: "#8a3ffc" }}
                      onClick={(e) => e.stopPropagation()}
                    >
                      {truncate(log.trace_id, 12)}
                    </a>
                  ) : null}

                  <span style={{ fontSize: "0.875rem", flexGrow: 1 }}>{log.message}</span>
                </div>

                {isExpanded && hasFields && (
                  <div style={{ marginTop: "0.5rem" }}>
                    <CodeBlock
                      code={JSON.stringify(log.fields, null, 2)}
                      language="json"
                      maxLines={20}
                    />
                  </div>
                )}
              </div>
            );
          }}
        />
      </div>

      {isFetchingNextPage && (
        <div style={{ padding: "0.5rem", textAlign: "center" }}>
          <InlineLoading description="Loading more…" />
        </div>
      )}
    </div>
  );
}
