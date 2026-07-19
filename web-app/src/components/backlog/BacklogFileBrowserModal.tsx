"use client";
// +feature: backlog:file-browser-modal

import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { FileTree } from "@/components/sessions/FileTree";
import { FileContentViewer } from "@/components/sessions/FileContentViewer";
import { getApiBaseUrl } from "@/lib/config";
import { useSessionVcs } from "@/lib/hooks/useSessionVcs";
import { buildGitStatusMap } from "@/lib/utils/gitStatus";
import {
  backdrop,
  modal,
  modalHeader,
  modalTitle,
  modalLabel,
  closeButton,
  modalBody,
  treePane,
  contentPane,
  openTerminalLink,
} from "./BacklogFileBrowserModal.css";

interface BacklogFileBrowserModalProps {
  sessionId: string;
  sessionTitle?: string;
  onClose: () => void;
}

/**
 * Lets a user click through a backlog work session's worktree files and
 * directories in-app, rather than only copying the worktree path. Embeds
 * FileTree + FileContentViewer directly (not FilesTab) since those two have
 * no dependency on SessionVcsContext/session tab-switching state — see
 * FileTree.tsx / FileContentViewer.tsx prop signatures.
 */
export function BacklogFileBrowserModal({ sessionId, sessionTitle, onClose }: BacklogFileBrowserModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const baseUrl = getApiBaseUrl();

  // Own lightweight VCS status fetch — this modal has no SessionVcsProvider ancestor.
  const { status } = useSessionVcs(sessionId, baseUrl);
  const gitStatusMap = useMemo(() => {
    if (!status) return new Map<string, string>();
    const { stagedFiles, unstagedFiles, untrackedFiles } = status;
    return buildGitStatusMap([...stagedFiles, ...unstagedFiles, ...untrackedFiles]);
  }, [status]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      }
    }
    window.addEventListener("keydown", handleKeyDown, { capture: true });
    return () => window.removeEventListener("keydown", handleKeyDown, { capture: true });
  }, [onClose]);

  useEffect(() => {
    modalRef.current?.focus();
  }, []);

  if (typeof document === "undefined") return null;

  return createPortal(
    <>
      <div className={backdrop} onClick={onClose} aria-hidden="true" />
      <div
        ref={modalRef}
        className={modal}
        role="dialog"
        aria-modal="true"
        aria-labelledby="file-browser-title"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
      >
        <div className={modalHeader}>
          <span id="file-browser-title" className={modalTitle}>
            {sessionTitle ?? sessionId}
          </span>
          <span className={modalLabel}>Files</span>
          <a
            className={openTerminalLink}
            href={`/?session=${sessionId}`}
            title="Open session in terminal"
          >
            Open in Terminal ↗
          </a>
          <button className={closeButton} onClick={onClose} aria-label="Close file browser">
            ✕
          </button>
        </div>
        <div className={modalBody}>
          <div className={treePane}>
            <FileTree
              sessionId={sessionId}
              baseUrl={baseUrl}
              selectedPath={selectedPath}
              onFileSelect={setSelectedPath}
              gitStatusMap={gitStatusMap}
            />
          </div>
          <div className={contentPane}>
            <FileContentViewer sessionId={sessionId} filePath={selectedPath} baseUrl={baseUrl} />
          </div>
        </div>
      </div>
    </>,
    document.body,
  );
}
