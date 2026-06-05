"use client";

import { useMemo } from "react";
import {
  Modal,
  StructuredListWrapper,
  StructuredListBody,
  StructuredListRow,
  StructuredListCell,
  Tag,
  Link,
} from "@carbon/react";
import { fmtTs, fmtRelative, fmtUptime } from "@/lib/utils/format";
import type { PodRecord } from "@/types/api";

interface PodDetailDrawerProps {
  pod: PodRecord | null;
  onClose: () => void;
}

/**
 * Slide-over modal showing detailed info for a selected daemon pod.
 */
export function PodDetailDrawer({ pod, onClose }: PodDetailDrawerProps) {
  const jaegerUrl = process.env.NEXT_PUBLIC_JAEGER_EXTERNAL_URL ?? "";

  const statusTag = useMemo(() => {
    if (!pod) return null;
    const type = pod.status === "healthy" ? "green" : pod.status === "degraded" ? "warm-gray" : "red";
    return <Tag type={type}>{pod.status.toUpperCase()}</Tag>;
  }, [pod]);

  return (
    <Modal
      open={pod !== null}
      modalHeading={pod?.gateway_id ?? "Pod Details"}
      passiveModal
      onRequestClose={onClose}
      size="sm"
    >
      {pod && (
        <StructuredListWrapper>
          <StructuredListBody>
            <StructuredListRow>
              <StructuredListCell noWrap>Gateway ID</StructuredListCell>
              <StructuredListCell>
                <code style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.875rem" }}>
                  {pod.gateway_id}
                </code>
              </StructuredListCell>
            </StructuredListRow>

            <StructuredListRow>
              <StructuredListCell noWrap>Status</StructuredListCell>
              <StructuredListCell>{statusTag}</StructuredListCell>
            </StructuredListRow>

            <StructuredListRow>
              <StructuredListCell noWrap>Version</StructuredListCell>
              <StructuredListCell>{pod.version}</StructuredListCell>
            </StructuredListRow>

            <StructuredListRow>
              <StructuredListCell noWrap>Config Version</StructuredListCell>
              <StructuredListCell>
                <code style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem" }}>
                  {pod.config_version.slice(0, 12)}
                </code>
              </StructuredListCell>
            </StructuredListRow>

            <StructuredListRow>
              <StructuredListCell noWrap>Uptime</StructuredListCell>
              <StructuredListCell>{fmtUptime(pod.uptime_seconds)}</StructuredListCell>
            </StructuredListRow>

            <StructuredListRow>
              <StructuredListCell noWrap>Last Seen</StructuredListCell>
              <StructuredListCell>{fmtRelative(pod.last_seen_at)} ({fmtTs(pod.last_seen_at)})</StructuredListCell>
            </StructuredListRow>

            <StructuredListRow>
              <StructuredListCell noWrap>OTEL Endpoint</StructuredListCell>
              <StructuredListCell>
                <code style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem" }}>
                  {pod.otel_endpoint || "—"}
                </code>
              </StructuredListCell>
            </StructuredListRow>

            <StructuredListRow>
              <StructuredListCell noWrap>Recent Errors (5m)</StructuredListCell>
              <StructuredListCell>
                <span style={{ color: pod.recent_error_count > 0 ? "#da1e28" : "#198038", fontWeight: 600 }}>
                  {pod.recent_error_count}
                </span>
              </StructuredListCell>
            </StructuredListRow>

            {Object.keys(pod.provider_ports).length > 0 && (
              <StructuredListRow>
                <StructuredListCell noWrap>Provider Ports</StructuredListCell>
                <StructuredListCell>
                  {Object.entries(pod.provider_ports).map(([provider, port]) => (
                    <div key={provider} style={{ fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem" }}>
                      {provider}: {port}
                    </div>
                  ))}
                </StructuredListCell>
              </StructuredListRow>
            )}

            {jaegerUrl && (
              <StructuredListRow>
                <StructuredListCell noWrap>Traces</StructuredListCell>
                <StructuredListCell>
                  <Link
                    href={`${jaegerUrl}/search?tags=${encodeURIComponent(`gateway_id=${pod.gateway_id}`)}`}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    View traces in Jaeger ↗
                  </Link>
                </StructuredListCell>
              </StructuredListRow>
            )}

            <StructuredListRow>
              <StructuredListCell noWrap>Registered</StructuredListCell>
              <StructuredListCell>{fmtTs(pod.registered_at)}</StructuredListCell>
            </StructuredListRow>
          </StructuredListBody>
        </StructuredListWrapper>
      )}
    </Modal>
  );
}
