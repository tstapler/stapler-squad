"use client";

import { createPortal } from "react-dom";
import { zIndex } from "@/styles/theme.css";

interface PathConfirmDialogProps {
  path: string;
  onCancel: () => void;
  onConfirm: () => void;
}

// Directory mode with a non-existent path: confirm before creating it and running
// `git init`. Portaled to document.body, so it competes with the Omnibar overlay
// (zIndex.modal) at page level and must sit above it to be clickable.
export function PathConfirmDialog({
  path,
  onCancel,
  onConfirm,
}: PathConfirmDialogProps) {
  return createPortal(
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="path-confirm-title"
      data-testid="path-confirm-dialog"
      style={{
        position: "fixed",
        inset: 0,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "rgba(0,0,0,0.5)",
        zIndex: zIndex.dialog,
        borderRadius: "inherit",
      }}
      onClick={(e) => e.stopPropagation()}
    >
      <div
        style={{
          background: "var(--card-background)",
          border: "1px solid var(--border-color)",
          borderRadius: "8px",
          padding: "24px",
          maxWidth: "420px",
          width: "100%",
          margin: "16px",
        }}
      >
        <div
          id="path-confirm-title"
          style={{ fontWeight: 600, fontSize: "1rem", marginBottom: "8px" }}
        >
          Create directory?
        </div>
        <div
          style={{
            fontSize: "0.875rem",
            color: "var(--text-secondary)",
            marginBottom: "16px",
          }}
        >
          The path{" "}
          <code style={{ fontFamily: "monospace", padding: "0 4px" }}>
            {path}
          </code>{" "}
          does not exist. Create it and initialize a git repository?
        </div>
        <div
          style={{ display: "flex", gap: "8px", justifyContent: "flex-end" }}
        >
          <button
            type="button"
            style={{
              padding: "6px 14px",
              fontSize: "0.875rem",
              borderRadius: "6px",
              border: "1px solid var(--border-color)",
              background: "transparent",
              color: "var(--text-primary)",
              cursor: "pointer",
            }}
            onClick={onCancel}
          >
            Cancel
          </button>
          <button
            type="button"
            style={{
              padding: "6px 14px",
              fontSize: "0.875rem",
              borderRadius: "6px",
              border: "none",
              background: "var(--primary)",
              color: "var(--primary-text)",
              cursor: "pointer",
            }}
            onClick={onConfirm}
          >
            Create &amp; Open
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
