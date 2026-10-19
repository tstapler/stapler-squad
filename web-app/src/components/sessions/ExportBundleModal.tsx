"use client";

// +feature: ui:bundle-export

import { useState } from "react";
import Link from "next/link";
import { Modal, ModalContent, ModalTitle, ModalDescription, ModalFooter } from "@/components/ui/Modal";
import { useExportBundle } from "@/lib/hooks/useExportBundle";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import type { SaveSessionBundleResponse } from "@/gen/session/v1/rum_bundle_pb";
import * as styles from "./ExportBundleModal.css";

export const CONSENT_TEXT =
  "This bundle may contain raw terminal output including typed credentials. Do not share it with untrusted parties.";

type DeliveryMode = "download" | "save";

export interface ExportBundleModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sessionId: string;
  sessionTitle: string;
  repoPath?: string;
}

function ymd(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}`;
}

function basename(path: string): string {
  return path.split("/").pop() ?? path;
}

export function ExportBundleModal({ open, onOpenChange, sessionId, sessionTitle, repoPath }: ExportBundleModalProps) {
  const { download, saveOnServer, loading, error } = useExportBundle();
  const { createBacklogItem } = useBacklogService();
  const { showActionToast } = useNotifications();
  const [mode, setMode] = useState<DeliveryMode>(() =>
    typeof window !== "undefined" && window.innerWidth < 640 ? "save" : "download"
  );
  const [consented, setConsented] = useState(false);
  const [saved, setSaved] = useState<SaveSessionBundleResponse | null>(null);
  const [backlogItemId, setBacklogItemId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [failed, setFailed] = useState(false);
  const toastKey = `bundle-export-${sessionId}`;

  const handleExport = async () => {
    setFailed(false);
    setSaved(null);
    setBacklogItemId(null);
    try {
      if (mode === "download") {
        const ok = await download(sessionId, sessionTitle);
        if (ok) showActionToast("Bundle downloaded.", "success", toastKey);
        else throw new Error("Export failed");
      } else {
        setSaved(await saveOnServer(sessionId));
        showActionToast("Bundle saved to server.", "success", toastKey);
      }
    } catch {
      setFailed(true);
      showActionToast("Bundle export failed.", "error", toastKey);
    }
  };

  const handleCreateBacklogItem = async () => {
    if (!saved) return;
    setCreating(true);
    const filename = basename(saved.filePath);
    let result: Awaited<ReturnType<typeof createBacklogItem>> = null;
    try {
      result = await createBacklogItem({
        title: `Diagnostic bundle: ${sessionTitle} (${ymd(new Date())})`,
        notes: `Bundle: ${filename} (SHA-256: ${saved.contentHash})`,
        repoPath,
      });
    } finally {
      setCreating(false);
    }
    if (result) {
      setBacklogItemId(result.item.id);
      showActionToast("Backlog item created.", "success", toastKey);
    } else {
      showActionToast("Failed to create backlog item.", "error", toastKey);
    }
  };

  return (
    <Modal open={open} onOpenChange={onOpenChange}>
      <ModalContent data-testid="export-bundle-modal">
        <ModalTitle>Export Diagnostics</ModalTitle>
        <ModalDescription>Package this session&apos;s traces, profiles and state into one zip.</ModalDescription>
        <fieldset className={styles.radioGroup}>
          <legend>Delivery</legend>
          <label className={styles.radioLabel}>
            <input type="radio" name="bundle-mode" checked={mode === "download"} onChange={() => setMode("download")} />
            Download .zip
          </label>
          <label className={styles.radioLabel}>
            <input type="radio" name="bundle-mode" checked={mode === "save"} onChange={() => setMode("save")} />
            Save on server
          </label>
        </fieldset>
        <label className={styles.consent}>
          <input
            type="checkbox"
            aria-required="true"
            checked={consented}
            onChange={(e) => setConsented(e.target.checked)}
            data-testid="export-bundle-consent"
          />
          <span>{CONSENT_TEXT}</span>
        </label>
        <div role="status" aria-live="polite">{loading ? "Assembling bundle…" : ""}</div>
        {(failed || error) && (
          <div role="alert" className={styles.errorBanner}>
            <span>{error ?? "Export failed"}</span>
            <button onClick={handleExport} disabled={loading}>Retry</button>
          </div>
        )}
        {saved && (
          <div className={styles.resultBox}>
            <span>Saved: {basename(saved.filePath)}</span>
            {backlogItemId ? (
              <Link href={`/backlog?item=${backlogItemId}`}>View backlog item →</Link>
            ) : (
              <button onClick={handleCreateBacklogItem} disabled={creating} data-testid="create-backlog-from-bundle">
                Create backlog item →
              </button>
            )}
          </div>
        )}
        <ModalFooter>
          <button onClick={() => onOpenChange(false)}>Close</button>
          <button onClick={handleExport} disabled={!consented || loading} data-testid="export-bundle-submit">
            {loading ? "Assembling bundle…" : "Export"}
          </button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
