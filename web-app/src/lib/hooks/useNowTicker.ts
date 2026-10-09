"use client";

import { useEffect, useState } from "react";

/** Returns `Date.now()`, re-read every `intervalMs`. One call per panel = one shared ticker. */
export function useNowTicker(intervalMs = 30_000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    setNow(Date.now());
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}
