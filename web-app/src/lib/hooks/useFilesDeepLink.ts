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
  /**
   * Whether this Files tab owns the URL hash: it is the visible tab of the focused pane. The hash is
   * global, so an inactive tab must not write it, and releases it (when still ours) on deactivation.
   */
  active?: boolean;
}

/**
 * The only code that writes `#file=` or touches history for the Files tab. Selection changes use
 * `replaceState(history.state, ...)` so a pushed pane entry survives.
 */
export function useFilesDeepLink({ initialSelectedPath, tree, selectedPath, onJump, active = true }: Options): void {
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
  const written = useRef<string | null>(null);
  useEffect(() => {
    if (!active) return;
    const hash = fileHash(selectedPath);
    // Skip the mount pass when nothing is selected, so an unrelated hash is not erased.
    if (first.current && !selectedPath) {
      first.current = false;
      return;
    }
    first.current = false;
    written.current = hash || null;
    if (window.location.hash === hash) return;
    replaceHash(hash);
  }, [selectedPath, active]);

  // Release the hash on tab switch, pane blur, or unmount so it never outlives the viewer.
  useEffect(() => {
    if (!active) return;
    return () => {
      if (written.current && window.location.hash === written.current) replaceHash("");
      written.current = null;
    };
  }, [active]);
}
