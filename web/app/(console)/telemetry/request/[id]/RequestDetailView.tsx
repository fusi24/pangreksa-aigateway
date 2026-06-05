"use client";

import { Grid, Column, Tile, Tag, Link, Breadcrumb, BreadcrumbItem } from "@carbon/react";
import { fmtTs, fmtMs, fmtUsd, fmtTokens } from "@/lib/utils/format";
import type { RequestDetail } from "@/types/api";

interface RequestDetailViewProps {
  detail: RequestDetail;
  jaegerBaseUrl: string;
}

/**
 * Client component rendering the Request Drill-Down UI.
 * Data is fetched server-side in the parent RSC page and passed as props.
 */
export function RequestDetailView({ detail, jaegerBaseUrl }: RequestDetailViewProps) {
  const { request: req, cost, trace } = detail;

  return (
    <div>
      <Breadcrumb style={{ marginBottom: "1rem" }}>
        <BreadcrumbItem href="/dashboard">Dashboard</BreadcrumbItem>
        <BreadcrumbItem href="/telemetry/traces">Telemetry</BreadcrumbItem>
        <BreadcrumbItem isCurrentPage>{req.request_id.slice(0, 8)}…</BreadcrumbItem>
      </Breadcrumb>

      <h1 style={{ fontSize: "1.5rem", fontWeight: 600, marginBottom: "1.5rem" }}>
        Request Detail
        <code style={{ fontSize: "1rem", marginLeft: "0.75rem", fontFamily: "IBM Plex Mono, monospace", color: "#525252" }}>
          {req.request_id}
        </code>
      </h1>

      <Grid>
        {/* Left: Request metadata */}
        <Column sm={4} md={4} lg={8}>
          <Tile>
            <h3 style={{ marginBottom: "1rem" }}>Request</h3>
            <dl style={{ display: "grid", gridTemplateColumns: "140px 1fr", gap: "0.5rem 1rem" }}>
              <dt style={{ color: "#525252", fontWeight: 600 }}>Gateway</dt>
              <dd>{req.gateway_id}</dd>
              <dt style={{ color: "#525252", fontWeight: 600 }}>User</dt>
              <dd>{req.user_id ?? "anonymous"}</dd>
              <dt style={{ color: "#525252", fontWeight: 600 }}>Provider</dt>
              <dd>{req.provider}</dd>
              <dt style={{ color: "#525252", fontWeight: 600 }}>Model</dt>
              <dd>{req.model}</dd>
              <dt style={{ color: "#525252", fontWeight: 600 }}>Status</dt>
              <dd>
                <Tag type={req.status === "success" ? "green" : "red"}>
                  {req.status.toUpperCase()}
                </Tag>
              </dd>
              <dt style={{ color: "#525252", fontWeight: 600 }}>Latency</dt>
              <dd>{fmtMs(req.latency_ms)}</dd>
              <dt style={{ color: "#525252", fontWeight: 600 }}>Tokens</dt>
              <dd>
                {fmtTokens(req.input_tokens)} in / {fmtTokens(req.output_tokens)} out
              </dd>
              <dt style={{ color: "#525252", fontWeight: 600 }}>Created</dt>
              <dd>{fmtTs(req.created_at)}</dd>
            </dl>
          </Tile>
        </Column>

        {/* Right: Cost + Trace */}
        <Column sm={4} md={4} lg={8}>
          {cost && (
            <Tile style={{ marginBottom: "1rem" }}>
              <h3 style={{ marginBottom: "1rem" }}>Cost</h3>
              <dl style={{ display: "grid", gridTemplateColumns: "140px 1fr", gap: "0.5rem 1rem" }}>
                <dt style={{ color: "#525252", fontWeight: 600 }}>Total</dt>
                <dd style={{ fontWeight: 700, color: "#198038" }}>{fmtUsd(cost.cost_usd)}</dd>
                <dt style={{ color: "#525252", fontWeight: 600 }}>Input</dt>
                <dd>{fmtUsd(cost.input_cost_usd)}</dd>
                <dt style={{ color: "#525252", fontWeight: 600 }}>Output</dt>
                <dd>{fmtUsd(cost.output_cost_usd)}</dd>
              </dl>
            </Tile>
          )}

          {trace && (
            <Tile>
              <h3 style={{ marginBottom: "1rem" }}>Trace</h3>
              <code style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem", display: "block", marginBottom: "0.5rem" }}>
                {trace.trace_id}
              </code>
              {jaegerBaseUrl && (
                <Link href={trace.jaeger_url} target="_blank" rel="noopener noreferrer">
                  View in Jaeger ↗
                </Link>
              )}
            </Tile>
          )}
        </Column>

        {/* Guardrails */}
        {req.guardrails_hit.length > 0 && (
          <Column sm={4} md={4} lg={8} style={{ marginTop: "1rem" }}>
            <Tile>
              <h3 style={{ marginBottom: "0.75rem" }}>Guardrails Fired</h3>
              <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
                {req.guardrails_hit.map((g) => (
                  <Tag key={g} type="red">{g}</Tag>
                ))}
              </div>
            </Tile>
          </Column>
        )}

        {/* Skills + Prompt */}
        {(req.skills_used.length > 0 || req.prompt_fqn) && (
          <Column sm={4} md={4} lg={8} style={{ marginTop: "1rem" }}>
            <Tile>
              <h3 style={{ marginBottom: "0.75rem" }}>Components Used</h3>
              {req.prompt_fqn && (
                <div style={{ marginBottom: "0.5rem" }}>
                  <span style={{ color: "#525252", fontSize: "0.875rem" }}>Prompt FQN: </span>
                  <Tag type="blue">{req.prompt_fqn}</Tag>
                </div>
              )}
              {req.skills_used.length > 0 && (
                <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
                  {req.skills_used.map((s) => (
                    <Tag key={s} type="purple">{s}</Tag>
                  ))}
                </div>
              )}
            </Tile>
          </Column>
        )}
      </Grid>
    </div>
  );
}
