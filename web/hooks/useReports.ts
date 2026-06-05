"use client";

import { useState } from "react";
import type { ReportParams, ReportData } from "@/types/api";

/**
 * Triggers a browser file download for a Blob.
 */
function triggerDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

interface UseReportsResult {
  generate: (params: ReportParams) => Promise<void>;
  generating: boolean;
  error: string | null;
}

/**
 * Orchestrates report generation:
 * - DOCX/XLSX: fetches data from /api/reports/data, generates client-side
 * - PDF: streams from /api/reports/pdf (Central Server server-side generation)
 */
export function useReports(): UseReportsResult {
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function generate(params: ReportParams): Promise<void> {
    setGenerating(true);
    setError(null);

    try {
      const dateStr = new Date().toISOString().split("T")[0] ?? "report";
      const filename = `pangreksa-${params.type}-${dateStr}`;

      if (params.format === "pdf") {
        const res = await fetch("/api/reports/pdf", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(params),
        });
        if (!res.ok) throw new Error("PDF generation failed");
        const blob = await res.blob();
        triggerDownload(blob, `${filename}.pdf`);
      } else {
        // Fetch aggregated data then generate client-side
        const dataParams = new URLSearchParams({
          type: params.type,
          from: params.from,
          to: params.to,
          scope: params.scope,
        });
        if (params.scope_id) dataParams.set("scope_id", params.scope_id);

        const dataRes = await fetch(`/api/reports/data?${dataParams.toString()}`);
        if (!dataRes.ok) throw new Error("Failed to fetch report data");
        const data = (await dataRes.json()) as ReportData;

        if (params.format === "docx") {
          const { generateDocx } = await import("@/lib/reports/docx-generator");
          const blob = await generateDocx(params, data);
          triggerDownload(blob, `${filename}.docx`);
        } else {
          const { generateXlsx } = await import("@/lib/reports/xlsx-generator");
          const blob = await generateXlsx(params, data);
          triggerDownload(blob, `${filename}.xlsx`);
        }
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Report generation failed");
    } finally {
      setGenerating(false);
    }
  }

  return { generate, generating, error };
}
