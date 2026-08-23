// Pure sorting primitives shared by console tables. Kept free of React so
// the comparator rules are unit-testable in isolation.

export type SortDirection = "asc" | "desc";

export type SortState<K extends string> = {
  key: K;
  direction: SortDirection;
} | null;

export function compareValues(a: unknown, b: unknown): number {
  // Missing values sort to the bottom regardless of direction intent,
  // mirroring how database NULL ordering behaves in admin tools.
  if (a == null && b == null) return 0;
  if (a == null) return -1;
  if (b == null) return 1;
  if (typeof a === "number" && typeof b === "number") return a - b;

  const timeA = parseTimestamp(a);
  const timeB = parseTimestamp(b);
  if (timeA !== null && timeB !== null) return timeA - timeB;

  const stringA = String(a).trim().toLowerCase();
  const stringB = String(b).trim().toLowerCase();
  if (stringA < stringB) return -1;
  if (stringA > stringB) return 1;
  return 0;
}

function parseTimestamp(value: unknown): number | null {
  if (typeof value !== "string") return null;
  const parsed = new Date(value).getTime();
  return Number.isNaN(parsed) ? null : parsed;
}

export function toggleSortState<K extends string>(
  current: SortState<K>,
  key: K
): SortState<K> {
  if (current && current.key === key) {
    return {
      key,
      direction: current.direction === "asc" ? "desc" : "asc",
    };
  }
  return { key, direction: "asc" };
}
