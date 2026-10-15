"use client";

import { useEffect, useState } from "react";

const QUERY = "(pointer: coarse)";

/** True while the primary input is a finger. False where matchMedia is unavailable (SSR, tests). */
export function useCoarsePointer(): boolean {
  const [coarse, setCoarse] = useState(() =>
    typeof window !== "undefined" && typeof window.matchMedia === "function" ? window.matchMedia(QUERY).matches : false,
  );

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const mq = window.matchMedia(QUERY);
    const update = () => setCoarse(mq.matches);
    update();
    mq.addEventListener("change", update);
    return () => mq.removeEventListener("change", update);
  }, []);

  return coarse;
}
