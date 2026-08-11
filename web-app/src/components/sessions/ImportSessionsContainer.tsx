"use client";
// +feature: import-sessions-container

import { useCallback, useState } from "react";
import { useImportSessionService } from "@/lib/hooks/useImportSessionService";
import {
  ImportStatus,
  type ExternalSessionCandidateRef,
  type PIDIdentity,
} from "@/gen/session/v1/import_pb";
import { ImportExternalSessionsPanel } from "./ImportExternalSessionsPanel";
import { ImportPreviewDialog } from "./ImportPreviewDialog";
import { ConfirmKillDialog } from "./ConfirmKillDialog";
import type { ImportRowStatus } from "./importRowStatus";
import * as styles from "./ImportSessionsContainer.css";

interface PendingKill {
  instanceId: string;
  pidIdentity?: PIDIdentity;
  program: string;
}

export function ImportSessionsContainer() {
  const { commitImport } = useImportSessionService();
  const [previewCandidate, setPreviewCandidate] =
    useState<ExternalSessionCandidateRef | null>(null);
  // importQueue holds candidates still waiting to be previewed after
  // previewCandidate, so a bulk "Import selected" (N candidates) works
  // through all of them one at a time instead of only the first.
  const [importQueue, setImportQueue] = useState<ExternalSessionCandidateRef[]>([]);
  const [pendingKill, setPendingKill] = useState<PendingKill | null>(null);
  const [commitError, setCommitError] = useState<string | null>(null);

  // advanceQueue pulls the next candidate (if any) off importQueue and makes
  // it the active preview, or clears previewCandidate once the queue is
  // empty. Call this whenever the current candidate's flow (preview
  // cancelled, commit succeeded with no kill needed, or kill dialog closed)
  // has finished, so the next selected candidate is picked up automatically.
  const advanceQueue = useCallback((queue: ExternalSessionCandidateRef[]) => {
    const [next, ...rest] = queue;
    setImportQueue(rest);
    setPreviewCandidate(next ?? null);
  }, []);

  const handleImport = useCallback((candidates: ExternalSessionCandidateRef[]) => {
    if (candidates.length === 0) return;
    setCommitError(null);
    advanceQueue(candidates);
  }, [advanceQueue]);

  const handleConfirmPreview = useCallback(
    async ({
      preview,
      disambiguationChoice,
    }: Parameters<
      React.ComponentProps<typeof ImportPreviewDialog>["onConfirm"]
    >[0]) => {
      const candidate = previewCandidate;
      if (!candidate || !preview.correlation) return;
      const result = await commitImport({
        candidate,
        expectedCorrelation: preview.correlation,
        disambiguationChoice,
        pidIdentity: preview.pidIdentity,
      });
      if (!result || result.status !== ImportStatus.COMMITTED) {
        setCommitError(result?.error || "Failed to import session.");
        return;
      }
      if (result.pidIdentity) {
        setPreviewCandidate(null);
        setPendingKill({
          instanceId: result.instanceId,
          pidIdentity: result.pidIdentity,
          program: preview.program,
        });
        return;
      }
      // No kill decision needed for this candidate -- move on to the next
      // queued candidate (if any) right away.
      advanceQueue(importQueue);
    },
    [previewCandidate, commitImport, importQueue, advanceQueue]
  );

  const handleKillStatusChange = useCallback((_status: ImportRowStatus) => {
    // Row status is displayed inline in ImportExternalSessionsPanel once wired
    // to live session state; this container only needs to close the dialog.
  }, []);

  return (
    <div className={styles.container} data-testid="import-sessions-container">
      {commitError && (
        <div className={styles.errorBanner} role="alert" data-testid="import-commit-error">
          {commitError}
        </div>
      )}

      <ImportExternalSessionsPanel onImport={handleImport} />

      {previewCandidate && (
        <ImportPreviewDialog
          candidate={previewCandidate}
          onConfirm={handleConfirmPreview}
          onCancel={() => advanceQueue(importQueue)}
        />
      )}

      {pendingKill && (
        <ConfirmKillDialog
          instanceId={pendingKill.instanceId}
          pidIdentity={pendingKill.pidIdentity}
          program={pendingKill.program}
          onStatusChange={handleKillStatusChange}
          onClose={() => {
            setPendingKill(null);
            advanceQueue(importQueue);
          }}
        />
      )}
    </div>
  );
}
