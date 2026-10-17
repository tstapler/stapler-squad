"use client";

import { useCallback, useMemo, useState } from "react";
import type { LeftPane } from "@/components/sessions/files/SearchPanel";
import type { SearchHit } from "@/lib/files/searchClient";
import type { FilesV2 } from "./useFilesV2";

interface Callbacks {
  expandPanel: () => void;
  showTreePane: () => void;
  showContentPane: () => void;
}

/** Which left view shows (Explorer or Search), and the glue between a search hit and the buffers. */
export function useSearchPane(v2: FilesV2 | undefined, { expandPanel, showTreePane, showContentPane }: Callbacks) {
  const [leftPane, setLeftPane] = useState<LeftPane>("explorer");
  const [focusToken, setFocusToken] = useState(0);

  const chooseLeftPane = useCallback((pane: LeftPane) => {
    setLeftPane(pane);
    if (pane === "search") setFocusToken((n) => n + 1);
  }, []);
  const openSearch = useCallback(() => {
    expandPanel();
    showTreePane();
    chooseLeftPane("search");
  }, [expandPanel, showTreePane, chooseLeftPane]);

  const buffers = v2?.buffers.state.buffers;
  const jumpTo = v2?.bridge.jumpTo;
  const openHit = useCallback((hit: SearchHit) => {
    if (!jumpTo) return false;
    const reused = !!buffers?.some((b) => b.path === hit.path);
    jumpTo(hit.path, hit.line, { source: "search" });
    showContentPane();
    return reused;
  }, [jumpTo, buffers, showContentPane]);

  const dirtyPaths = useMemo(
    () => new Set((buffers ?? []).flatMap((b) => (b.status === "loaded" && b.dirty ? [b.path] : []))),
    [buffers],
  );
  return { leftPane, chooseLeftPane, focusToken, openSearch, openHit, dirtyPaths };
}
