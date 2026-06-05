"use client";

import { useState } from "react";
import { formatISO, subDays } from "date-fns";
import {
  ProgressIndicator,
  ProgressStep,
  ContentSwitcher,
  Switch,
  DatePicker,
  DatePickerInput,
  Select,
  SelectItem,
  RadioButtonGroup,
  RadioButton,
  Button,
  InlineNotification,
  Tile,
  InlineLoading,
} from "@carbon/react";
import { Download } from "@carbon/icons-react";
import { useReports } from "@/hooks/useReports";
import type { ReportType, ReportScope, ReportFormat } from "@/types/api";

const REPORT_TYPES: Array<{ key: ReportType; label: string; description: string }> = [
  { key: "usage_summary", label: "Usage Summary", description: "Request volume, latency, error rate over the period." },
  { key: "budget_report", label: "Budget Report", description: "Spend vs. limits per budget rule." },
  { key: "guardrail_audit", label: "Guardrail Audit", description: "All guardrail events, rules fired, and outcomes." },
  { key: "request_detail", label: "Request Detail", description: "Full request log export for the period." },
  { key: "cost_breakdown", label: "Cost Breakdown", description: "Cost by model, provider, and user." },
];

const ROW_WARNING_THRESHOLD = 10_000;

/**
 * Report Builder — 4-step wizard for DOCX, XLSX, and PDF report generation.
 */
export default function ReportsPage() {
  const { generate, generating, error } = useReports();

  const [step, setStep] = useState(0);
  const [reportType, setReportType] = useState<ReportType>("usage_summary");
  const [from, setFrom] = useState<Date>(subDays(new Date(), 30));
  const [to, setTo] = useState<Date>(new Date());
  const [scope, setScope] = useState<ReportScope>("org");
  const [scopeId, setScopeId] = useState("");
  const [format, setFormat] = useState<ReportFormat>("xlsx");

  const dayDiff = Math.round((to.getTime() - from.getTime()) / 86_400_000);
  const estimatedRows = dayDiff * 1000;
  const showRowWarning = estimatedRows > ROW_WARNING_THRESHOLD;

  async function handleGenerate() {
    await generate({
      type: reportType,
      from: formatISO(from),
      to: formatISO(to),
      scope,
      ...(scopeId ? { scope_id: scopeId } : {}),
      format,
    });
  }

  const selectedType = REPORT_TYPES.find((t) => t.key === reportType);

  return (
    <div>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>Report Builder</h1>

      <ProgressIndicator currentIndex={step} style={{ marginBottom: "2rem" }}>
        <ProgressStep label="Report type" description="What to include" />
        <ProgressStep label="Scope & dates" description="Date range and filters" />
        <ProgressStep label="Output format" description="DOCX, XLSX, or PDF" />
        <ProgressStep label="Generate" description="Download your report" />
      </ProgressIndicator>

      {/* Step 0: Report Type */}
      {step === 0 && (
        <Tile>
          <h3 style={{ marginBottom: "1rem" }}>Select Report Type</h3>
          <ContentSwitcher
            selectedIndex={REPORT_TYPES.findIndex((t) => t.key === reportType)}
            onChange={({ index }) => {
              if (typeof index === "number") {
                const t = REPORT_TYPES[index];
                if (t) setReportType(t.key);
              }
            }}
            size="md"
          >
            {REPORT_TYPES.map((t) => <Switch key={t.key} name={t.key} text={t.label} />)}
          </ContentSwitcher>
          {selectedType && (
            <p style={{ marginTop: "1rem", color: "#525252" }}>{selectedType.description}</p>
          )}
          <div style={{ marginTop: "1.5rem" }}>
            <Button onClick={() => setStep(1)}>Next</Button>
          </div>
        </Tile>
      )}

      {/* Step 1: Scope & Dates */}
      {step === 1 && (
        <Tile>
          <h3 style={{ marginBottom: "1rem" }}>Scope & Date Range</h3>
          <div style={{ display: "flex", flexDirection: "column", gap: "1rem", maxWidth: 500 }}>
            <DatePicker
              datePickerType="range"
              onChange={(dates: Date[]) => {
                if (dates[0]) setFrom(dates[0]);
                if (dates[1]) setTo(dates[1]);
              }}
              value={[from, to]}
            >
              <DatePickerInput id="report-from" placeholder="mm/dd/yyyy" labelText="From" />
              <DatePickerInput id="report-to" placeholder="mm/dd/yyyy" labelText="To" />
            </DatePicker>

            <Select id="report-scope" labelText="Scope" value={scope}
              onChange={(e) => setScope(e.target.value as ReportScope)}>
              <SelectItem value="org" text="Organization-wide" />
              <SelectItem value="user" text="By user" />
              <SelectItem value="provider" text="By provider" />
              <SelectItem value="model" text="By model" />
            </Select>

            {scope !== "org" && (
              <div>
                <label style={{ fontSize: "0.875rem", fontWeight: 600, display: "block", marginBottom: "0.25rem" }}>
                  {scope === "user" ? "User ID" : scope === "provider" ? "Provider name" : "Model name"}
                </label>
                <input
                  style={{ width: "100%", padding: "0.5rem", border: "1px solid #e0e0e0", borderRadius: 4, fontFamily: "IBM Plex Mono, monospace", fontSize: "0.875rem" }}
                  value={scopeId}
                  onChange={(e) => setScopeId(e.target.value)}
                  placeholder={scope === "user" ? "uuid" : scope === "provider" ? "openai" : "gpt-4o"}
                />
              </div>
            )}
          </div>
          <div style={{ marginTop: "1.5rem", display: "flex", gap: "1rem" }}>
            <Button kind="secondary" onClick={() => setStep(0)}>Back</Button>
            <Button onClick={() => setStep(2)}>Next</Button>
          </div>
        </Tile>
      )}

      {/* Step 2: Format */}
      {step === 2 && (
        <Tile>
          <h3 style={{ marginBottom: "1rem" }}>Output Format</h3>

          {showRowWarning && (
            <InlineNotification kind="warning" title="Large report"
              subtitle={`Estimated ~${estimatedRows.toLocaleString()} rows. Consider narrowing the date range for faster generation.`}
              lowContrast hideCloseButton style={{ marginBottom: "1rem" }} />
          )}

          <RadioButtonGroup
            legendText="Select format"
            name="report-format"
            valueSelected={format}
            onChange={(value: string | number | undefined) => { if (value !== undefined) setFormat(value as ReportFormat); }}
          >
            <RadioButton labelText="Excel (XLSX) — recommended for large datasets" value="xlsx" id="fmt-xlsx" />
            <RadioButton labelText="Word (DOCX) — formatted narrative report" value="docx" id="fmt-docx" />
            <RadioButton labelText="PDF — server-generated, print-ready" value="pdf" id="fmt-pdf" />
          </RadioButtonGroup>
          <div style={{ marginTop: "1.5rem", display: "flex", gap: "1rem" }}>
            <Button kind="secondary" onClick={() => setStep(1)}>Back</Button>
            <Button onClick={() => setStep(3)}>Next</Button>
          </div>
        </Tile>
      )}

      {/* Step 3: Generate */}
      {step === 3 && (
        <Tile>
          <h3 style={{ marginBottom: "1rem" }}>Generate Report</h3>
          <div style={{ color: "#525252", marginBottom: "1.5rem", lineHeight: 1.6 }}>
            <div><strong>Report type:</strong> {selectedType?.label}</div>
            <div><strong>Period:</strong> {from.toLocaleDateString()} – {to.toLocaleDateString()}</div>
            <div><strong>Scope:</strong> {scope}{scopeId ? ` (${scopeId})` : ""}</div>
            <div><strong>Format:</strong> {format.toUpperCase()}</div>
          </div>

          {error && (
            <InlineNotification kind="error" title="Generation failed" subtitle={error}
              lowContrast hideCloseButton style={{ marginBottom: "1rem" }} />
          )}

          <div style={{ display: "flex", gap: "1rem", alignItems: "center" }}>
            <Button kind="secondary" onClick={() => setStep(2)} disabled={generating}>Back</Button>
            <Button {...(!generating ? { renderIcon: Download } : {})}
              onClick={handleGenerate} disabled={generating}>
              {generating ? <><InlineLoading /> Generating…</> : "Generate & Download"}
            </Button>
          </div>
        </Tile>
      )}
    </div>
  );
}
