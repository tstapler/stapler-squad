"use client";

import { useEffect, useRef, type RefObject } from "react";
import type { FileTreeHandle } from "@/components/sessions/FileTree";
import { fileHash, parseDeepLink } from "@/lib/files/deepLink";
import { replaceHash } from "@/lib/nav";

interface Options {
  /** Cross-linked path (e.g. from the VCS panel); revealed in the tree whenever it changes. */
  initialSelectedPath: string | null | undefined;
  tree: RefObject<FileTreeHandle | null>;
  selectedPath: string | null;
  /** Opens `?file=&line=` pinned at the line; absent when the viewer is not buffered. */
  onJump?: (path: string, line: number | undefined) => void;
}

/**
 * The only code that writes `#file=` or touches history for the Files tab. Selection changes use
 * `replaceState(history.state, ...)` so a pushed pane entry survives.
 */
export function useFilesDeepLink({ initialSelectedPath, tree, selectedPath, onJump }: Options): void {
  useEffect(() => {
    if (initialSelectedPath) tree.current?.revealPath(initialSelectedPath);
  }, [initialSelectedPath, tree]);

  const jumped = useRef(false);
  useEffect(() => {
    if (jumped.current || !onJump) return;
    jumped.current = true;
    const link = parseDeepLink(window.location.search, "");
    if (link) onJump(link.file, link.line);
  }, [onJump]);

  const first = useRef(true);
  useEffect(() => {
    const hash = fileHash(selectedPath);
    // Skip the mount pass when nothing is selected, so an unrelated hash is not erased.
    if (first.current && !selectedPath) {
      first.current = false;
      return;
    }
    first.current = false;
    if (window.location.hash === hash) return;
    replaceHash(hash);
  }, [selectedPath]);
}
