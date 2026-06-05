/**
 * Registers Carbon Design System themes for Apache ECharts.
 * Call once at module load in chart components (safe to call multiple times).
 */

let registered = false;

export function registerCarbonTheme(): void {
  if (registered || typeof window === "undefined") return;

  // Dynamic import avoids SSR issues with echarts
  import("echarts").then(({ registerTheme }) => {
    registerTheme("carbon-white", {
      backgroundColor: "#ffffff",
      textStyle: { color: "#161616", fontFamily: "IBM Plex Sans, sans-serif" },
      color: ["#0f62fe", "#da1e28", "#198038", "#f1c21b", "#8a3ffc", "#007d79", "#ff832b"],
      categoryAxis: {
        axisLine: { lineStyle: { color: "#e0e0e0" } },
        axisTick: { lineStyle: { color: "#e0e0e0" } },
        axisLabel: { color: "#525252" },
        splitLine: { lineStyle: { color: "#f4f4f4" } },
      },
      valueAxis: {
        axisLine: { lineStyle: { color: "#e0e0e0" } },
        axisTick: { lineStyle: { color: "#e0e0e0" } },
        axisLabel: { color: "#525252" },
        splitLine: { lineStyle: { color: "#f4f4f4" } },
      },
      legend: { textStyle: { color: "#161616" } },
      tooltip: {
        backgroundColor: "#ffffff",
        borderColor: "#e0e0e0",
        textStyle: { color: "#161616" },
      },
    });

    registerTheme("carbon-dark", {
      backgroundColor: "#161616",
      textStyle: { color: "#f4f4f4", fontFamily: "IBM Plex Sans, sans-serif" },
      color: ["#4589ff", "#ff8389", "#42be65", "#f1c21b", "#a56eff", "#3ddbd9", "#ff832b"],
      categoryAxis: {
        axisLine: { lineStyle: { color: "#393939" } },
        axisTick: { lineStyle: { color: "#393939" } },
        axisLabel: { color: "#c6c6c6" },
        splitLine: { lineStyle: { color: "#262626" } },
      },
      valueAxis: {
        axisLine: { lineStyle: { color: "#393939" } },
        axisTick: { lineStyle: { color: "#393939" } },
        axisLabel: { color: "#c6c6c6" },
        splitLine: { lineStyle: { color: "#262626" } },
      },
      legend: { textStyle: { color: "#f4f4f4" } },
      tooltip: {
        backgroundColor: "#262626",
        borderColor: "#393939",
        textStyle: { color: "#f4f4f4" },
      },
    });

    registered = true;
  });
}
