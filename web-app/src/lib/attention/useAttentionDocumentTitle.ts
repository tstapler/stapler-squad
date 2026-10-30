"use client";

import { useEffect } from "react";
import { useAttentionCount } from "./useAttentionCount";

const COUNT_PREFIX = /^\(\d+\)\s+/;

/**
 * Prefixes document.title with "(N) " for the needs-human count. Inactive while the
 * prose-question flag is off, so the legacy title is untouched. A MutationObserver re-applies
 * the prefix after a route change rewrites the title.
 */
export function useAttentionDocumentTitle(): void {
  const { count, needsHumanOnly } = useAttentionCount();

  useEffect(() => {
    if (!needsHumanOnly) return;
    const apply = () => {
      const base = document.title.replace(COUNT_PREFIX, "");
      const next = count > 0 ? `(${count}) ${base}` : base;
      if (document.title !== next) document.title = next;
    };
    apply();
    const titleEl = document.querySelector("title");
    if (!titleEl) return;
    const observer = new MutationObserver(apply);
    observer.observe(titleEl, { childList: true, characterData: true, subtree: true });
    return () => {
      observer.disconnect();
      document.title = document.title.replace(COUNT_PREFIX, "");
    };
  }, [count, needsHumanOnly]);
}
