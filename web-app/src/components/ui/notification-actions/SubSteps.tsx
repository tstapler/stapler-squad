"use client";

import type { Ref } from "react";
import { COPY, createRulePrompt } from "@/lib/copy/notificationCopy";
import { ActionButton } from "./ActionButton";
import { approveVariant, denyVariant, field, stepActions, stickyBar, subStep } from "./ActionRow.css";

interface StepBase {
  onBack: () => void;
  minHeight?: number;
  busy: boolean;
}

interface ConfirmStepProps extends StepBase {
  prompt: string;
  confirmLabel: string;
  /** "Back" for an inline step, "Keep" when declining a cancel. */
  backLabel: string;
  onConfirm: () => void;
  confirmRef: Ref<HTMLButtonElement>;
  testId?: string;
  confirmId?: string;
}

/** An inline two-step ("Merge PR #901?"); focus lands on the confirm button. */
export function ConfirmStep({ prompt, confirmLabel, backLabel, onConfirm, onBack, confirmRef, busy, minHeight, testId = "confirm-step", confirmId = "confirm" }: ConfirmStepProps) {
  return (
    <div className={subStep} data-testid={testId}>
      <span>{prompt}</span>
      <div className={stepActions}>
        <ActionButton id={confirmId} label={confirmLabel} onClick={onConfirm} buttonRef={confirmRef} variant={approveVariant} disabled={busy} minHeight={minHeight} />
        <ActionButton id="back" label={backLabel} onClick={onBack} minHeight={minHeight} />
      </div>
    </div>
  );
}

interface DenyStepProps extends StepBase {
  fieldId: string;
  reason: string;
  onReason: (v: string) => void;
  onSend: () => void;
  fieldRef: Ref<HTMLInputElement>;
}

/** Optional one-line reason; Back keeps the draft. */
export function DenyStep({ fieldId, reason, onReason, onSend, onBack, fieldRef, busy, minHeight }: DenyStepProps) {
  return (
    <div className={subStep} data-testid="deny-step">
      <label htmlFor={fieldId}>{COPY.denyReasonLabel}</label>
      <input
        id={fieldId}
        ref={fieldRef}
        type="text"
        className={field}
        value={reason}
        data-testid="deny-reason"
        // Mobile keyboard matrix (ux.md 11.4): Enter sends a single-line reason.
        enterKeyHint="send"
        autoCapitalize="sentences"
        autoComplete="off"
        spellCheck
        onFocus={(e) => e.currentTarget.scrollIntoView?.({ block: "nearest" })}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.nativeEvent.isComposing && !busy) {
            e.preventDefault();
            onSend();
          }
        }}
        onChange={(e) => onReason(e.target.value)}
      />
      <div className={`${stepActions} ${stickyBar}`}>
        <ActionButton id="send-denial" label={COPY.sendDenial} onClick={onSend} variant={denyVariant} disabled={busy} minHeight={minHeight} />
        <ActionButton id="back" label={COPY.back} onClick={onBack} minHeight={minHeight} />
      </div>
    </div>
  );
}

export const createRuleStepProps = (scope: string) => ({
  prompt: createRulePrompt(scope),
  confirmLabel: COPY.createRule,
  testId: "create-rule-step",
  confirmId: "create-rule-confirm",
});
