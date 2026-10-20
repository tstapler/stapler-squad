// +feature: scheduled-prompts
"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import type { WakeTriggerInfo } from "@/gen/session/v1/wake_trigger_pb";
import { WakeCancelOutcome } from "@/gen/session/v1/wake_trigger_pb";
import { cancelWakeTrigger } from "@/lib/api/wakeTriggerApi";
import { SCHEDULED_COPY, missedReason } from "@/lib/copy/scheduledPromptsCopy";
import type { ScheduledPromptsState } from "@/lib/hooks/useScheduledPrompts";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { routes } from "@/lib/routes";
import { ageText, conditionText, sortTriggers, tsMs } from "@/lib/utils/scheduledPrompts";
import {
  action,
  groupLabel,
  row as rowClass,
  rowActions,
  rowHead,
  rowInfo,
  rowList,
  rowMessage,
  rowMeta,
  rowNeedsHuman,
  rowTitle,
  section,
  staleLine,
  stateChip,
  statusLabel,
  statusLabelNeedsHuman,
} from "./BackgroundActivity.css";
import { InlineConfirm, InlineError } from "./InlineConfirm";
import { formatClockTime } from "./OfflineBanner";
import { PeerHelpRow } from "./PeerHelpRow";
import { inlineMessage, pausedBanner, sectionHead } from "./ScheduledPrompts.css";

export const SCHEDULED_ROW_CAP = 20;

export interface ScheduledPromptsProps {
  state: ScheduledPromptsState;
  /** Turns agent_wake_triggers back on (the host's flag writer); the section re-reads afterwards. */
  resumeFlag: () => Promise<void>;
  rowCap?: number;
  now?: number;
}

type RowUi =
  | { phase: "idle" }
  | { phase: "confirm" }
  | { phase: "busy" }
  | { phase: "fired"; at: number }
  | { phase: "ended" }
  | { phase: "error" };

const sessionHref = (id: string) => `/?session=${encodeURIComponent(id)}&tab=terminal`;

function WakeRow({
  trigger,
  paused,
  now,
  onCancelled,
}: {
  trigger: WakeTriggerInfo;
  paused: boolean;
  now: number;
  onCancelled: (id: string) => void;
}) {
  const { announce } = useAnnounce();
  const [ui, setUi] = useState<RowUi>({ phase: "idle" });
  const missed = trigger.status === "missed";
  const reminder = trigger.operatorReminder;

  const cancel = async () => {
    setUi({ phase: "busy" });
    try {
      const res = await cancelWakeTrigger(trigger.id);
      if (res.outcome === WakeCancelOutcome.CANCELLED) {
        announce(reminder ? SCHEDULED_COPY.reminderCancelled : SCHEDULED_COPY.promptCancelled, "polite", "scheduled-cancel");
        onCancelled(trigger.id);
      } else if (res.outcome === WakeCancelOutcome.ALREADY_FIRED) {
        // The row stays readable until the next poll drops it; re-reading now would erase the note.
        setUi({ phase: "fired", at: tsMs(res.deliveredAt) });
      } else {
        setUi({ phase: "ended" });
      }
    } catch {
      setUi({ phase: "error" });
    }
  };

  const title = trigger.ownerExists ? trigger.ownerTitle : SCHEDULED_COPY.ownerGone;
  const next = trigger.nextFireAt ? SCHEDULED_COPY.nextAt(formatClockTime(tsMs(trigger.nextFireAt))) : SCHEDULED_COPY.nextEvent;
  const settled = ui.phase === "fired" || ui.phase === "ended";

  return (
    <li
      className={`${rowClass} ${missed ? rowNeedsHuman : rowInfo}`}
      data-testid="scheduled-prompt-row"
      data-trigger-id={trigger.id}
      data-status={trigger.status}
    >
      <div className={rowHead}>
        {missed && <span aria-hidden="true">!</span>}
        {trigger.ownerExists ? (
          <Link href={sessionHref(trigger.ownerSessionUuid)} className={rowTitle} data-testid="scheduled-owner-link">
            {title}
          </Link>
        ) : (
          <strong className={rowTitle}>{title}</strong>
        )}
        {reminder && <span className={stateChip}>{SCHEDULED_COPY.reminderBadge}</span>}
        {paused && <span className={stateChip}>{SCHEDULED_COPY.pausedLabel}</span>}
        {missed && (
          <span className={`${statusLabel} ${statusLabelNeedsHuman}`} data-testid="scheduled-missed">
            {SCHEDULED_COPY.missed(missedReason(trigger.endReason))}
          </span>
        )}
      </div>
      {!missed && <div className={rowMeta}>{conditionText(trigger, formatClockTime)}</div>}
      <div className={rowMeta}>
        {SCHEDULED_COPY.sinceAgo(ageText(now - tsMs(trigger.createdAt)))}
        {missed ? "" : ` - ${next}`}
      </div>
      {trigger.message && <p className={rowMessage}>{trigger.message}</p>}
      {trigger.ownerExists && trigger.ownerArchived && <div className={rowMeta}>{SCHEDULED_COPY.ownerArchived}</div>}
      {ui.phase === "fired" && <p className={inlineMessage}>{SCHEDULED_COPY.alreadyFired(formatClockTime(ui.at))}</p>}
      {ui.phase === "ended" && <p className={inlineMessage}>{SCHEDULED_COPY.alreadyEnded}</p>}
      {ui.phase === "error" && (
        <InlineError
          message={SCHEDULED_COPY.cancelFailed}
          onRetry={() => void cancel()}
          onDismiss={() => setUi({ phase: "idle" })}
        />
      )}
      {(ui.phase === "confirm" || ui.phase === "busy") && (
        <InlineConfirm
          question={reminder ? SCHEDULED_COPY.cancelReminderQuestion : SCHEDULED_COPY.cancelQuestion}
          yesLabel={SCHEDULED_COPY.yesCancel}
          keepLabel={SCHEDULED_COPY.keep}
          busy={ui.phase === "busy"}
          onYes={() => void cancel()}
          onKeep={() => setUi({ phase: "idle" })}
        />
      )}
      {ui.phase === "idle" && !settled && (
        <div className={rowActions}>
          {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
          <button type="button" className={action} data-testid="scheduled-cancel" onClick={() => setUi({ phase: "confirm" })}>
            {reminder ? SCHEDULED_COPY.cancelReminder : SCHEDULED_COPY.cancelPrompt}
          </button>
          {missed && trigger.ownerExists && (
            <Link href={sessionHref(trigger.ownerSessionUuid)} className={action} data-testid="scheduled-open-owner">
              {SCHEDULED_COPY.openOwner}
            </Link>
          )}
        </div>
      )}
    </li>
  );
}

function PausedBanner({ resumeFlag, refresh }: { resumeFlag: () => Promise<void>; refresh: () => Promise<boolean | null> }) {
  const { announce } = useAnnounce();
  const [phase, setPhase] = useState<"idle" | "confirm" | "busy" | "error">("idle");
  const resume = async () => {
    setPhase("busy");
    try {
      await resumeFlag();
      // The server's read-back decides: a flag write that was ignored leaves it off.
      if ((await refresh()) === true) {
        announce("Scheduled prompts resumed", "polite", "scheduled-resume");
        setPhase("idle");
        return;
      }
    } catch {
      // falls through to the error state
    }
    setPhase("error");
  };
  return (
    <div className={pausedBanner} data-testid="scheduled-paused">
      <p className={inlineMessage}>{SCHEDULED_COPY.paused}</p>
      {phase === "error" && (
        <InlineError message={SCHEDULED_COPY.resumeFailed} onRetry={() => void resume()} onDismiss={() => setPhase("idle")} />
      )}
      {(phase === "confirm" || phase === "busy") && (
        <InlineConfirm
          question={SCHEDULED_COPY.resumeQuestion}
          yesLabel={SCHEDULED_COPY.resumeYes}
          keepLabel={SCHEDULED_COPY.resumeKeep}
          busy={phase === "busy"}
          onYes={() => void resume()}
          onKeep={() => setPhase("idle")}
        />
      )}
      <div className={rowActions}>
        {phase === "idle" && (
          // analytics-exempt
          <button type="button" className={action} data-testid="scheduled-resume" onClick={() => setPhase("confirm")}>
            {SCHEDULED_COPY.resume}
          </button>
        )}
        <Link href={routes.settingsFeatures} className={action} data-testid="scheduled-flag-settings">
          {SCHEDULED_COPY.flagSettings}
        </Link>
      </div>
    </div>
  );
}

/**
 * The Background tab's "Scheduled prompts" section (UX-S5): every pending trigger and open
 * peer-help request with a Cancel control, the kill-switch banner, and a way out of every
 * failure. Presentational: the host owns polling (`useScheduledPrompts`) and the flag write.
 */
export function ScheduledPrompts({ state, resumeFlag, rowCap = SCHEDULED_ROW_CAP, now = Date.now() }: ScheduledPromptsProps) {
  const [expanded, setExpanded] = useState(true);
  const [showAll, setShowAll] = useState(false);
  const [gone, setGone] = useState<ReadonlySet<string>>(new Set());
  const heading = useRef<HTMLHeadingElement>(null);
  const focusHeading = useRef(false);

  useEffect(() => {
    if (focusHeading.current) {
      focusHeading.current = false;
      heading.current?.focus();
    }
  });

  const triggers = sortTriggers(state.triggers.filter((t) => !gone.has(t.id)));
  const openPeers = state.peers.filter((p) => p.state === "waiting" || p.state === "not_sent");
  const total = triggers.length + openPeers.length;
  const loaded = state.lastUpdatedAt !== null;

  if (!loaded && state.failed) {
    return (
      <section className={section} data-testid="scheduled-prompts-error">
        <p className={inlineMessage}>{SCHEDULED_COPY.loadFailed}</p>
        <div className={rowActions}>
          {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
          <button type="button" className={action} data-testid="scheduled-retry" onClick={() => void state.refresh()}>
            {SCHEDULED_COPY.retry}
          </button>
        </div>
      </section>
    );
  }
  if (!loaded || (!state.enabled && total === 0)) return null;

  const wakeLimit = showAll ? triggers.length : Math.min(triggers.length, rowCap);
  const peerLimit = showAll ? openPeers.length : Math.max(0, Math.min(openPeers.length, rowCap - wakeLimit));
  const hidden = total - wakeLimit - peerLimit;

  return (
    <section className={section} data-testid="scheduled-prompts" aria-labelledby="scheduled-prompts-heading">
      <div className={sectionHead}>
        <h3 id="scheduled-prompts-heading" className={groupLabel} tabIndex={-1} ref={heading}>
          {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
          <button
            type="button"
            className={action}
            aria-expanded={expanded}
            aria-controls="scheduled-prompts-body"
            data-testid="scheduled-toggle"
            // eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step
            onClick={() => setExpanded((v) => !v)}
          >
            {SCHEDULED_COPY.count(total)}
          </button>
        </h3>
      </div>
      <div id="scheduled-prompts-body" hidden={!expanded}>
        {!state.enabled && <PausedBanner resumeFlag={resumeFlag} refresh={state.refresh} />}
        {state.failed && (
          <div data-testid="scheduled-stale">
            <p className={staleLine}>{SCHEDULED_COPY.loadFailed}</p>
            {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
            <button type="button" className={action} data-testid="scheduled-retry" onClick={() => void state.refresh()}>
              {SCHEDULED_COPY.retry}
            </button>
          </div>
        )}
        {total === 0 && state.enabled && <p className={staleLine} data-testid="scheduled-empty">{SCHEDULED_COPY.empty}</p>}
        {total > 0 && (
          <ul className={rowList} aria-label={SCHEDULED_COPY.heading}>
            {triggers.slice(0, wakeLimit).map((t) => (
              <WakeRow
                key={t.id}
                trigger={t}
                paused={!state.enabled}
                now={now}
                onCancelled={(id) => {
                  focusHeading.current = true;
                  setGone((prev) => new Set(prev).add(id));
                  void state.refresh();
                }}
              />
            ))}
            {openPeers.slice(0, peerLimit).map((p) => (
              <PeerHelpRow key={p.id} info={p} now={now} onChanged={() => void state.refresh()} />
            ))}
          </ul>
        )}
        {hidden > 0 && (
          // analytics-exempt
          <button type="button" className={action} data-testid="scheduled-show-all" onClick={() => setShowAll(true)}>
            {SCHEDULED_COPY.showAll(total)}
          </button>
        )}
      </div>
    </section>
  );
}
