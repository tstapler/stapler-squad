"use client";

import { useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";

type FilterValue = string | undefined;

export function useFilterState<K extends string>(keys: readonly K[]) {
  const searchParams = useSearchParams();
  const router = useRouter();

  const filterState = Object.fromEntries(
    keys.map((k) => [k, searchParams.get(k) ?? undefined])
  ) as Record<K, FilterValue>;

  const setFilter = useCallback(
    (key: K, value: FilterValue) => {
      const params = new URLSearchParams(searchParams.toString());
      if (value === undefined || value === "") {
        params.delete(key);
      } else {
        params.set(key, value);
      }
      router.replace(`?${params.toString()}`, { scroll: false });
    },
    [searchParams, router]
  );

  const clearFilters = useCallback(() => {
    const params = new URLSearchParams(searchParams.toString());
    keys.forEach((k) => params.delete(k));
    router.replace(`?${params.toString()}`, { scroll: false });
  }, [keys, searchParams, router]);

  return { filterState, setFilter, clearFilters };
}
