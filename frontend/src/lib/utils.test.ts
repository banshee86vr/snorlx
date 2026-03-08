import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  cn,
  formatDate,
  formatDateTime,
  formatRelativeTime,
  formatDuration,
  getStatusColor,
  getConclusionColor,
  truncate,
  capitalizeFirst,
} from "./utils";

// ===== cn (class merging) =====

describe("cn", () => {
  it("merges class strings", () => {
    expect(cn("foo", "bar")).toBe("foo bar");
  });

  it("deduplicates Tailwind classes (last wins)", () => {
    const result = cn("text-red-500", "text-blue-500");
    expect(result).toBe("text-blue-500");
  });

  it("handles conditional classes", () => {
    const active = true;
    const result = cn("base", active && "active");
    expect(result).toBe("base active");
  });

  it("handles falsy conditionals", () => {
    const result = cn("base", false && "never");
    expect(result).toBe("base");
  });

  it("handles empty inputs", () => {
    expect(cn()).toBe("");
  });
});

// ===== formatDate =====

describe("formatDate", () => {
  it("returns Never for null", () => {
    expect(formatDate(null)).toBe("Never");
  });

  it("formats a date string", () => {
    const result = formatDate("2024-06-15T10:00:00Z");
    expect(result).toMatch(/Jun/);
    expect(result).toMatch(/2024/);
    expect(result).toMatch(/15/);
  });

  it("returns a non-empty string for a valid date", () => {
    expect(formatDate("2023-01-01T00:00:00Z")).toBeTruthy();
  });
});

// ===== formatDateTime =====

describe("formatDateTime", () => {
  it("returns Never for null", () => {
    expect(formatDateTime(null)).toBe("Never");
  });

  it("formats a datetime string with time", () => {
    const result = formatDateTime("2024-06-15T14:30:00Z");
    expect(result).toMatch(/Jun/);
    expect(result).toMatch(/2024/);
  });

  it("returns a non-empty string for a valid datetime", () => {
    expect(formatDateTime("2023-12-31T23:59:59Z")).toBeTruthy();
  });
});

// ===== formatRelativeTime =====

describe("formatRelativeTime", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns 'just now' for very recent dates", () => {
    const now = new Date("2024-06-15T12:00:00Z");
    vi.setSystemTime(now);
    const result = formatRelativeTime("2024-06-15T12:00:30Z"); // 30s ago
    expect(result).toBe("just now");
  });

  it("returns minutes ago for dates within the last hour", () => {
    const now = new Date("2024-06-15T12:30:00Z");
    vi.setSystemTime(now);
    const result = formatRelativeTime("2024-06-15T12:00:00Z"); // 30 min ago
    expect(result).toBe("30m ago");
  });

  it("returns hours ago for dates within the last day", () => {
    const now = new Date("2024-06-15T16:00:00Z");
    vi.setSystemTime(now);
    const result = formatRelativeTime("2024-06-15T12:00:00Z"); // 4h ago
    expect(result).toBe("4h ago");
  });

  it("returns days ago for dates within the last week", () => {
    const now = new Date("2024-06-15T12:00:00Z");
    vi.setSystemTime(now);
    const result = formatRelativeTime("2024-06-12T12:00:00Z"); // 3d ago
    expect(result).toBe("3d ago");
  });

  it("falls back to formatted date for dates older than a week", () => {
    const now = new Date("2024-06-15T12:00:00Z");
    vi.setSystemTime(now);
    const result = formatRelativeTime("2024-01-01T00:00:00Z");
    expect(result).toMatch(/Jan/);
    expect(result).toMatch(/2024/);
  });
});

// ===== formatDuration =====

describe("formatDuration", () => {
  it("returns '-' for null", () => {
    expect(formatDuration(null)).toBe("-");
  });

  it("returns '-' for undefined", () => {
    expect(formatDuration(undefined)).toBe("-");
  });

  it("returns '-' for 0", () => {
    expect(formatDuration(0)).toBe("-");
  });

  it("formats seconds only", () => {
    expect(formatDuration(45)).toBe("45s");
  });

  it("formats minutes without remainder seconds", () => {
    expect(formatDuration(120)).toBe("2m");
  });

  it("formats minutes with remainder seconds", () => {
    expect(formatDuration(135)).toBe("2m 15s");
  });

  it("formats exactly 1 minute", () => {
    expect(formatDuration(60)).toBe("1m");
  });

  it("formats hours without remainder minutes", () => {
    expect(formatDuration(7200)).toBe("2h");
  });

  it("formats hours with remainder minutes", () => {
    expect(formatDuration(7500)).toBe("2h 5m");
  });

  it("formats exactly 1 hour", () => {
    expect(formatDuration(3600)).toBe("1h");
  });

  it("formats 1 second", () => {
    expect(formatDuration(1)).toBe("1s");
  });

  it("formats 59 seconds", () => {
    expect(formatDuration(59)).toBe("59s");
  });
});

// ===== getStatusColor =====

describe("getStatusColor", () => {
  it("returns badge-success for success", () => {
    expect(getStatusColor("success")).toBe("badge-success");
  });

  it("returns badge-success for completed", () => {
    expect(getStatusColor("completed")).toBe("badge-success");
  });

  it("returns badge-danger for failure", () => {
    expect(getStatusColor("failure")).toBe("badge-danger");
  });

  it("returns badge-danger for failed", () => {
    expect(getStatusColor("failed")).toBe("badge-danger");
  });

  it("returns badge-info for in_progress", () => {
    expect(getStatusColor("in_progress")).toBe("badge-info");
  });

  it("returns badge-info for running", () => {
    expect(getStatusColor("running")).toBe("badge-info");
  });

  it("returns badge-warning for queued", () => {
    expect(getStatusColor("queued")).toBe("badge-warning");
  });

  it("returns badge-warning for pending", () => {
    expect(getStatusColor("pending")).toBe("badge-warning");
  });

  it("returns badge-warning for waiting", () => {
    expect(getStatusColor("waiting")).toBe("badge-warning");
  });

  it("returns badge-neutral for cancelled", () => {
    expect(getStatusColor("cancelled")).toBe("badge-neutral");
  });

  it("returns badge-neutral for skipped", () => {
    expect(getStatusColor("skipped")).toBe("badge-neutral");
  });

  it("returns badge-neutral for unknown status", () => {
    expect(getStatusColor("unknown")).toBe("badge-neutral");
  });

  it("is case-insensitive", () => {
    expect(getStatusColor("SUCCESS")).toBe("badge-success");
    expect(getStatusColor("FAILURE")).toBe("badge-danger");
    expect(getStatusColor("IN_PROGRESS")).toBe("badge-info");
  });
});

// ===== getConclusionColor =====

describe("getConclusionColor", () => {
  it("returns badge-neutral for null", () => {
    expect(getConclusionColor(null)).toBe("badge-neutral");
  });

  it("returns badge-success for success", () => {
    expect(getConclusionColor("success")).toBe("badge-success");
  });

  it("returns badge-danger for failure", () => {
    expect(getConclusionColor("failure")).toBe("badge-danger");
  });

  it("returns badge-neutral for cancelled", () => {
    expect(getConclusionColor("cancelled")).toBe("badge-neutral");
  });

  it("returns badge-neutral for skipped", () => {
    expect(getConclusionColor("skipped")).toBe("badge-neutral");
  });

  it("returns badge-warning for timed_out", () => {
    expect(getConclusionColor("timed_out")).toBe("badge-warning");
  });

  it("returns badge-neutral for unknown conclusion", () => {
    expect(getConclusionColor("unknown")).toBe("badge-neutral");
  });

  it("is case-insensitive", () => {
    expect(getConclusionColor("SUCCESS")).toBe("badge-success");
    expect(getConclusionColor("FAILURE")).toBe("badge-danger");
  });
});

// ===== truncate =====

describe("truncate", () => {
  it("returns string unchanged if within limit", () => {
    expect(truncate("hello", 10)).toBe("hello");
  });

  it("returns string unchanged if exactly at limit", () => {
    expect(truncate("hello", 5)).toBe("hello");
  });

  it("truncates and appends ellipsis when over limit", () => {
    expect(truncate("hello world", 5)).toBe("hello...");
  });

  it("truncates long strings", () => {
    const long = "a".repeat(200);
    const result = truncate(long, 50);
    expect(result).toBe("a".repeat(50) + "...");
  });

  it("handles empty string", () => {
    expect(truncate("", 5)).toBe("");
  });

  it("handles limit of 0", () => {
    expect(truncate("hello", 0)).toBe("...");
  });
});

// ===== capitalizeFirst =====

describe("capitalizeFirst", () => {
  it("capitalizes first letter and lowercases the rest", () => {
    expect(capitalizeFirst("hello")).toBe("Hello");
  });

  it("lowercases all but first", () => {
    expect(capitalizeFirst("hELLO WORLD")).toBe("Hello world");
  });

  it("handles already capitalized string", () => {
    expect(capitalizeFirst("Hello")).toBe("Hello");
  });

  it("handles single character", () => {
    expect(capitalizeFirst("a")).toBe("A");
  });

  it("handles empty string", () => {
    expect(capitalizeFirst("")).toBe("");
  });

  it("handles all caps", () => {
    expect(capitalizeFirst("SUCCESS")).toBe("Success");
  });
});
