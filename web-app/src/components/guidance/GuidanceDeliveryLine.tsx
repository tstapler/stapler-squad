"use client";

import { useRef, useState, type ReactNode } from "react";
import { COPY, DELIVERY_COPY } from "@/lib/copy/notificationCopy";
import type { DeliveryAction, DeliveryView } from "@/lib/guidance/deliveryView";
import { ActionButton } from "@/components/ui/notification-actions/ActionButton";
import { ConfirmStep } from "@/components/ui/notification-actions/SubSteps";
import * as styles from "./GuidanceQuestionForm.css";

type Confirm = "retryAnyway" | "markHandled" | "dismissConfirm";

const CONFIRM_COPY: Record<Confirm, { prompt: string; confirmLabel: string }> = {
  retryAnyway: { prompt: COPY.retryAnywayPrompt, confirmLabel: COPY.retryAnywayConfirm },
  markHandled: { prompt: DELIVERY_COPY.markHandledPrompt, confirmLabel: DELIVERY_COPY.markHandledConfirm },
  dismissConfirm: { prompt: DELIVERY_COPY.dismissUnfinishedPrompt, confirmLabel: DELIVERY_COPY.dismissUnfinishedConfirm },
};

const LABEL: Record<Exclude<DeliveryAction, "open">, string> = {
  retry: COPY.retry,
  retryAnyway: COPY.retryAnyway,
  markHandled: DELIVERY_COPY.markHandled,
  dismiss: COPY.dismiss,
  dismissConfirm: COPY.dismiss,
};

interface DeliveryLineProps {
  view: DeliveryView;
  /** Runs an action; the confirm-gated ones run only after their inline confirm. */
  onAction: (action: Exclude<DeliveryAction, "open">) => void;
  /** Open session control; the row supplies its own link. */
  openControl: ReactNode;
  minHeight?: number;
  busy?: boolean;
}

/**
 * The one delivery-state line of a question card: plain text (never a live region; the
 * single global polite region announces the short result), an Unconfirmed chip on rows 3 and
 * 6, and the row's actions. Retry anyway, Mark as handled and Dismiss (aborted) confirm inline.
 */
export function GuidanceDeliveryLine({ view, onAction, openControl, minHeight, busy = false }: DeliveryLineProps) {
  const [confirming, setConfirming] = useState<Confirm | null>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);
  const press = (a: DeliveryAction) => {
    if (a === "retryAnyway" || a === "markHandled" || a === "dismissConfirm") setConfirming(a);
    else if (a !== "open") onAction(a);
  };
  if (confirming) {
    const copy = CONFIRM_COPY[confirming];
    return (
      <div className={styles.deliveryLine} data-testid="delivery-line" data-delivery-row={view.row}>
        <ConfirmStep
          {...copy}
          backLabel={COPY.back}
          busy={busy}
          minHeight={minHeight}
          confirmRef={confirmRef}
          testId="delivery-confirm"
          onBack={() => setConfirming(null)}
          onConfirm={() => {
            setConfirming(null);
            onAction(confirming);
          }}
        />
      </div>
    );
  }
  return (
    <div className={styles.deliveryLine} data-testid="delivery-line" data-delivery-row={view.row}>
      <p className={styles.deliveryText} data-testid="delivery-text">
        {view.text}
      </p>
      {view.chip && (
        <span className={styles.chip} data-testid="delivery-chip">
          {view.chip}
        </span>
      )}
      <div className={styles.optionList}>
        {view.actions.map((a) =>
          a === "open" ? (
            <span key="open">{openControl}</span>
          ) : (
            <ActionButton key={a} id={`delivery-${a}`} label={LABEL[a]} minHeight={minHeight} disabled={busy} onClick={() => press(a)} />
          ),
        )}
      </div>
    </div>
  );
}
