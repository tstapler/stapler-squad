"use client";

import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionDetail } from "./SessionDetail";
import { backdrop, modal, modalHeader, modalTitle, peekBadge, closeButton, modalBody } from "./SessionPeekModal.css";

interface SessionPeekModalProps {
  session: Session;
  onClose: () => void;
}

export function SessionPeekModal({ session, onClose }: SessionPeekModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);

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
        aria-labelledby="peek-modal-title"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
      >
        <div className={modalHeader}>
          <span id="peek-modal-title" className={modalTitle}>{session.title}</span>
          <span className={peekBadge}>Peek</span>
          <button
            className={closeButton}
            onClick={onClose}
            aria-label="Close peek modal"
          >
            ✕
          </button>
        </div>
        <div className={modalBody}>
          <SessionDetail
            session={session}
            onClose={onClose}
            embedded={true}
            initialTab="terminal"
          />
        </div>
      </div>
    </>,
    document.body
  );
}
