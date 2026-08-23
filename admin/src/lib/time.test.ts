import { describe, expect, it } from "vitest";
import { formatDateTime, formatDuration, formatRelativeTime } from "./time";

describe("formatRelativeTime", () => {
  const now = new Date("2026-08-23T12:00:00Z");

  it("returns a dash for empty or invalid values", () => {
    expect(formatRelativeTime(null, now)).toBe("—");
    expect(formatRelativeTime(undefined, now)).toBe("—");
    expect(formatRelativeTime("", now)).toBe("—");
    expect(formatRelativeTime("not-a-date", now)).toBe("—");
  });

  it("reports sub-minute differences as just now", () => {
    expect(formatRelativeTime("2026-08-23T11:59:30Z", now)).toBe("just now");
    expect(formatRelativeTime("2026-08-23T12:00:45Z", now)).toBe("just now");
  });

  it("formats past timestamps with the ago suffix", () => {
    expect(formatRelativeTime("2026-08-23T11:30:00Z", now)).toBe("30m ago");
    expect(formatRelativeTime("2026-08-23T06:00:00Z", now)).toBe("6h ago");
    expect(formatRelativeTime("2026-08-20T12:00:00Z", now)).toBe("3d ago");
    expect(formatRelativeTime("2026-08-02T12:00:00Z", now)).toBe("3w ago");
    expect(formatRelativeTime("2026-07-24T12:00:00Z", now)).toBe("1mo ago");
    expect(formatRelativeTime("2024-09-01T12:00:00Z", now)).toBe("1y ago");
  });

  it("formats future timestamps with an in prefix", () => {
    expect(formatRelativeTime("2026-08-23T13:00:00Z", now)).toBe("in 1h");
    expect(formatRelativeTime("2026-08-26T12:00:00Z", now)).toBe("in 3d");
  });
});

describe("formatDateTime", () => {
  it("falls back to a dash for empty or unparseable input", () => {
    expect(formatDateTime(null)).toBe("—");
    expect(formatDateTime("garbage")).toBe("—");
  });
});

describe("formatDuration", () => {
  it("returns null for absent, zero or invalid values", () => {
    expect(formatDuration(null)).toBeNull();
    expect(formatDuration(undefined)).toBeNull();
    expect(formatDuration(0)).toBeNull();
    expect(formatDuration(Number.NaN)).toBeNull();
  });

  it("prefers the largest clean unit", () => {
    expect(formatDuration(86400)).toBe("1d");
    expect(formatDuration(45 * 86400)).toBe("45d");
    expect(formatDuration(2592000)).toBe("1mo");
    expect(formatDuration(31536000)).toBe("1y");
    expect(formatDuration(730 * 86400)).toBe("2y");
    expect(formatDuration(3600)).toBe("1h");
    expect(formatDuration(7200)).toBe("2h");
    expect(formatDuration(90 * 60)).toBe("90m");
    expect(formatDuration(45)).toBe("45s");
  });
});
