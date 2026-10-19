// +feature: notification-inline-actions
"use client";

import { useCallback, useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import {
  ActionKind,
  ActionOutcome,
  type NotificationActionDescriptor,
} from "@/gen/session/v1/notification_action_pb";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { useCardEngagement, useEngagement } from "@/lib/hooks/useEngagedCard";
import { useInvokeNotificationAction, type ActionSpec } from "@/lib/hooks/useInvokeNotificationAction";
import { useDeckViewport } from "@/lib/contexts/deckViewportContext";
import { clearDraft, readDraft, writeDraft } from "@/lib/utils/sessionDrafts";
import { COPY, cancelPrompt, mergePrompt, rateLimited } from "@/lib/copy/notificationCopy";
import { PendingNotice, useActionsDisabledReason } from "../OfflineBanner";
import { ActionButton } from "./ActionButton";
import { ApprovalButtons, OtherButtons, type ButtonsUi } from "./ActionButtons";
import { ApprovalCommand } from "./ApprovalCommand";
import { MoreMenu } from "./MoreMenu";
import { ConfirmStep, DenyStep, createRuleStepProps } from "./SubSteps";
import { outcomeView, type OutcomeContext } from "./actionOutcome";
import { KNOWN_KINDS, CANCEL_KINDS, buildSpec, isMerge, kindSlug, needsConfirm, unavailableReason, type Pick } from "./actionSpec";
import { TOUCH_TARGET_PX } from "./layout";
import { buttons, root, secondaryVariant, status } from "./ActionRow.css";

export { TOUCH_TARGET_PX, APPROVE_EXTRA_GAP_PX } from "./layout";

export interface ApprovalDisplay {
  tool?: string;
  /** Full command or argument text. */
  text?: string;
  /** The server could not render the tool input as text: fail closed. */
  unrenderable?: boolean;
  /** Set when a classifier rule already handled this approval. */
  ruleName?: string;
  /** Scope named in the Create rule confirm, e.g. "git push". */
  ruleScope?: string;
}

export interface ActionRowProps {
  /** Scopes sessionStorage drafts and the polite-region key. */
  rowId: string;
  descriptors: NotificationActionDescriptor[];
  approval?: ApprovalDisplay;
  prNumber?: string;
  /** The row's own Open link; defaults to an "Open session" button calling `onOpen`. */
  openControl?: ReactNode;
  onOpen?: () => void;
  /** INVALID: refetch descriptors (GetNotificationHistory). */
  onRefresh?: () => void;
  /** Enables the desktop-only More menu. */
  onCreateRule?: () => void;
  /** The More menu is never rendered on the remote listener. */
  localListener?: boolean;
  onApplied?: (kind: ActionKind) => void;
  /** Overrides the shared connectivity reason (Offline, Reconnecting, Updating...). */
  disabledReason?: string;
}

type Step = "row" | "deny" | "confirm" | "retry-anyway" | "more" | "create-rule";
type Options = { reason?: string; confirm?: boolean };

/** Seconds left of a RATE_LIMITED hold; ticks once a second and calls `onDone` at zero. */
function useCountdown(seconds: number | undefined, onDone: () => void): number {
  const [left, setLeft] = useState(seconds ?? 0);
  const doneRef = useRef(onDone);
  doneRef.current = onDone;
  useEffect(() => {
    setLeft(seconds ?? 0);
    if (!seconds) return;
    let remaining = seconds;
    const timer = setInterval(() => {
      remaining -= 1;
      setLeft(remaining);
      if (remaining <= 0) {
        clearInterval(timer);
        doneRef.current();
      }
    }, 1_000);
    return () => clearInterval(timer);
  }, [seconds]);
  return left;
}

function confirmCopy(p: Pick, prNumber?: string) {
  const { kind } = p.descriptor;
  if (CANCEL_KINDS.has(kind)) {
    const word = kind === ActionKind.CANCEL_PEER_HELP ? "request" : "prompt";
    const prompt = word === "request" ? COPY.cancelRequestQuestion : COPY.cancelPromptQuestion;
    return { prompt, confirmLabel: cancelPrompt(word), backLabel: COPY.keep };
  }
  const label = p.option?.label ?? p.descriptor.label;
  return {
    prompt: isMerge(p) ? mergePrompt(prNumber) : `${label}?`,
    confirmLabel: isMerge(p) ? COPY.confirmMerge : `Confirm ${label.toLowerCase()}`,
    backLabel: COPY.back,
  };
}

/**
 * The one inline action UI for a notification (ADR-002): renders only what the server's
 * descriptors allow, runs sub-steps inline (never a modal) and maps each typed outcome to
 * its copy. Every write goes through `dispatch`, the one place a hold before sending
 * (the Story 2.10 commit delay) can wrap.
 */
export function ActionRow(props: ActionRowProps) {
  const { rowId, descriptors, approval, prNumber, openControl, onOpen, onRefresh, onCreateRule, localListener = true, onApplied } = props;
  const { isInnerScreen } = useDeckViewport();
  const mobile = !isInnerScreen;
  const minHeight = mobile ? TOUCH_TARGET_PX : undefined;
  const { announce } = useAnnounce();
  const contextReason = useActionsDisabledReason();
  const offlineReason = props.disabledReason ?? contextReason;
  const { state, invoke, retry, reset } = useInvokeNotificationAction();
  const uid = useId();
  const engagement = useCardEngagement();
  const tracker = useEngagement();
  const statusRef = useRef<HTMLParagraphElement>(null);
  const focusWasInside = useRef(false);

  const [step, setStep] = useState<Step>("row");
  const [pick, setPick] = useState<Pick | null>(null);
  const lastCtx = useRef<OutcomeContext | null>(null);
  const draftKey = `deny:${rowId}`;
  const [reason, setReason] = useState(() => readDraft(draftKey));

  const actionable = useMemo(() => descriptors.filter((d) => KNOWN_KINDS.has(d.kind)), [descriptors]);
  const approve = actionable.find((d) => d.kind === ActionKind.APPROVE);
  const deny = actionable.find((d) => d.kind === ActionKind.DENY);
  const others = actionable.filter((d) => d !== approve && d !== deny);
  const hasApproval = !!(approve || deny);
  const failClosed = hasApproval && approval?.unrenderable === true;

  const pending = state.status === "pending";
  const failed = state.status === "failed";
  const view = useMemo(() => (state.status === "settled" && lastCtx.current ? outcomeView(state.response, lastCtx.current) : null), [state]);
  const recovering = failed || (!!view && !view.settled && view.retry !== "countdown");
  const countdown = useCountdown(view?.retry === "countdown" ? view.countdownSeconds : undefined, reset);
  const blocked = pending || countdown > 0 || !!offlineReason;

  const dispatch = useCallback(
    (spec: ActionSpec, ctx: OutcomeContext) => {
      lastCtx.current = ctx;
      focusWasInside.current = !!rootRef.current?.contains(document.activeElement);
      setStep("row");
      return invoke(spec);
    },
    [invoke],
  );

  // Sub-steps and drafts keep the card engaged; a final state releases it (design/ux.md 7.1).
  const draftOpen = reason.trim() !== "";
  useEffect(() => {
    engagement.setSubStep(step !== "row" || draftOpen);
    return () => engagement.setSubStep(false);
  }, [engagement, step, draftOpen]);
  const finalState = !!view?.settled;
  useEffect(() => {
    engagement.setResolved(finalState);
    return () => engagement.setResolved(false);
  }, [engagement, finalState]);
  // The pressed button leaves with the row's controls; keep focus in the card on the result line.
  useEffect(() => {
    if (finalState && focusWasInside.current) {
      focusWasInside.current = false;
      tracker?.focusProgrammatically(statusRef.current);
    }
  }, [finalState, tracker]);

  // One short result per settled outcome, through the panel's single polite region.
  useEffect(() => {
    if (state.status !== "settled" || !view) return;
    if (view.announce) announce(view.announce, "polite", `action-${rowId}`);
    if (state.response.outcome === ActionOutcome.APPLIED) onApplied?.(state.spec.kind);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state]);

  const run = (p: Pick, opts?: Options) =>
    dispatch(buildSpec(p, opts), {
      kind: p.descriptor.kind,
      tier: p.descriptor.tier,
      chosenLabel: p.option?.label,
      ruleName: approval?.ruleName,
    });

  const onPress = (p: Pick) => {
    if (p.descriptor.kind === ActionKind.DENY) {
      setPick(p);
      setStep("deny");
    } else if (needsConfirm(p)) {
      setPick(p);
      setStep("confirm");
    } else {
      void run(p);
    }
  };

  // Focus follows the operator's own sub-step; Back returns it to the control that opened it.
  const fieldRef = useRef<HTMLInputElement>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);
  const moreRef = useRef<HTMLButtonElement>(null);
  const firstMenuItemRef = useRef<HTMLButtonElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  // The opener may unmount when the sub-step replaces the row, so remember it by test id.
  const opener = useRef<string | null>(null);
  useEffect(() => {
    if (step === "deny") fieldRef.current?.focus();
    else if (step === "more") firstMenuItemRef.current?.focus();
    else if (step !== "row") confirmRef.current?.focus();
  }, [step]);
  const rememberOpener = () => {
    opener.current = document.activeElement?.getAttribute("data-testid") ?? null;
  };
  const open = (next: Step) => {
    rememberOpener();
    setStep(next);
  };
  const back = () => {
    const id = opener.current;
    setStep("row");
    requestAnimationFrame(() => {
      if (id) rootRef.current?.querySelector<HTMLElement>(`[data-testid="${id}"]`)?.focus();
    });
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "Escape" || step === "row" || e.nativeEvent.isComposing) return;
    // The innermost layer closes first; the panel's own Esc handler honours defaultPrevented.
    e.preventDefault();
    e.stopPropagation();
    back();
  };

  const reasonFor = (d: NotificationActionDescriptor) =>
    !d.available ? `${uid}-${kindSlug(d.kind)}` : offlineReason ? `${uid}-offline` : undefined;
  const ui: ButtonsUi = {
    blocked,
    mobile,
    minHeight,
    pendingKind: state.status === "pending" ? state.spec.kind : undefined,
    reasonFor,
    onPress: (p) => {
      rememberOpener();
      onPress(p);
    },
  };

  const openButton = openControl !== undefined ? openControl : (
    <ActionButton id="open" label={COPY.openSession} variant={secondaryVariant} minHeight={minHeight} onClick={() => onOpen?.()} />
  );
  const statusLine = (text: string) => (
    <p ref={statusRef} tabIndex={-1} className={status} data-testid="action-status">
      {text}
    </p>
  );

  const subStep = (): ReactNode => {
    const common = { onBack: back, minHeight, busy: pending };
    if (step === "deny" && pick) {
      return (
        <DenyStep
          {...common}
          fieldId={`${uid}-reason`}
          fieldRef={fieldRef}
          reason={reason}
          onReason={(v) => {
            setReason(v);
            writeDraft(draftKey, v);
          }}
          onSend={() => {
            clearDraft(draftKey);
            void run(pick, { reason: reason.trim() || undefined });
          }}
        />
      );
    }
    if (step === "confirm" && pick) {
      return <ConfirmStep {...common} {...confirmCopy(pick, prNumber)} confirmRef={confirmRef} onConfirm={() => void run(pick, { confirm: true })} />;
    }
    if (step === "retry-anyway" && state.status === "settled" && lastCtx.current) {
      const { spec } = state;
      const ctx = lastCtx.current;
      return (
        <ConfirmStep
          {...common}
          prompt={COPY.retryAnywayPrompt}
          confirmLabel={COPY.retryAnywayConfirm}
          backLabel={COPY.back}
          confirmRef={confirmRef}
          confirmId="retry-confirm"
          testId="retry-anyway-step"
          onConfirm={() => void dispatch({ ...spec, params: { ...spec.params, confirm: true } }, ctx)}
        />
      );
    }
    if (step === "create-rule") {
      return (
        <ConfirmStep
          {...common}
          {...createRuleStepProps(approval?.ruleScope ?? approval?.tool ?? "this tool")}
          backLabel={COPY.back}
          confirmRef={confirmRef}
          onConfirm={() => {
            setStep("row");
            onCreateRule?.();
          }}
        />
      );
    }
    return null;
  };

  const showMore = hasApproval && !failClosed && !!onCreateRule && isInnerScreen && localListener;
  const sub = subStep();

  let body: ReactNode;
  if (view?.settled) {
    body = (
      <>
        {statusLine(view.text)}
        <div className={buttons}>{openButton}</div>
      </>
    );
  } else if (recovering) {
    const retryHere = failed || view?.retry === "direct";
    body = (
      <>
        {statusLine(failed ? COPY.transient : (view?.text ?? ""))}
        {sub ?? (
          <div className={buttons}>
            {view?.openPrimary && openButton}
            {retryHere && <ActionButton id="retry" label={COPY.retry} minHeight={minHeight} disabled={pending} onClick={() => void retry()} />}
            {view?.retry === "confirm" && (
              <ActionButton id="retry-anyway" label={COPY.retryAnyway} minHeight={minHeight} onClick={() => open("retry-anyway")} />
            )}
            {view?.refresh && (
              <ActionButton
                id="refresh"
                label={COPY.refresh}
                minHeight={minHeight}
                onClick={() => {
                  reset();
                  onRefresh?.();
                }}
              />
            )}
            {failed && <ActionButton id="dismiss" label={COPY.dismiss} minHeight={minHeight} onClick={reset} />}
            {!view?.openPrimary && openButton}
          </div>
        )}
      </>
    );
  } else {
    const showRow = step === "row" || step === "more";
    body = (
      <>
        {hasApproval && !failClosed && <ApprovalCommand tool={approval?.tool} text={approval?.text} />}
        {failClosed && (
          <p className={status} data-testid="approval-unrenderable">
            {approval?.tool ? `${approval.tool}. ` : ""}
            {COPY.cantShowCommand}
          </p>
        )}
        {showRow ? (
          <div className={buttons} role="group" aria-label="Actions">
            <ApprovalButtons approve={failClosed ? undefined : approve} deny={deny} ui={ui} />
            {showMore && (
              <MoreMenu
                open={step === "more"}
                blocked={blocked}
                minHeight={minHeight}
                buttonRef={moreRef}
                firstItemRef={firstMenuItemRef}
                onToggle={() => (step === "more" ? back() : open("more"))}
                onCreateRule={() => setStep("create-rule")}
              />
            )}
            <OtherButtons descriptors={others} ui={ui} />
            {openButton}
          </div>
        ) : (
          sub
        )}
        {countdown > 0 && statusLine(rateLimited(countdown))}
        {pending && <PendingNotice pending onRetry={() => void retry()} />}
      </>
    );
  }

  return (
    <div ref={rootRef} className={root} data-testid="action-row" aria-busy={pending ? true : undefined} onKeyDown={onKeyDown}>
      {body}
      {offlineReason && (
        <span id={`${uid}-offline`} className={status} data-testid="row-offline-reason">
          {offlineReason}
        </span>
      )}
      {actionable
        .filter((d) => !d.available)
        .map((d) => (
          <span key={`${d.kind}-${d.targetKey}`} id={`${uid}-${kindSlug(d.kind)}`} className={status} data-testid="action-unavailable-reason">
            {unavailableReason(d)}
          </span>
        ))}
    </div>
  );
}
