import { describe, it, expect } from "vitest";
import { fmtUsd, fmtMs, fmtTokens, fmtUptime, truncate } from "@/lib/utils/format";

describe("fmtUsd", () => {
  it("formats cost to 4 decimal places", () => {
    expect(fmtUsd(0.00384)).toBe("$0.0038");
    expect(fmtUsd(1.5)).toBe("$1.5000");
    expect(fmtUsd(0)).toBe("$0.0000");
  });
});

describe("fmtMs", () => {
  it("formats milliseconds with thousands separator", () => {
    expect(fmtMs(842)).toBe("842ms");
    expect(fmtMs(1234)).toContain("1");
    expect(fmtMs(1234)).toContain("ms");
  });
});

describe("fmtTokens", () => {
  it("formats small numbers as-is", () => {
    expect(fmtTokens(512)).toBe("512");
  });

  it("formats thousands with K suffix", () => {
    expect(fmtTokens(1500)).toBe("1.5K");
    expect(fmtTokens(10000)).toBe("10.0K");
  });

  it("formats millions with M suffix", () => {
    expect(fmtTokens(1_500_000)).toBe("1.50M");
    expect(fmtTokens(2_200_000)).toBe("2.20M");
  });

  it("handles zero", () => {
    expect(fmtTokens(0)).toBe("0");
  });
});

describe("fmtUptime", () => {
  it("formats seconds into human-readable string", () => {
    expect(fmtUptime(60)).toBe("1m");
    expect(fmtUptime(3600)).toBe("1h");
    expect(fmtUptime(86400)).toBe("1d");
    expect(fmtUptime(90061)).toBe("1d 1h 1m");
  });

  it("handles zero seconds", () => {
    expect(fmtUptime(0)).toBe("0m");
  });
});

describe("truncate", () => {
  it("returns string unchanged if within limit", () => {
    expect(truncate("hello", 10)).toBe("hello");
    expect(truncate("hello", 5)).toBe("hello");
  });

  it("appends ellipsis when truncated", () => {
    expect(truncate("hello world", 5)).toBe("hello…");
  });
});
