"use client";

import { Suspense } from "react";
import { Grid, Column } from "@carbon/react";
import { DashboardCharts } from "./DashboardCharts";
import { BudgetPanel } from "./BudgetPanel";
import { TimeRangeSelector } from "./TimeRangeSelector";
import { DashboardSkeleton } from "./DashboardSkeleton";

/**
 * Observability Dashboard — RSC wrapper.
 * Chart rendering is delegated to client components.
 */
export default function DashboardPage() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "1.5rem" }}>
        <h1 style={{ fontSize: "1.75rem", fontWeight: 600 }}>Observability Dashboard</h1>
        <TimeRangeSelector />
      </div>

      <Suspense fallback={<DashboardSkeleton />}>
        <Grid>
          <DashboardCharts />
        </Grid>
        <div style={{ marginTop: "2rem" }}>
          <BudgetPanel />
        </div>
      </Suspense>
    </div>
  );
}
