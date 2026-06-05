"use client";

import { useQuery } from "@tanstack/react-query";
import { Tabs, Tab, TabList, TabPanels, TabPanel, InlineNotification, SkeletonText, Tag } from "@carbon/react";
import { CrudTable } from "@/components/config/CrudTable";
import { CodeBlock } from "@/components/telemetry/CodeBlock";
import type { BudgetRuleRecord, RateLimitRuleRecord } from "@/types/api";

const BUDGET_QK = ["config", "budget-rules"];
const RATE_QK = ["config", "rate-rules"];

/**
 * Budget and Rate Limit Policy Manager — tabbed view with YAML editor.
 */
export default function PoliciesPage() {
  const budget = useQuery<{ rules?: BudgetRuleRecord[] }>({
    queryKey: BUDGET_QK,
    queryFn: async () => {
      const res = await fetch("/api/config/budget-rules");
      if (!res.ok) throw new Error("Failed to load budget rules");
      return res.json() as Promise<{ rules?: BudgetRuleRecord[] }>;
    },
  });

  const rate = useQuery<{ rules?: RateLimitRuleRecord[] }>({
    queryKey: RATE_QK,
    queryFn: async () => {
      const res = await fetch("/api/config/rate-rules");
      if (!res.ok) throw new Error("Failed to load rate limit rules");
      return res.json() as Promise<{ rules?: RateLimitRuleRecord[] }>;
    },
  });

  const budgetRows = (budget.data?.rules ?? []).sort((a, b) => a.priority - b.priority).map((r) => ({
    id: r.id,
    name: r.name,
    entity_type: r.entity_type,
    limit_usd: `$${r.limit_usd.toFixed(2)}`,
    period: r.period,
    priority: String(r.priority),
    yaml: r.config_yaml,
  }));

  const rateRows = (rate.data?.rules ?? []).sort((a, b) => a.priority - b.priority).map((r) => ({
    id: r.id,
    name: r.name,
    entity_type: r.entity_type,
    limit_rpm: r.limit_rpm !== null ? String(r.limit_rpm) : "—",
    limit_tpm: r.limit_tpm !== null ? String(r.limit_tpm) : "—",
    priority: String(r.priority),
    yaml: r.config_yaml,
  }));

  return (
    <>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>Policies</h1>
      <Tabs>
        <TabList aria-label="Policy types">
          <Tab>Budget Rules</Tab>
          <Tab>Rate Limit Rules</Tab>
        </TabList>
        <TabPanels>
          <TabPanel>
            {budget.error && <InlineNotification kind="error" title="Failed to load budget rules" subtitle={(budget.error as Error).message} />}
            {budget.isLoading ? <SkeletonText paragraph /> : (
              <CrudTable title="Budget Rules"
                headers={[
                  { key: "priority", header: "Priority" },
                  { key: "name", header: "Name" },
                  { key: "entity_type", header: "Entity" },
                  { key: "limit_usd", header: "Limit (USD)" },
                  { key: "period", header: "Period" },
                  { key: "yaml", header: "Config" },
                ]}
                rows={budgetRows} isLoading={budget.isLoading} queryKey={BUDGET_QK}
                deleteUrl={(id) => `/api/config/budget-rules/${id}`}
                onAdd={() => {}} onEdit={() => {}}
                renderCell={(header, value) => {
                  if (header === "yaml") return (
                    <details style={{ cursor: "pointer" }}>
                      <summary style={{ fontSize: "0.75rem", color: "#0f62fe" }}>View YAML</summary>
                      <CodeBlock code={value as string} language="yaml" maxLines={8} />
                    </details>
                  );
                  if (header === "entity_type") return <Tag type="blue">{value as string}</Tag>;
                  return value;
                }}
              />
            )}
          </TabPanel>
          <TabPanel>
            {rate.error && <InlineNotification kind="error" title="Failed to load rate rules" subtitle={(rate.error as Error).message} />}
            {rate.isLoading ? <SkeletonText paragraph /> : (
              <CrudTable title="Rate Limit Rules"
                headers={[
                  { key: "priority", header: "Priority" },
                  { key: "name", header: "Name" },
                  { key: "entity_type", header: "Entity" },
                  { key: "limit_rpm", header: "RPM limit" },
                  { key: "limit_tpm", header: "TPM limit" },
                  { key: "yaml", header: "Config" },
                ]}
                rows={rateRows} isLoading={rate.isLoading} queryKey={RATE_QK}
                deleteUrl={(id) => `/api/config/rate-rules/${id}`}
                onAdd={() => {}} onEdit={() => {}}
                renderCell={(header, value) => {
                  if (header === "yaml") return (
                    <details style={{ cursor: "pointer" }}>
                      <summary style={{ fontSize: "0.75rem", color: "#0f62fe" }}>View YAML</summary>
                      <CodeBlock code={value as string} language="yaml" maxLines={8} />
                    </details>
                  );
                  if (header === "entity_type") return <Tag type="blue">{value as string}</Tag>;
                  return value;
                }}
              />
            )}
          </TabPanel>
        </TabPanels>
      </Tabs>
    </>
  );
}
