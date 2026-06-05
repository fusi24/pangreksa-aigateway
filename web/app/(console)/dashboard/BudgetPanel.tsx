"use client";

import { Column, Grid, Tile, Tag, InlineNotification, SkeletonPlaceholder } from "@carbon/react";
import { BudgetGauge } from "@/components/charts/BudgetGauge";
import { useBudget } from "@/hooks/useBudget";
import { fmtUsd } from "@/lib/utils/format";

/**
 * Budget consumption panel showing all active budget rules
 * with gauge charts and status indicators.
 */
export function BudgetPanel() {
  const { data, isLoading, error } = useBudget();

  if (error) {
    return (
      <InlineNotification
        kind="error"
        title="Failed to load budget data"
        subtitle={(error as Error).message}
      />
    );
  }

  if (isLoading) {
    return (
      <Grid>
        {[0, 1, 2].map((i) => (
          <Column key={i} sm={4} md={2} lg={4}>
            <div style={{ width: "100%", height: 200 }}><SkeletonPlaceholder /></div>
          </Column>
        ))}
      </Grid>
    );
  }

  const rules = data?.rules ?? [];

  if (rules.length === 0) {
    return (
      <Tile>
        <p style={{ color: "#525252" }}>No active budget rules.</p>
      </Tile>
    );
  }

  return (
    <div>
      <h3 style={{ marginBottom: "1rem" }}>Budget Consumption</h3>
      <Grid>
        {rules.map((rule) => (
          <Column key={rule.id} sm={4} md={2} lg={4}>
            <Tile style={{ textAlign: "center" }}>
              <BudgetGauge
                ruleName={rule.name}
                spendUsd={rule.spend_usd}
                limitUsd={rule.limit_usd}
              />
              <div style={{ marginTop: "0.5rem" }}>
                <Tag
                  type={
                    rule.status === "CRITICAL" ? "red"
                    : rule.status === "WARNING" ? "warm-gray"
                    : "green"
                  }
                >
                  {rule.status}
                </Tag>
                <div style={{ fontSize: "0.75rem", color: "#525252", marginTop: "0.25rem" }}>
                  {fmtUsd(rule.spend_usd)} / {fmtUsd(rule.limit_usd)} ({rule.period})
                </div>
              </div>
            </Tile>
          </Column>
        ))}
      </Grid>
    </div>
  );
}
