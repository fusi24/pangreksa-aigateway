/**
 * API response shapes for the Pangreksa AI Gateway Console.
 * All shapes are sourced from SRS §6.2, §6.3, §9.x.
 */

// ─── Metrics ─────────────────────────────────────────────────────────────────

export interface MetricPoint {
  ts: string;
  value: number;
}

export interface MetricSeries {
  metric: string;
  points: MetricPoint[];
}

export interface MetricsSummary {
  total_requests: number;
  total_cost_usd: number;
  total_input_tokens: number;
  total_output_tokens: number;
  avg_latency_p95_ms: number;
  error_rate_pct: number;
}

export interface MetricsResponse {
  series: MetricSeries[];
  summary: MetricsSummary;
}

// ─── Budget ───────────────────────────────────────────────────────────────────

export type BudgetStatus = "OK" | "WARNING" | "CRITICAL";

export interface BudgetRule {
  id: string;
  name: string;
  entity: string;
  entity_type: "user" | "org" | "team";
  period: string;
  spend_usd: number;
  limit_usd: number;
  pct_consumed: number;
  status: BudgetStatus;
}

export interface BudgetSummaryResponse {
  rules: BudgetRule[];
}

// ─── Pods ─────────────────────────────────────────────────────────────────────

export type PodStatus = "healthy" | "degraded" | "dead";

export interface PodRecord {
  gateway_id: string;
  status: PodStatus;
  version: string;
  config_version: string;
  uptime_seconds: number;
  last_seen_at: string;
  provider_ports: Record<string, number>;
  otel_endpoint: string;
  recent_error_count: number;
  registered_at: string;
}

export interface PodsResponse {
  pods: PodRecord[];
  total: number;
  healthy: number;
  degraded: number;
  dead: number;
}

// ─── Logs ─────────────────────────────────────────────────────────────────────

export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogEntry {
  ts: string;
  level: LogLevel;
  gateway_id: string;
  request_id: string;
  trace_id: string;
  message: string;
  fields: Record<string, unknown>;
}

export interface LogsResponse {
  logs: LogEntry[];
  next_cursor: string | null;
  total_matched: number;
}

// ─── Traces ───────────────────────────────────────────────────────────────────

export interface TraceRecord {
  trace_id: string;
  request_id: string;
  root_operation: string;
  root_latency_ms: number;
  span_count: number;
  status: "success" | "error";
  jaeger_url: string;
  created_at: string;
}

export interface TracesResponse {
  traces: TraceRecord[];
}

// ─── Request Drill-Down ───────────────────────────────────────────────────────

export interface RequestLogRecord {
  request_id: string;
  gateway_id: string;
  user_id: string | null;
  org_id: string;
  provider: string;
  model: string;
  status: "success" | "error";
  latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  prompt_fqn: string | null;
  skills_used: string[];
  guardrails_hit: string[];
  created_at: string;
}

export interface CostRecord {
  request_id: string;
  cost_usd: number;
  input_cost_usd: number;
  output_cost_usd: number;
}

export interface RequestDetail {
  request: RequestLogRecord;
  cost: CostRecord | null;
  trace: {
    trace_id: string;
    jaeger_url: string;
  } | null;
}

// ─── Config ───────────────────────────────────────────────────────────────────

export interface PromptVersion {
  fqn: string;
  repo: string;
  name: string;
  version: number;
  content: string;
  config: {
    variables: string[];
    guardrails: string[];
  };
  is_active: boolean;
  created_at: string;
}

export interface PromptRepo {
  name: string;
  prompts: PromptVersion[];
}

export interface PromptsResponse {
  repos: PromptRepo[];
}

export interface SkillRecord {
  id: string;
  name: string;
  description: string;
  content: string;
  preload: boolean;
  usage_7d: number;
  updated_at: string;
}

export interface McpServerRecord {
  id: string;
  name: string;
  url: string;
  auth_type: "none" | "bearer" | "basic";
  tool_count: number;
  hitl_enabled: boolean;
  last_tested_at: string | null;
  status: "reachable" | "unreachable" | "untested";
}

export interface McpTool {
  name: string;
  description: string;
  hitl: boolean;
  input_schema: Record<string, unknown>;
}

export interface McpTestResult {
  reachable: boolean;
  tools: McpTool[];
}

export interface GuardrailRecord {
  id: string;
  name: string;
  type: string;
  hook: "pre_request" | "post_response" | "streaming";
  mode: "block" | "flag" | "audit";
  enforcement: "hard" | "soft";
  config: Record<string, unknown>;
  enabled: boolean;
  priority: number;
  hit_count_7d: number;
}

export interface BudgetRuleRecord {
  id: string;
  name: string;
  entity_type: "user" | "org" | "team";
  entity_id: string | null;
  limit_usd: number;
  period: "daily" | "weekly" | "monthly";
  priority: number;
  config_yaml: string;
}

export interface RateLimitRuleRecord {
  id: string;
  name: string;
  entity_type: "user" | "org" | "team";
  entity_id: string | null;
  limit_rpm: number | null;
  limit_tpm: number | null;
  priority: number;
  config_yaml: string;
}

export interface EntitlementRecord {
  user_id: string;
  email: string;
  allowed_prompts: string[];
  allowed_skills: string[];
  allowed_mcps: string[];
  budget_limit_usd: number | null;
  rate_limit_rpm: number | null;
  rate_limit_tpm: number | null;
  data_scope: string;
  permissions: string[];
}

export interface AuditLogEntry {
  id: string;
  ts: string;
  user_id: string;
  user_email: string;
  event_type: string;
  resource_type: string;
  resource_id: string;
  summary: string;
  old_value: Record<string, unknown> | null;
  new_value: Record<string, unknown> | null;
}

export interface AuditLogResponse {
  entries: AuditLogEntry[];
  next_cursor: string | null;
  total_matched: number;
}

// ─── Reports ─────────────────────────────────────────────────────────────────

export type ReportType =
  | "usage_summary"
  | "budget_report"
  | "guardrail_audit"
  | "request_detail"
  | "cost_breakdown";

export type ReportScope = "org" | "user" | "provider" | "model";
export type ReportFormat = "docx" | "xlsx" | "pdf";

export interface ReportParams {
  type: ReportType;
  from: string;
  to: string;
  scope: ReportScope;
  scope_id?: string;
  format: ReportFormat;
}

export interface ReportDataRow {
  [key: string]: string | number | boolean | null;
}

export interface ReportData {
  summary: Record<string, number | string>;
  rows: ReportDataRow[];
  meta: {
    type: ReportType;
    from: string;
    to: string;
    scope: ReportScope;
    scope_id?: string;
    generated_at: string;
    generated_by: string;
  };
}

// ─── SSE Events ──────────────────────────────────────────────────────────────

export interface TransactionEvent {
  request_id: string;
  gateway_id: string;
  user_id: string | null;
  provider: string;
  model: string;
  status: "success" | "error";
  latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  guardrails_hit: string[];
  skills_used: string[];
  created_at: string;
}

export interface PodHeartbeatEvent {
  gateway_id: string;
  status: PodStatus;
  last_seen_at: string;
  config_version: string;
}

export interface BudgetAlertEvent {
  rule_id: string;
  rule_name: string;
  entity: string;
  threshold_pct: number;
  spend_usd: number;
  limit_usd: number;
  period: string;
}

export type SseEvent =
  | { type: "transaction"; data: TransactionEvent }
  | { type: "pod_heartbeat"; data: PodHeartbeatEvent }
  | { type: "budget_alert"; data: BudgetAlertEvent };

// ─── API Key Management ───────────────────────────────────────────────────────

export interface ApiKeyRecord {
  id: string;
  name: string;
  prefix: string;
  type: "PAT" | "VAT";
  created_at: string;
  expires_at: string | null;
  last_used_at: string | null;
}

export interface ApiKeyCreateResponse {
  id: string;
  name: string;
  prefix: string;
  token: string;
}

// ─── Generic API error shape ──────────────────────────────────────────────────

export interface ApiErrorBody {
  error: string;
  code?: string;
}
