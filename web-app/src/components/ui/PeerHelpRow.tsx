// +feature: peer-help-rows
"use client";

import { useRef, useState } from "react";
import { Link } from "@/lib/nav";
import type { PeerHelpInfo } from "@/gen/session/v1/wake_trigger_pb";
import { ActionKind, ActionOutcome, ActionTargetKey } from "@/gen/session/v1/notification_action_pb";
import { invokeNotificationAction } from "@/lib/api/notificationActionApi";
import { PEER_COPY } from "@/lib/copy/scheduledPromptsCopy";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { ageText, tsMs } from "@/lib/utils/scheduledPrompts";
import { action, row as rowClass, rowActions, rowHead, rowInfo, rowMessage, rowMeta, rowNeedsHuman, rowTitle, roleTag, stateChip } from "./BackgroundActivity.css";
import { InlineConfirm, InlineError } from "./InlineConfirm";
import { answerField, fieldLabel, inlineMessage } from "./ScheduledPrompts.css";

export interface PeerHelpRowProps {
  info: PeerHelpInfo;
  /** Called after a change the list should re-read (answered, cancelled, raced). */
  onChanged: () => void;
  now?: number;
}

type Ui =
  | { phase: "idle" }
  | { phase: "answering" }
  | { phase: "confirmCancel" }
  | { phase: "busy"; kind: "answer" | "cancel" }
  | { phase: "error"; retry: "answer" | "cancel"; message: string };

const sessionHref = (id: string) => `/?session=${encodeURIComponent(id)}&tab=terminal`;

/** The outcome of a peer-help action reduced to what the row shows. */
function failureOf(outcome: ActionOutcome, fallback: string): string {
  return outcome === ActionOutcome.FLAG_OFF ? PEER_COPY.flagOff : fallback;
}

/**
 * One agent-to-agent help request (UX-S4). It never touches the notification store, so a
 * waiting request cannot move the Needs-you badge; only the escalation notification the
 * server raises after 10 minutes or a peer ending does.
 */
export function PeerHelpRow({ info, onChanged, now = Date.now() }: PeerHelpRowProps) {
  const { announce } = useAnnounce();
  const [ui, setUi] = useState<Ui>({ phase: "idle" });
  const [draft, setDraft] = useState("");
  const [raceNote, setRaceNote] = useState<string | null>(null);
  const attempt = useRef(0);
  const asker = info.askerTitle || info.askerSessionUuid;
  const peer = info.answererTitle || info.answererSessionUuid;
  const open = info.state === "waiting" || info.state === "not_sent";
  const stalled = open && info.escalated;
  const age = ageText(now - tsMs(info.createdAt));

  const invoke = async (kind: ActionKind, params: { answer?: string }): Promise<{ outcome: ActionOutcome; detail: string } | null> => {
    attempt.current += 1;
    setUi({ phase: "busy", kind: kind === ActionKind.CANCEL_PEER_HELP ? "cancel" : "answer" });
    try {
      const res = await invokeNotificationAction({
        actionKind: kind,
        targetKeyType: ActionTargetKey.GUIDANCE_REQUEST_ID,
        targetKey: info.id,
        sessionId: "",
        params,
        idempotencyKey: `peer:${kind}:${info.id}:${attempt.current}`,
      });
      return { outcome: res.outcome, detail: res.detail };
    } catch {
      return null;
    }
  };

  const settle = (res: { outcome: ActionOutcome; detail: string } | null, retry: "answer" | "cancel", done: string, failed: string) => {
    if (res?.outcome === ActionOutcome.APPLIED) {
      setUi({ phase: "idle" });
      announce(done, "polite", "peer-help-action");
      onChanged();
    } else if (res?.outcome === ActionOutcome.ALREADY_RESOLVED) {
      setUi({ phase: "idle" });
      setRaceNote(res.detail || PEER_COPY.alreadyAnsweredElsewhere);
      onChanged();
    } else {
      setUi({ phase: "error", retry, message: failureOf(res?.outcome ?? ActionOutcome.UNSPECIFIED, failed) });
    }
  };

  const sendAnswer = async () => {
    settle(await invoke(ActionKind.ANSWER_GUIDANCE, { answer: draft.trim() }), "answer", PEER_COPY.answerSent, PEER_COPY.answerFailed);
  };
  const cancelRequest = async () => {
    settle(await invoke(ActionKind.CANCEL_PEER_HELP, {}), "cancel", PEER_COPY.requestCancelled, PEER_COPY.cancelRequestFailed);
  };

  const heading = stalled
    ? info.stateReason && info.state === "waiting"
      ? `${asker} asked ${peer}: ${info.stateReason}`
      : info.state === "not_sent"
        ? PEER_COPY.asked(asker, peer)
        : PEER_COPY.noReply(asker, peer)
    : PEER_COPY.asked(asker, peer);

  return (
    <li
      className={`${rowClass} ${stalled ? rowNeedsHuman : rowInfo}`}
      data-testid="peer-help-row"
      data-state={info.state}
      data-escalated={stalled ? "true" : "false"}
      data-request-id={info.id}
    >
      <div className={rowHead}>
        <span className={roleTag}>{stalled ? PEER_COPY.stalledTag : PEER_COPY.tag}</span>
        <strong className={rowTitle}>{heading}</strong>
        {info.state === "waiting" && <span className={stateChip}>{PEER_COPY.waitingAge(age)}</span>}
      </div>
      <p className={rowMessage}>{info.question}</p>
      <div className={rowMeta} data-testid="peer-help-state">
        {stateLine(info, peer, raceNote)}
      </div>

      {ui.phase === "error" && (
        <InlineError
          message={ui.message}
          onRetry={ui.message === PEER_COPY.flagOff ? undefined : () => void (ui.retry === "answer" ? sendAnswer() : cancelRequest())}
          onDismiss={() => setUi({ phase: "idle" })}
        />
      )}
      {(ui.phase === "confirmCancel" || (ui.phase === "busy" && ui.kind === "cancel")) && (
        <InlineConfirm
          question={PEER_COPY.cancelRequestQuestion}
          yesLabel="Yes, cancel"
          keepLabel="Keep"
          busy={ui.phase === "busy"}
          onYes={() => void cancelRequest()}
          onKeep={() => setUi({ phase: "idle" })}
        />
      )}
      {(ui.phase === "answering" || (ui.phase === "busy" && ui.kind === "answer")) && answerForm(ui.phase === "busy")}

      <div className={rowActions}>
        {stalled && ui.phase === "idle" && (
          <>
            {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
            <button type="button" className={action} data-testid="peer-help-answer" onClick={() => setUi({ phase: "answering" })}>
              {PEER_COPY.answerFor(peer)}
            </button>
            {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
            <button type="button" className={action} data-testid="peer-help-cancel" onClick={() => setUi({ phase: "confirmCancel" })}>
              {PEER_COPY.cancelRequest}
            </button>
          </>
        )}
        <Link href={sessionHref(info.askerSessionUuid)} className={action} data-testid="peer-help-open-asker">
          {stalled ? PEER_COPY.open : PEER_COPY.openAsker}
        </Link>
        {!stalled && info.state !== "not_sent" && (
          <Link href={sessionHref(info.answererSessionUuid)} className={action} data-testid="peer-help-open-peer">
            {PEER_COPY.openPeer}
          </Link>
        )}
      </div>
    </li>
  );

  function answerForm(busy: boolean) {
    return (
      <div className={inlineMessage}>
        <label className={fieldLabel} htmlFor={`peer-answer-${info.id}`}>
          {PEER_COPY.answerLabel(peer)}
        </label>
        <textarea
          id={`peer-answer-${info.id}`}
          className={answerField}
          value={draft}
          autoFocus
          disabled={busy}
          onChange={(e) => setDraft(e.target.value)}
        />
        <div className={rowActions}>
          {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
          <button
            type="button"
            className={action}
            data-testid="peer-help-send"
            disabled={busy || draft.trim() === ""}
            // eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step
            onClick={() => void sendAnswer()}
          >
            {busy ? PEER_COPY.sending : PEER_COPY.send}
          </button>
          {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
          <button type="button" className={action} disabled={busy} onClick={() => setUi({ phase: "idle" })}>
            Keep
          </button>
        </div>
      </div>
    );
  }
}

function stateLine(info: PeerHelpInfo, peer: string, raceNote: string | null): string {
  switch (info.state) {
    case "answered":
      if (raceNote) return PEER_COPY.alreadyAnswered(info.answeredBy || peer, info.answer);
      return info.onBehalfOf
        ? PEER_COPY.answeredOnBehalf(info.onBehalfOf)
        : `${PEER_COPY.answeredBy(info.answeredBy || peer)} (${PEER_COPY.provenanceFrom(info.answeredBy || peer)})`;
    case "not_sent":
      return PEER_COPY.notSent(info.stateReason);
    case "cancelled":
      return info.stateReason || PEER_COPY.cancelled;
    default:
      return raceNote ?? (info.stateReason ? info.stateReason : PEER_COPY.waitingFor(peer));
  }
}
