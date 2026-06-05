"use client";

import { Grid, Column, SkeletonPlaceholder, SkeletonText } from "@carbon/react";

/**
 * Loading skeleton matching the Dashboard layout structure.
 */
export function DashboardSkeleton() {
  return (
    <Grid>
      {/* KPI row */}
      {[0, 1, 2, 3].map((i) => (
        <Column key={i} sm={2} md={2} lg={3}>
          <div style={{ padding: "1rem", background: "#f4f4f4", borderRadius: 4, marginBottom: "1rem" }}>
            <SkeletonText heading />
            <SkeletonText width="60%" />
          </div>
        </Column>
      ))}
      {/* Chart row 1 */}
      {[0, 1].map((i) => (
        <Column key={`c${i}`} sm={4} md={4} lg={8}>
          <div style={{ width: "100%", height: 300, marginBottom: "1rem" }}><SkeletonPlaceholder /></div>
        </Column>
      ))}
      {/* Chart row 2 */}
      {[0, 1].map((i) => (
        <Column key={`d${i}`} sm={4} md={4} lg={8}>
          <div style={{ width: "100%", height: 300, marginBottom: "1rem" }}><SkeletonPlaceholder /></div>
        </Column>
      ))}
    </Grid>
  );
}
