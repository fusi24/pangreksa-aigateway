"use client";

import { ContentSwitcher, Switch } from "@carbon/react";
import { useUIStore } from "@/store/ui";
import type { TimeRange } from "@/store/ui";

const OPTIONS: Array<{ key: TimeRange; text: string }> = [
  { key: "1h", text: "1h" },
  { key: "6h", text: "6h" },
  { key: "24h", text: "24h" },
  { key: "7d", text: "7d" },
  { key: "30d", text: "30d" },
];

/**
 * Time-range selector that writes to the Zustand store,
 * triggering re-queries in all useMetrics() hooks.
 */
export function TimeRangeSelector() {
  const { timeRange, setTimeRange } = useUIStore();

  return (
    <ContentSwitcher
      selectedIndex={OPTIONS.findIndex((o) => o.key === timeRange)}
      onChange={({ index }) => {
        if (typeof index === "number") {
          const option = OPTIONS[index];
          if (option) setTimeRange(option.key);
        }
      }}
      size="sm"
    >
      {OPTIONS.map((o) => (
        <Switch key={o.key} name={o.key} text={o.text} />
      ))}
    </ContentSwitcher>
  );
}
