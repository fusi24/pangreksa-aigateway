import { format, formatDistanceToNow } from "date-fns";

/**
 * Formats an ISO 8601 timestamp as "yyyy-MM-dd HH:mm:ss" in local timezone.
 */
export function fmtTs(iso: string): string {
  return format(new Date(iso), "yyyy-MM-dd HH:mm:ss");
}

/**
 * Formats an ISO 8601 timestamp as a human-readable relative string (e.g. "3 minutes ago").
 */
export function fmtRelative(iso: string): string {
  return formatDistanceToNow(new Date(iso), { addSuffix: true });
}

/**
 * Formats a USD cost value to 4 decimal places (e.g. "$0.0038").
 */
export function fmtUsd(n: number): string {
  return `$${n.toFixed(4)}`;
}

/**
 * Formats a latency value in milliseconds with thousands separator (e.g. "1,234ms").
 */
export function fmtMs(n: number): string {
  return `${n.toLocaleString()}ms`;
}

/**
 * Formats a token count with K/M suffixes for large values.
 */
export function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

/**
 * Formats uptime in seconds as a human-readable string (e.g. "2d 3h 45m").
 */
export function fmtUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);

  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0) parts.push(`${h}h`);
  if (m > 0 || parts.length === 0) parts.push(`${m}m`);

  return parts.join(" ");
}

/**
 * Truncates a string to maxLen characters, appending "…" if truncated.
 */
export function truncate(s: string, maxLen: number): string {
  return s.length > maxLen ? `${s.slice(0, maxLen)}…` : s;
}
