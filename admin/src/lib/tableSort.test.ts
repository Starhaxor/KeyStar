import { describe, expect, it } from "vitest";
import { compareValues, toggleSortState } from "./tableSort";

describe("compareValues", () => {
  it("compares numbers numerically", () => {
    expect(compareValues(2, 10)).toBeLessThan(0);
    expect(compareValues(10, 2)).toBeGreaterThan(0);
    expect(compareValues(3, 3)).toBe(0);
  });

  it("compares strings case-insensitively", () => {
    expect(compareValues("apple", "Banana")).toBeLessThan(0);
    expect(compareValues("Zebra", "apple")).toBeGreaterThan(0);
  });

  it("compares ISO date strings chronologically", () => {
    expect(
      compareValues("2026-08-23T10:00:00Z", "2026-08-23T12:00:00Z")
    ).toBeLessThan(0);
  });

  it("sorts missing values before present ones", () => {
    expect(compareValues(null, 1)).toBeLessThan(0);
    expect(compareValues(undefined, "x")).toBeLessThan(0);
    expect(compareValues("x", null)).toBeGreaterThan(0);
    expect(compareValues(null, undefined)).toBe(0);
  });
});

describe("toggleSortState", () => {
  it("starts ascending for a new column", () => {
    expect(toggleSortState(null, "email")).toEqual({
      key: "email",
      direction: "asc",
    });
  });

  it("flips direction for the same column", () => {
    expect(
      toggleSortState({ key: "email", direction: "asc" }, "email")
    ).toEqual({ key: "email", direction: "desc" });
    expect(
      toggleSortState({ key: "email", direction: "desc" }, "email")
    ).toEqual({ key: "email", direction: "asc" });
  });

  it("resets to ascending when switching columns", () => {
    expect(
      toggleSortState({ key: "status", direction: "desc" }, "email")
    ).toEqual({ key: "email", direction: "asc" });
  });
});
