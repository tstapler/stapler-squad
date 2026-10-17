"use client";

import { useEffect, type RefObject } from "react";
import type { FileTreeHandle } from "@/components/sessions/FileTree";

/** Reveals a cross-linked path (e.g. from the VCS panel) in the file tree whenever it changes. */
export function useFilesDeepLink(path: string | null | undefined, tree: RefObject<FileTreeHandle | null>): void {
  useEffect(() => {
    if (path) tree.current?.revealPath(path);
  }, [path, tree]);
}
