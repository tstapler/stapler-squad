"use client";
// +feature: backlog:claim-conflict-banner

import { useCallback, useEffect, useRef, useState } from "react";
import { AlertTriangle, Info } from "lucide-react";
import { ClaimOverrideForm } from "./ClaimOverrideForm";
import * as styles from "./ClaimConflictBanner.css";

const COPIED_RESET_MS = 1500;

interface ClaimConflictBannerProps {
  /** Literal name of the claiming host: never "elsewhere". */
  hostname: string;
  itemDeepLink: string;
  /** Human age of the claim, e.g. "12m ago". Omitted when unknown. */
  lastSeenAt?: string;
  /** Two hosts both claim the URL: shows Resolve instead of Override. */
  disputed?: boolean;
  /** A peer was unreachable: "could not confirm", offering only Retry check. */
  indeterminate?: boolean;
  onCopyDeepLink?: () => void;
  /** Called with the trimmed reason (>= 5 characters). Reject to show the message on the form. */
  onOverride?: (reason: string) => void | Promise<void>;
  onResolveDispute?: (reason: string) => void | Promise<void>;
  onRetry?: () => void;
}

/**
 * Claim-conflict banner for the item detail (design/ux.md Surface 1). Always
 * role="status"/aria-live="polite": the current link is not a dead end, so this
 * never uses role="alert".
 */
export function ClaimConflictBanner({
  hostname,
  itemDeepLink,
  lastSeenAt,
  disputed = false,
  indeterminate = false,
  onCopyDeepLink,
  onOverride,
  onResolveDispute,
  onRetry,
}: ClaimConflictBannerProps) {
  const [copied, setCopied] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current); }, []);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(itemDeepLink);
    } catch {
      return;
    }
    setCopied(true);
    onCopyDeepLink?.();
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setCopied(false), COPIED_RESET_MS);
  }, [itemDeepLink, onCopyDeepLink]);

  const submit = disputed ? onResolveDispute : onOverride;
  const handleConfirm = useCallback(
    async (reason: string) => {
      if (!submit) return;
      setBusy(true);
      setFormError(null);
      try {
        await submit(reason);
        setFormOpen(false);
      } catch (err) {
        setFormError(err instanceof Error ? err.message : "Action failed.");
      } finally {
        setBusy(false);
      }
    },
    [submit]
  );

  const Icon = disputed ? AlertTriangle : Info;
  const headline = indeterminate
    ? "Couldn't confirm this isn't claimed elsewhere"
    : disputed
      ? "Claim disputed — two hosts both claim this issue"
      : `This issue is already claimed by "${hostname}"`;
  const body = indeterminate
    ? "None of the known hosts responded to the claim check. Proceeding is allowed, but check back if this seems off."
    : disputed
      ? `"${hostname}" and another host have each claimed this. Nothing was auto-resolved; pick which claim is correct.`
      : `"${hostname}" claimed this${lastSeenAt ? ` ${lastSeenAt}` : ""}. Working on it here risks a duplicate PR.`;
  const actionLabel = disputed ? "Resolve — work on it here" : "Override — work on it here";

  return (
    <div className={styles.container} role="status" aria-live="polite" data-testid="claim-conflict-banner">
      <Icon className={styles.icon} size={16} aria-hidden="true" />
      <div className={styles.content}>
        <span className={styles.headline}>{headline}</span>
        <p className={styles.body}>{body}</p>
        <div className={styles.actions}>
          {indeterminate ? (
            onRetry && (
              <button type="button" className={styles.actionButton} onClick={onRetry} data-testid="claim-banner-retry">
                Retry check
              </button>
            )
          ) : (
            <>
              <button type="button" className={styles.actionButton} onClick={() => void handleCopy()} data-testid="claim-banner-copy">
                {copied ? "✓ Copied" : `Copy link to "${hostname}"'s item`}
              </button>
              {submit && (
                <button
                  type="button"
                  className={styles.actionButton}
                  aria-expanded={formOpen}
                  onClick={() => setFormOpen((open) => !open)}
                  data-testid="claim-banner-action"
                >
                  {actionLabel}
                </button>
              )}
            </>
          )}
        </div>
        {formOpen && submit && (
          <div className={styles.formSlot}>
            <ClaimOverrideForm
              label={disputed ? "Reason for resolving this dispute (required)" : "Reason for working on this despite the claim (required)"}
              confirmLabel={disputed ? "Resolve" : "Override"}
              onConfirm={handleConfirm}
              onCancel={() => setFormOpen(false)}
              busy={busy}
              errorMessage={formError}
            />
          </div>
        )}
        <span className={styles.srOnly} aria-live="polite" data-testid="claim-banner-copy-status">
          {copied ? `Link to ${hostname}'s item copied to clipboard` : ""}
        </span>
      </div>
    </div>
  );
}
