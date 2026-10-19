// +feature: remind-me
"use client";

import { useEffect, useId, useRef, useState } from "react";
import { ReminderOutcome } from "@/gen/session/v1/wake_trigger_pb";
import { cancelWakeTrigger, createOperatorReminder } from "@/lib/api/wakeTriggerApi";
import { REMIND_COPY } from "@/lib/copy/scheduledPromptsCopy";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { action, rowActions } from "./BackgroundActivity.css";
import { InlineError } from "./InlineConfirm";
import { formatClockTime } from "./OfflineBanner";
import { fieldLabel, inlineMessage, menuItem, menuList, pickField } from "./ScheduledPrompts.css";

/** How long the confirmation offers Undo (UX-AC40). */
export const REMINDER_UNDO_MS = 10_000;
const TOMORROW_HOUR = 9;

export interface ReminderPreset {
  id: string;
  label: string;
  /** The fire time for a choice made at `now`. */
  at: (now: Date) => Date;
}

/** The shipped presets; the owner can change these values, which blocks nothing. */
export const DEFAULT_PRESETS: readonly ReminderPreset[] = [
  { id: "hour", label: REMIND_COPY.inOneHour, at: (now) => new Date(now.getTime() + 60 * 60 * 1000) },
  {
    id: "tomorrow",
    label: REMIND_COPY.tomorrow,
    at: (now) => {
      const d = new Date(now);
      d.setDate(d.getDate() + 1);
      d.setHours(TOMORROW_HOUR, 0, 0, 0);
      return d;
    },
  },
];

export interface RemindMeMenuProps {
  /** The session the reminder is about; it may be archived or stopped by the time it fires. */
  sessionUuid: string;
  /** Text of the reminder notification ("Review finished: pr-1042"). */
  label: string;
  presets?: readonly ReminderPreset[];
  /** Where "Scheduled prompts" lives, for the cap message link. */
  scheduledHref?: string;
  /** The clock; tests inject a fixed one. */
  now?: () => Date;
}

type Phase =
  | { kind: "closed" }
  | { kind: "menu" }
  | { kind: "pick"; error?: string }
  | { kind: "busy" }
  | { kind: "set"; at: Date; triggerId: string; undoable: boolean }
  | { kind: "undone" }
  | { kind: "failed"; at: Date }
  | { kind: "cap" };

const toLocalInput = (d: Date): string => {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
};

/**
 * "Remind me" on an informational Background row (UX-S6): it creates a notify_operator
 * trigger and never appears on a blocking question. The host decides where to render it.
 */
export function RemindMeMenu({ sessionUuid, label, presets = DEFAULT_PRESETS, scheduledHref = "#scheduled-prompts-heading", now = () => new Date() }: RemindMeMenuProps) {
  const { announce } = useAnnounce();
  const [phase, setPhase] = useState<Phase>({ kind: "closed" });
  const [pickValue, setPickValue] = useState("");
  const button = useRef<HTMLButtonElement>(null);
  const items = useRef<Array<HTMLButtonElement | null>>([]);
  const menuId = useId();
  const pickId = useId();

  // Undo is offered for 10 s, then the reminder just persists in Scheduled prompts.
  const undoTrigger = phase.kind === "set" && phase.undoable ? phase.triggerId : null;
  useEffect(() => {
    if (!undoTrigger) return;
    const timer = setTimeout(() => setPhase((p) => (p.kind === "set" ? { ...p, undoable: false } : p)), REMINDER_UNDO_MS);
    return () => clearTimeout(timer);
  }, [undoTrigger]);

  useEffect(() => {
    if (phase.kind === "menu") items.current[0]?.focus();
  }, [phase.kind]);

  const create = async (at: Date) => {
    if (at.getTime() <= now().getTime()) {
      setPhase({ kind: "pick", error: REMIND_COPY.past });
      return;
    }
    setPhase({ kind: "busy" });
    try {
      const res = await createOperatorReminder({ sessionUuid, fireAt: at, label });
      switch (res.outcome) {
        case ReminderOutcome.CREATED:
        case ReminderOutcome.DUPLICATE:
          setPhase({ kind: "set", at, triggerId: res.trigger?.id ?? "", undoable: res.outcome === ReminderOutcome.CREATED && !!res.trigger });
          announce(REMIND_COPY.set(formatClockTime(at.getTime())), "polite", "remind-set");
          break;
        case ReminderOutcome.PAST_TIME:
          setPhase({ kind: "pick", error: REMIND_COPY.past });
          break;
        case ReminderOutcome.CAP_EXCEEDED:
          setPhase({ kind: "cap" });
          break;
        default:
          setPhase({ kind: "failed", at });
      }
    } catch {
      setPhase({ kind: "failed", at });
    }
  };

  const undo = async () => {
    if (phase.kind !== "set") return;
    try {
      await cancelWakeTrigger(phase.triggerId);
      setPhase({ kind: "undone" });
      announce(REMIND_COPY.undone, "polite", "remind-set");
    } catch {
      setPhase({ ...phase, undoable: false });
    }
  };

  const closeMenu = () => {
    setPhase({ kind: "closed" });
    button.current?.focus();
  };

  const onMenuKey = (e: React.KeyboardEvent) => {
    const list = items.current.filter((n): n is HTMLButtonElement => n !== null);
    const at = list.indexOf(document.activeElement as HTMLButtonElement);
    const go = (i: number) => {
      e.preventDefault();
      list[(i + list.length) % list.length]?.focus();
    };
    if (e.key === "ArrowDown") go(at + 1);
    else if (e.key === "ArrowUp") go(at - 1);
    else if (e.key === "Home") go(0);
    else if (e.key === "End") go(list.length - 1);
    else if (e.key === "Escape") {
      e.stopPropagation();
      closeMenu();
    }
  };

  const choices = [...presets.map((p) => ({ id: p.id, label: p.label })), { id: "pick", label: REMIND_COPY.pickTime }];

  return (
    <div data-testid="remind-me" onKeyDown={phase.kind === "menu" ? onMenuKey : undefined}>
      {(phase.kind === "closed" || phase.kind === "menu") && (
        // analytics-exempt
        <button
          type="button"
          ref={button}
          className={action}
          aria-haspopup="menu"
          aria-expanded={phase.kind === "menu"}
          aria-controls={menuId}
          data-testid="remind-me-button"
          onClick={() => setPhase(phase.kind === "menu" ? { kind: "closed" } : { kind: "menu" })}
        >
          {REMIND_COPY.button}
        </button>
      )}
      {phase.kind === "menu" && (
        <ul id={menuId} role="menu" aria-label={REMIND_COPY.menuLabel} className={menuList}>
          {choices.map((c, i) => (
            <li key={c.id} role="none">
              {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
              <button
                type="button"
                role="menuitem"
                ref={(n) => {
                  items.current[i] = n;
                }}
                className={`${action} ${menuItem}`}
                data-testid={`remind-me-${c.id}`}
                // eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step
                onClick={() => {
                  if (c.id === "pick") setPhase({ kind: "pick" });
                  else {
                    const preset = presets.find((p) => p.id === c.id);
                    if (preset) void create(preset.at(now()));
                  }
                }}
              >
                {c.label}
              </button>
            </li>
          ))}
        </ul>
      )}
      {phase.kind === "pick" && (
        <div>
          <label className={fieldLabel} htmlFor={pickId}>
            {REMIND_COPY.pickLabel}
          </label>
          <input
            id={pickId}
            type="datetime-local"
            className={pickField}
            value={pickValue}
            min={toLocalInput(now())}
            onChange={(e) => setPickValue(e.target.value)}
          />
          {phase.error && <p className={inlineMessage}>{phase.error}</p>}
          <div className={rowActions}>
            {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
            <button type="button" className={action} data-testid="remind-me-set-pick" disabled={pickValue === ""} onClick={() => void create(new Date(pickValue))}>
              {REMIND_COPY.setPick}
            </button>
            {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
            <button type="button" className={action} onClick={closeMenu}>
              Keep
            </button>
          </div>
        </div>
      )}
      {phase.kind === "busy" && <p className={inlineMessage}>Setting the reminder...</p>}
      {phase.kind === "set" && (
        <div className={rowActions} data-testid="remind-me-confirmation">
          <p className={inlineMessage}>{REMIND_COPY.set(formatClockTime(phase.at.getTime()))}</p>
          {phase.undoable && (
            // analytics-exempt
            <button type="button" className={action} data-testid="remind-me-undo" onClick={() => void undo()}>
              {REMIND_COPY.undo}
            </button>
          )}
        </div>
      )}
      {phase.kind === "undone" && <p className={inlineMessage}>{REMIND_COPY.undone}</p>}
      {phase.kind === "failed" && (
        <InlineError message={REMIND_COPY.failed} onRetry={() => void create(phase.at)} onDismiss={() => setPhase({ kind: "closed" })} />
      )}
      {phase.kind === "cap" && (
        <div data-testid="remind-me-cap">
          <p className={inlineMessage}>{REMIND_COPY.cap}</p>
          <a className={action} href={scheduledHref}>
            {REMIND_COPY.capLink}
          </a>
        </div>
      )}
    </div>
  );
}
