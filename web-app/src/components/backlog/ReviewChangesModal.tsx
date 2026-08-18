"use client";
// +feature: backlog:review-changes-modal

import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import { DiffRenderer } from "@/components/shared/DiffRenderer";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { getErrorMessage } from "@/lib/utils/connectError";
import {
  backdrop,
  modal,
  modalHeader,
  modalTitle,
  modalLabel,
  closeButton,
  modalBody,
  openTerminalLink,
} from "./ReviewChangesModal.css";

interface ReviewChangesModalProps {
  itemId: string;
  sessionId?: string;
  sessionTitle?: string;
  onClose: () => void;
}

export function ReviewChangesModal({ itemId, sessionId, sessionTitle, onClose }: ReviewChangesModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  const [diff, setDiff] = useState<{ content: string; added: number; removed: number } | null>(null);
  const [loading, setLoading] = useState(true);
  // Distinct from a genuinely empty diff — a fetch failure must not render as
  // "No changes to display", which looks identical to real emptiness and hides
  // exactly the case a reviewer most needs to notice (see
  // docs/tasks/backlog-feature-improvement.md, Manual Gates section).
  const [fetchError, setFetchError] = useState<string | null>(null);

  const fetchDiff = useCallback(() => {
    setLoading(true);
    setFetchError(null);
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    const client = createClient(BacklogService, transport);

    client.getBacklogItemDiff({ itemId }).then((resp) => {
      setDiff({ content: resp.diff, added: resp.added, removed: resp.removed });
    }).catch((err: unknown) => {
      setFetchError(getErrorMessage(err, "Could not reach the server."));
    }).finally(() => setLoading(false));
  }, [itemId]);

  useEffect(() => {
    fetchDiff();
  }, [fetchDiff]);

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
        aria-labelledby="review-changes-title"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
      >
        <div className={modalHeader}>
          <span id="review-changes-title" className={modalTitle}>
            {sessionTitle ?? itemId}
          </span>
          <span className={modalLabel}>Changes</span>
          {sessionId && (
            <a
              className={openTerminalLink}
              href={`/?session=${sessionId}`}
              title="Open session in terminal"
            >
              Open in Terminal ↗
            </a>
          )}
          <button className={closeButton} onClick={onClose} aria-label="Close changes viewer">
            ✕
          </button>
        </div>
        <div className={modalBody}>
          <DiffRenderer
            content={diff?.content ?? ""}
            added={diff?.added ?? 0}
            removed={diff?.removed ?? 0}
            loading={loading}
            error={fetchError}
            onRefresh={fetchDiff}
          />
        </div>
      </div>
    </>,
    document.body,
  );
}
