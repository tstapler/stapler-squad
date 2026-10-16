"use client";

import { useCallback, useState } from "react";
import type { ClaimedElsewhere } from "@/lib/hooks/useBacklogService";
import { ClaimOverrideForm } from "./ClaimOverrideForm";
import * as styles from "./ClaimedElsewhereNotice.css";

export interface ClaimedElsewhereEntry {
  issueUrl: string;
  claim: ClaimedElsewhere;
}

interface ClaimedElsewhereNoticeProps {
  entries: ClaimedElsewhereEntry[];
  /** Resubmits the import with override + reason. Reject to show the message on the form. */
  onImportAnyway: (entry: ClaimedElsewhereEntry, reason: string) => Promise<void>;
}

/** The claiming host's name for display: the deep link's authority, else its opaque ID. */
export function claimingHostLabel(claim: ClaimedElsewhere): string {
  try {
    const host = new URL(claim.itemDeepLink).hostname;
    if (host) return host;
  } catch {
    // fall through to the opaque host ID
  }
  return claim.claimingHostId;
}

/**
 * Third import-result bucket, distinct from "already imported" (same host) and
 * "import failed": another stapler-squad host already claimed the issue. Offers
 * Copy link and an audited "Import anyway" (reason >= 5 characters).
 */
export function ClaimedElsewhereNotice({ entries, onImportAnyway }: ClaimedElsewhereNoticeProps) {
  const [openUrl, setOpenUrl] = useState<string | null>(null);
  const [busyUrl, setBusyUrl] = useState<string | null>(null);
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);
  const [errorUrl, setErrorUrl] = useState<{ url: string; message: string } | null>(null);

  const copyLink = useCallback(async (entry: ClaimedElsewhereEntry) => {
    try {
      await navigator.clipboard.writeText(entry.claim.itemDeepLink);
      setCopiedUrl(entry.issueUrl);
    } catch {
      setCopiedUrl(null);
    }
  }, []);

  const confirm = useCallback(
    async (entry: ClaimedElsewhereEntry, reason: string) => {
      setBusyUrl(entry.issueUrl);
      setErrorUrl(null);
      try {
        await onImportAnyway(entry, reason);
        setOpenUrl(null);
      } catch (err) {
        setErrorUrl({ url: entry.issueUrl, message: err instanceof Error ? err.message : "Import failed." });
      } finally {
        setBusyUrl(null);
      }
    },
    [onImportAnyway]
  );

  if (entries.length === 0) return null;

  return (
    <div className={styles.container} role="status" data-testid="claimed-elsewhere-notice">
      <p className={styles.heading}>
        {entries.length === 1 ? "1 issue is already claimed by another host" : `${entries.length} issues are already claimed by another host`}
      </p>
      {entries.map((entry) => {
        const host = claimingHostLabel(entry.claim);
        return (
          <div key={entry.issueUrl} className={styles.entry} data-testid="claimed-elsewhere-entry">
            <span className={styles.entryText}>
              {entry.issueUrl} — {entry.claim.disputed ? `disputed between hosts, currently ${host}` : `claimed by ${host}`}
            </span>
            <div className={styles.entryActions}>
              <button type="button" className={styles.actionButton} onClick={() => void copyLink(entry)} data-testid="claimed-elsewhere-copy">
                {copiedUrl === entry.issueUrl ? "Copied" : `Copy link to ${host}'s item`}
              </button>
              <button
                type="button"
                className={styles.actionButton}
                aria-expanded={openUrl === entry.issueUrl}
                onClick={() => setOpenUrl((prev) => (prev === entry.issueUrl ? null : entry.issueUrl))}
                data-testid="claimed-elsewhere-import-anyway"
              >
                Import anyway
              </button>
            </div>
            {openUrl === entry.issueUrl && (
              <ClaimOverrideForm
                label="Reason for importing anyway (required)"
                confirmLabel="Import anyway"
                onConfirm={(reason) => confirm(entry, reason)}
                onCancel={() => setOpenUrl(null)}
                busy={busyUrl === entry.issueUrl}
                errorMessage={errorUrl?.url === entry.issueUrl ? errorUrl.message : null}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
