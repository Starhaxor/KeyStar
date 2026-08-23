"use client";

import { useCallback, useMemo, useState } from "react";
import {
  compareValues,
  toggleSortState,
  type SortState,
} from "@/lib/tableSort";

// Client-side sorting for the rows currently rendered by a table. Accessors
// map each sortable column to a comparable value; the accessor map is part
// of the memo dependencies so updated closures are always picked up.
export function useTableSort<
  T,
  K extends string,
>(
  rows: T[],
  accessors: Record<K, (row: T) => string | number | null | undefined>,
  initial: SortState<K> = null
) {
  const [sort, setSort] = useState<SortState<K>>(initial);

  const toggleSort = useCallback((key: K) => {
    setSort((prev) => toggleSortState(prev, key));
  }, []);

  const sorted = useMemo(() => {
    if (!sort) return rows;
    const accessor = accessors[sort.key];
    if (!accessor) return rows;
    const copy = [...rows];
    copy.sort((a, b) => {
      const result = compareValues(accessor(a), accessor(b));
      return sort.direction === "asc" ? result : -result;
    });
    return copy;
  }, [rows, sort, accessors]);

  return { sorted, sort, toggleSort };
}
