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
  const [pendingKill, setPendingKill] = useState<PendingKill | null>(null);
  const [commitError, setCommitError] = useState<string | null>(null);

  const handleImport = useCallback((candidates: ExternalSessionCandidateRef[]) => {
    const [first] = candidates;
    if (first) {
      setCommitError(null);
      setPreviewCandidate(first);
    }
  }, []);

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
      setPreviewCandidate(null);
      if (result.pidIdentity) {
        setPendingKill({
          instanceId: result.instanceId,
          pidIdentity: result.pidIdentity,
          program: preview.program,
        });
      }
    },
    [previewCandidate, commitImport]
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
          onCancel={() => setPreviewCandidate(null)}
        />
      )}

      {pendingKill && (
        <ConfirmKillDialog
          instanceId={pendingKill.instanceId}
          pidIdentity={pendingKill.pidIdentity}
          program={pendingKill.program}
          onStatusChange={handleKillStatusChange}
          onClose={() => setPendingKill(null)}
        />
      )}
    </div>
  );
}
