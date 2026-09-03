"use client";
// +feature: import-preview-dialog

import { useEffect, useRef, useState, RefObject } from "react";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useImportSessionService } from "@/lib/hooks/useImportSessionService";
import {
  CorrelationKind,
  CorrelationConfidence,
  type ExternalSessionCandidateRef,
  type PreviewImportExternalSessionResponse,
} from "@/gen/session/v1/import_pb";
import * as styles from "./ImportPreviewDialog.css";

interface ImportPreviewDialogProps {
  candidate: ExternalSessionCandidateRef;
  onConfirm: (params: {
    preview: PreviewImportExternalSessionResponse;
    disambiguationChoice?: string;
  }) => Promise<void> | void;
  onCancel: () => void;
  triggerRef?: RefObject<HTMLElement | null>;
}

const CONFIDENCE_LABEL: Record<CorrelationConfidence, string> = {
  [CorrelationConfidence.UNSPECIFIED]: "",
  [CorrelationConfidence.NONE]: "No confidence",
  [CorrelationConfidence.PID_EXACT]: "Exact PID match",
  [CorrelationConfidence.PATH_HEURISTIC]: "Path heuristic match",
};

export function ImportPreviewDialog({
  candidate,
  onConfirm,
  onCancel,
  triggerRef,
}: ImportPreviewDialogProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  useFocusTrap(modalRef, true, triggerRef);
  const { previewImport } = useImportSessionService();

  const [preview, setPreview] = useState<PreviewImportExternalSessionResponse | null>(
    null
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [disambiguationChoice, setDisambiguationChoice] = useState<string>("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    previewImport(candidate)
      .then((result) => {
        if (cancelled) return;
        if (!result) {
          setError("Failed to preview this session.");
          return;
        }
        setPreview(result);
        const firstCandidate = result.correlation?.candidates?.[0];
        if (
          result.correlation?.kind === CorrelationKind.AMBIGUOUS &&
          firstCandidate
        ) {
          setDisambiguationChoice(firstCandidate.conversationUuid);
        }
      })
      .catch(() => {
        if (!cancelled) setError("Failed to preview this session.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // candidate identity is stable per dialog instance
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const kind = preview?.correlation?.kind ?? CorrelationKind.UNSPECIFIED;
  const canConfirm =
    !loading &&
    !error &&
    !!preview &&
    (kind !== CorrelationKind.AMBIGUOUS || !!disambiguationChoice);

  const handleSubmit = async () => {
    if (!preview || !canConfirm || isSubmitting) return;
    setIsSubmitting(true);
    try {
      await onConfirm({
        preview,
        disambiguationChoice:
          kind === CorrelationKind.AMBIGUOUS ? disambiguationChoice : undefined,
      });
    } catch {
      setIsSubmitting(false);
    }
  };

  const badgeClassForKind = (k: CorrelationKind) => {
    switch (k) {
      case CorrelationKind.RESOLVED:
        return `${styles.correlationBadge} ${styles.correlationBadgeResolved}`;
      case CorrelationKind.AMBIGUOUS:
        return `${styles.correlationBadge} ${styles.correlationBadgeAmbiguous}`;
      default:
        return `${styles.correlationBadge} ${styles.correlationBadgeNotFound}`;
    }
  };

  const badgeTextForKind = (k: CorrelationKind) => {
    switch (k) {
      case CorrelationKind.RESOLVED:
        return "History matched";
      case CorrelationKind.AMBIGUOUS:
        return "Multiple histories found";
      default:
        return "No history matched";
    }
  };

  return (
    <div className={styles.overlay} onClick={onCancel}>
      <div
        className={styles.modal}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="import-preview-title"
        ref={modalRef}
        data-testid="import-preview-dialog"
      >
        <div className={styles.header}>
          <h2 className={styles.title} id="import-preview-title">
            Import External Session
          </h2>
          <p className={styles.subtitle}>
            Review the discovered session before importing
          </p>
        </div>

        <div className={styles.body}>
          {loading && (
            <div className={styles.loadingState}>Looking up session history…</div>
          )}

          {!loading && error && <div className={styles.errorState}>{error}</div>}

          {!loading && !error && preview && (
            <>
              <div className={styles.contextGrid}>
                <div className={styles.contextRow}>
                  <span className={styles.contextLabel}>Program:</span>
                  <span className={styles.contextValue}>{preview.program}</span>
                </div>
                <div className={styles.contextRow}>
                  <span className={styles.contextLabel}>Path:</span>
                  <span className={styles.contextValue} title={preview.path}>
                    {preview.path}
                  </span>
                </div>
              </div>

              <div className={styles.section}>
                <span className={styles.fieldLabel}>Conversation History</span>
                <span className={badgeClassForKind(kind)}>
                  {badgeTextForKind(kind)}
                  {preview.correlation?.confidence !== undefined &&
                    preview.correlation.confidence !==
                      CorrelationConfidence.UNSPECIFIED &&
                    ` · ${CONFIDENCE_LABEL[preview.correlation.confidence]}`}
                </span>

                {kind === CorrelationKind.RESOLVED && (
                  <>
                    <div className={styles.contextRow}>
                      <span className={styles.contextLabel}>Turns:</span>
                      <span className={styles.contextValue}>
                        {preview.turnCount}
                      </span>
                    </div>
                    {preview.lastMessageExcerpt && (
                      <div className={styles.excerptBox}>
                        {preview.lastMessageExcerpt}
                      </div>
                    )}
                  </>
                )}

                {kind === CorrelationKind.AMBIGUOUS && (
                  <div className={styles.candidateList} role="radiogroup" aria-label="Select conversation history">
                    {preview.correlation?.candidates.map((c) => (
                      <label
                        key={c.conversationUuid}
                        className={
                          disambiguationChoice === c.conversationUuid
                            ? `${styles.candidateOption} ${styles.candidateOptionSelected}`
                            : styles.candidateOption
                        }
                      >
                        <input
                          type="radio"
                          className={styles.candidateRadio}
                          name="disambiguation-choice"
                          checked={disambiguationChoice === c.conversationUuid}
                          onChange={() =>
                            setDisambiguationChoice(c.conversationUuid)
                          }
                        />
                        <div className={styles.candidateDetails}>
                          <span className={styles.candidatePath} title={c.historyFilePath}>
                            {c.projectDir || c.historyFilePath}
                          </span>
                          <span className={styles.candidateUuid}>
                            {c.conversationUuid}
                          </span>
                        </div>
                      </label>
                    ))}
                  </div>
                )}
              </div>

              {preview.pidIdentity && (
                <div className={styles.sigstopBanner}>
                  <span>⚠</span>
                  <span>
                    Importing will suspend (SIGSTOP) the original process
                    (PID {preview.pidIdentity.pid}) and resume it under Stapler
                    Squad&apos;s management. The original terminal will freeze
                    until you confirm or cancel the kill afterward.
                  </span>
                </div>
              )}
            </>
          )}
        </div>

        <div className={styles.footer}>
          <button
            onClick={onCancel}
            className={styles.cancelButton}
            disabled={isSubmitting}
            type="button"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            className={styles.confirmButton}
            disabled={!canConfirm || isSubmitting}
            type="button"
            data-testid="import-preview-confirm-button"
          >
            {isSubmitting ? "Importing..." : "Import Session"}
          </button>
        </div>
      </div>
    </div>
  );
}
