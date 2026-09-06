"use client";
// +feature: jules-dispatch

// JulesDispatchDialog — Dispatch UX (google-jules-integration Epic 3.2,
// Story 3.2.1). Opened from ActionsSection's gated "Dispatch to Jules"
// button (BacklogItemDetail.tsx). Asks for the branch + prompt and, the
// first time a given repo is dispatched to, requires an explicit egress
// confirmation before any code leaves the machine. See
// project_plans/google-jules-integration/design/ux.md §3 for the full
// wireframe/interaction spec this implements.

import { useEffect, useId, useRef, useState, RefObject } from "react";
import { createPortal } from "react-dom";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { AcCriterion } from "@/lib/hooks/useBacklogService";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { getErrorMessage } from "@/lib/utils/connectError";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import {
  overlay,
  dialog,
  heading,
  egressBlock,
  egressCheckboxRow,
  field,
  label as labelClass,
  input,
  textarea,
  hint,
  errorBanner,
  actions,
  primaryButton,
  secondaryButton,
} from "./JulesDispatchDialog.css";

export interface JulesDispatchDialogProps {
  itemId: string;
  itemTitle: string;
  acCriteria: AcCriterion[];
  /** Absolute filesystem path to the item's repo — used both to derive the
   * display label and as ConfirmEgressConsent's repo_path. */
  repoPath: string;
  /** Newest non-empty worktree_branch across the item's sessions — the
   * dialog's starting Branch value, editable before submit. */
  initialBranch: string;
  /** True when repoPath is already in EgressAcknowledgedRepos — suppresses
   * the egress confirmation block entirely (Story 3.2.1's third AC). */
  egressAcknowledged: boolean;
  onClose: () => void;
  /** Ref to the button that opened this dialog — focus returns here on close. */
  triggerRef?: RefObject<HTMLElement | null>;
}

// shortRepoLabel mirrors JulesSettings.tsx's own local helper (not exported,
// so duplicated here) — derives a display-friendly "owner/repo" from the
// full filesystem path stored in RepoPath/EgressAcknowledgedRepos.
function shortRepoLabel(repoPath: string): string {
  const segments = repoPath.split("/").filter(Boolean);
  if (segments.length < 2) return repoPath;
  return segments.slice(-2).join("/");
}

// buildDefaultPrompt seeds the Prompt textarea from the item's own title +
// acceptance criteria, the same inputs a local spawn's triage prompt is
// built from — kept intentionally simple (no server round-trip) since the
// user can freely edit it before dispatch.
function buildDefaultPrompt(title: string, acCriteria: AcCriterion[]): string {
  if (acCriteria.length === 0) return title;
  const criteriaText = acCriteria.map((c) => `- ${c.text}`).join("\n");
  return `${title}\n\nAcceptance criteria:\n${criteriaText}`;
}

export function JulesDispatchDialog({
  itemId,
  itemTitle,
  acCriteria,
  repoPath,
  initialBranch,
  egressAcknowledged,
  onClose,
  triggerRef,
}: JulesDispatchDialogProps) {
  const headingId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  useFocusTrap(dialogRef, true, triggerRef);

  const [branch, setBranch] = useState(initialBranch);
  const [prompt, setPrompt] = useState(() => buildDefaultPrompt(itemTitle, acCriteria));
  const [consentChecked, setConsentChecked] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const repoLabel = shortRepoLabel(repoPath);

  const clientsRef = useRef<{
    backlog: ReturnType<typeof createClient<typeof BacklogService>>;
    session: ReturnType<typeof createClient<typeof SessionService>>;
  } | null>(null);
  if (!clientsRef.current) {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    clientsRef.current = {
      backlog: createClient(BacklogService, transport),
      session: createClient(SessionService, transport),
    };
  }

  // Escape closes the dialog, mirroring ReviewChangesModal/
  // BacklogFileBrowserModal — useFocusTrap only handles Tab-cycling, not Esc.
  // Ignored while a dispatch is in flight, same guard as the disabled
  // Cancel button below (prevents an in-flight submit from being abandoned
  // mid-request).
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape" && !submitting) {
        e.stopPropagation();
        onClose();
      }
    }
    window.addEventListener("keydown", handleKeyDown, { capture: true });
    return () => window.removeEventListener("keydown", handleKeyDown, { capture: true });
  }, [onClose, submitting]);

  const dispatchDisabled =
    submitting || branch.trim() === "" || prompt.trim() === "" || (!egressAcknowledged && !consentChecked);

  async function handleDispatch() {
    if (dispatchDisabled || !clientsRef.current) return;
    setSubmitting(true);
    setError(null);
    try {
      if (!egressAcknowledged) {
        await clientsRef.current.session.confirmEgressConsent({ repoPath });
      }
      await clientsRef.current.backlog.dispatchToJules({
        itemId,
        branch: branch.trim(),
        prompt: prompt.trim(),
      });
      onClose();
    } catch (err) {
      setError(getErrorMessage(err, "Couldn't reach Jules. Try again."));
    } finally {
      setSubmitting(false);
    }
  }

  const content = (
    <div className={overlay} data-testid="jules-dispatch-overlay">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={headingId}
        className={dialog}
        data-testid="jules-dispatch-dialog"
      >
        <h2 id={headingId} className={heading}>
          Dispatch to Jules
        </h2>

        {!egressAcknowledged && (
          <div className={egressBlock} data-testid="jules-dispatch-egress-block">
            <span>
              The contents of {repoLabel} will be sent to Google&apos;s cloud VM to run this
              session.
            </span>
            <label className={egressCheckboxRow}>
              <input
                type="checkbox"
                checked={consentChecked}
                onChange={(e) => setConsentChecked(e.target.checked)}
                data-testid="jules-dispatch-egress-checkbox"
              />
              I understand and want to continue
            </label>
          </div>
        )}

        <div className={field}>
          <label htmlFor="jules-dispatch-branch" className={labelClass}>
            Branch
          </label>
          <input
            id="jules-dispatch-branch"
            type="text"
            className={input}
            value={branch}
            onChange={(e) => setBranch(e.target.value)}
            aria-describedby="jules-dispatch-branch-hint"
            data-testid="jules-dispatch-branch"
          />
          <p id="jules-dispatch-branch-hint" className={hint}>
            Jules starts from a branch already pushed to GitHub — local-only branches
            won&apos;t work.
          </p>
        </div>

        <div className={field}>
          <label htmlFor="jules-dispatch-prompt" className={labelClass}>
            Prompt
          </label>
          <textarea
            id="jules-dispatch-prompt"
            className={textarea}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            rows={6}
            data-testid="jules-dispatch-prompt"
          />
        </div>

        {error && (
          <p role="alert" className={errorBanner} data-testid="jules-dispatch-error">
            {error}
          </p>
        )}

        <div className={actions}>
          <button
            type="button"
            className={secondaryButton}
            onClick={onClose}
            disabled={submitting}
            data-testid="jules-dispatch-cancel"
          >
            Cancel
          </button>
          <button
            type="button"
            className={primaryButton}
            onClick={() => {
              void handleDispatch();
            }}
            disabled={dispatchDisabled}
            data-testid="jules-dispatch-submit"
          >
            {submitting ? "Dispatching…" : "Dispatch"}
          </button>
        </div>
      </div>
    </div>
  );

  if (typeof document === "undefined") return null;
  return createPortal(content, document.body);
}
