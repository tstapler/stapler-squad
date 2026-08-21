"use client";
// +feature: session-create-pr

import { useEffect, useRef, useState, RefObject, KeyboardEvent } from "react";
import { createPortal } from "react-dom";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import type { Session } from "@/gen/session/v1/types_pb";
import type {
  DraftPullRequestResponse,
  CreatePullRequestResponse,
} from "@/gen/session/v1/session_pb";
import {
  confirmDialog,
  dialogContent,
  dialogActions,
  submitButton,
  cancelButton,
  errorMessage,
  bodyTextarea,
  fieldLabel,
  textInput,
  branchContext,
  loadingState,
  successMessage,
  prLink,
  warningMessage,
} from "./CreatePullRequestModal.css";

export interface CreatePullRequestRequest {
  sessionId: string;
  title: string;
  body: string;
  baseBranch: string;
}

interface CreatePullRequestModalProps {
  session: Session;
  isOpen: boolean;
  onClose: () => void;
  draftPullRequest: (sessionId: string) => Promise<DraftPullRequestResponse | null>;
  createPullRequest: (
    req: CreatePullRequestRequest
  ) => Promise<CreatePullRequestResponse | null>;
  // RefObject<HTMLElement | null> (not RefObject<HTMLButtonElement>) to match
  // useFocusTrap's own signature and every other dialog's triggerRef prop in
  // this file's sibling components (TagEditor, ConfirmKillDialog).
  triggerRef?: RefObject<HTMLElement | null>;
}

interface ExistingPr {
  url: string;
  number: number;
}

interface SubmitSuccess {
  prUrl: string;
  prNumber: number;
  alreadyExisted: boolean;
  persisted: boolean;
}

const GENERIC_DRAFT_ERROR = "Couldn't load PR draft — try again.";

export function CreatePullRequestModal({
  session,
  isOpen,
  onClose,
  draftPullRequest,
  createPullRequest,
  triggerRef,
}: CreatePullRequestModalProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const titleInputRef = useRef<HTMLInputElement>(null);
  // Guards against a fast synthetic double-click racing the isSubmitting
  // re-render (ux.md criterion #8) — the disabled attribute alone isn't
  // enough because React state updates aren't synchronous.
  const submittingRef = useRef(false);

  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [baseBranch, setBaseBranch] = useState("");
  const [isDrafting, setIsDrafting] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [existingPr, setExistingPr] = useState<ExistingPr | null>(null);
  const [success, setSuccess] = useState<SubmitSuccess | null>(null);
  const [hasDrafted, setHasDrafted] = useState(false);

  useFocusTrap(dialogRef, isOpen, triggerRef);

  const fetchDraft = async () => {
    setIsDrafting(true);
    setError(null);
    setExistingPr(null);
    setHasDrafted(false);
    try {
      const response = await draftPullRequest(session.id);
      if (!response) {
        setError(GENERIC_DRAFT_ERROR);
        return;
      }
      if (response.existingPrUrl) {
        setExistingPr({ url: response.existingPrUrl, number: response.existingPrNumber });
        return;
      }
      setTitle(response.title);
      setBody(response.body);
      setBaseBranch(response.baseBranch);
      setHasDrafted(true);
    } catch {
      setError(GENERIC_DRAFT_ERROR);
    } finally {
      setIsDrafting(false);
    }
  };

  // Reset all state when the modal opens and kick off the draft fetch.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!isOpen) return;
    setTitle("");
    setBody("");
    setBaseBranch("");
    setExistingPr(null);
    setSuccess(null);
    setError(null);
    setHasDrafted(false);
    submittingRef.current = false;
    void fetchDraft();
    // Only re-run when the modal transitions to open, or for a different session.
  }, [isOpen, session.id]);

  // Autofocus the title input once the draft resolves into the editable
  // form (Surface 4) — not on dialog open, and not the submit button, per
  // ux.md's explicit rationale that the body needs review first.
  useEffect(() => {
    if (isOpen && hasDrafted && !isDrafting && !existingPr && !success) {
      titleInputRef.current?.focus();
    }
  }, [isOpen, hasDrafted, isDrafting, existingPr, success]);

  if (!isOpen) return null;

  const handleClose = () => {
    onClose();
  };

  const handleSubmit = async () => {
    if (submittingRef.current) return;
    if (isSubmitting || isDrafting || !title.trim()) return;
    submittingRef.current = true;
    setIsSubmitting(true);
    setError(null);
    try {
      const response = await createPullRequest({
        sessionId: session.id,
        title,
        body,
        baseBranch,
      });
      if (!response || !response.prUrl) {
        setError("Couldn't create the pull request — try again.");
        return;
      }
      setSuccess({
        prUrl: response.prUrl,
        prNumber: response.prNumber,
        alreadyExisted: response.alreadyExisted,
        persisted: response.persisted,
      });
    } catch (err) {
      // AC6/criterion #4: surface the backend error verbatim, never wrapped.
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      submittingRef.current = false;
      setIsSubmitting(false);
    }
  };

  const handleBackdropClick = () => {
    if (isSubmitting) return;
    handleClose();
  };

  const handleDialogKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape" && !isSubmitting) {
      handleClose();
    }
  };

  const fieldsDisabled = isSubmitting;
  const submitDisabled = isSubmitting || isDrafting || !title.trim();

  return createPortal(
    <div
      className={confirmDialog}
      onClick={(e) => {
        e.stopPropagation();
        handleBackdropClick();
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="createPrDialogTitle"
        className={dialogContent}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleDialogKeyDown}
      >
        <h3 id="createPrDialogTitle">Create Pull Request</h3>

        {/* Surface 7: success state (created / reused, +/- persist warning) */}
        {success ? (
          <>
            <p className={successMessage}>
              {"✅"} {success.alreadyExisted ? "Updated" : "Created"} PR #{success.prNumber}
            </p>
            <a
              className={prLink}
              href={success.prUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={`PR #${success.prNumber}`}
              data-testid="github-pr-link"
            >
              {success.prUrl}
            </a>
            {!success.persisted && (
              <p role="alert" className={warningMessage} data-testid="create-pr-persist-warning">
                {"⚠"} PR created but couldn&apos;t be saved to the session — refresh to
                check.
              </p>
            )}
            <div className={dialogActions}>
              <button onClick={handleClose} className={submitButton} data-testid="create-pr-close">
                Close
              </button>
            </div>
          </>
        ) : existingPr ? (
          /* Surface 5: existing-PR / "View PR" dead end */
          <>
            <p>This session already has a pull request.</p>
            <a
              className={prLink}
              href={existingPr.url}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={`PR #${existingPr.number}`}
              data-testid="github-pr-link"
            >
              View PR #{existingPr.number}
            </a>
            <div className={dialogActions}>
              <button onClick={handleClose} className={submitButton} data-testid="create-pr-close">
                Close
              </button>
            </div>
          </>
        ) : isDrafting ? (
          /* Surface 3: drafting/loading state */
          <>
            <p className={loadingState} data-testid="create-pr-loading">
              {"⏳"} Drafting PR description…
            </p>
            <div className={dialogActions}>
              <button onClick={handleClose} className={cancelButton} data-testid="create-pr-cancel">
                Cancel
              </button>
            </div>
          </>
        ) : error && !hasDrafted ? (
          /* Surface 8 Variant A: draft-fetch failure (form never rendered) */
          <>
            <p role="alert" className={errorMessage} data-testid="create-pr-error">
              {error}
            </p>
            <div className={dialogActions}>
              <button onClick={handleClose} className={cancelButton} data-testid="create-pr-close">
                Close
              </button>
              <button
                onClick={() => void fetchDraft()}
                className={submitButton}
                data-testid="create-pr-retry"
              >
                Retry
              </button>
            </div>
          </>
        ) : (
          /* Surface 4/6/8B: editable form (idle, submitting, or submit-error) */
          <>
            {(session.branch || baseBranch) && (
              <p className={branchContext} data-testid="create-pr-branch-context">
                {session.branch || "?"} → {baseBranch || "?"}
              </p>
            )}

            <label className={fieldLabel} htmlFor="createPrTitle">
              Title
            </label>
            <input
              id="createPrTitle"
              ref={titleInputRef}
              type="text"
              className={textInput}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              disabled={fieldsDisabled}
              data-testid="create-pr-title-input"
            />

            <label className={fieldLabel} htmlFor="createPrBody">
              Description
            </label>
            <textarea
              id="createPrBody"
              className={bodyTextarea}
              value={body}
              onChange={(e) => setBody(e.target.value)}
              disabled={fieldsDisabled}
              data-testid="create-pr-body-input"
            />

            <label className={fieldLabel} htmlFor="createPrBaseBranch">
              Base branch
            </label>
            <input
              id="createPrBaseBranch"
              type="text"
              className={textInput}
              value={baseBranch}
              onChange={(e) => setBaseBranch(e.target.value)}
              disabled={fieldsDisabled}
              data-testid="create-pr-base-branch-select"
            />

            {error && (
              <p role="alert" className={errorMessage} data-testid="create-pr-error">
                {error}
              </p>
            )}

            <div className={dialogActions}>
              <button
                onClick={handleClose}
                disabled={isSubmitting}
                className={cancelButton}
                data-testid="create-pr-cancel"
              >
                Cancel
              </button>
              <button
                onClick={() => void handleSubmit()}
                disabled={submitDisabled}
                className={submitButton}
                data-testid="create-pr-submit"
              >
                {isSubmitting ? "Creating PR…" : "Create PR"}
              </button>
            </div>
          </>
        )}
      </div>
    </div>,
    document.body
  );
}
