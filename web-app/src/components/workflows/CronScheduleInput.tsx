"use client";
// +feature: workflow-cron-schedule-input

import { useId, useState } from "react";
import { explainCron } from "@/lib/cron/explainCron";
import { validateCron } from "@/lib/cron/validateCron";
import {
  buildCronFromSimple,
  parseCronToSimple,
  DEFAULT_SIMPLE_SCHEDULE,
  type SimpleSchedule,
  type SimpleFrequency,
} from "@/lib/cron/buildCronFromSimple";
import * as styles from "./CronScheduleInput.css";

interface CronScheduleInputProps {
  /** id applied to the widget's group container, referenced by the field's <label>. */
  id: string;
  /** id of the field's <label>, wired via aria-labelledby (no single input owns this widget). */
  labelId: string;
  value: string;
  onChange: (value: string) => void;
}

const DAY_LABELS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

function timeToFields(t: string): { hour: number; minute: number } | null {
  const m = /^(\d{2}):(\d{2})$/.exec(t);
  if (!m) return null;
  return { hour: parseInt(m[1], 10), minute: parseInt(m[2], 10) };
}

function fieldsToTime(hour: number, minute: number): string {
  return `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`;
}

export function CronScheduleInput({ id, labelId, value, onChange }: CronScheduleInputProps) {
  const uid = useId();
  const explanationId = `${id}-explanation`;
  const errorId = `${id}-error`;

  const initialParsed = parseCronToSimple(value);
  const [mode, setMode] = useState<"simple" | "advanced">(
    value.trim() === "" || initialParsed ? "simple" : "advanced"
  );
  const [simple, setSimple] = useState<SimpleSchedule>(initialParsed ?? DEFAULT_SIMPLE_SCHEDULE);
  const [fallbackNotice, setFallbackNotice] = useState(false);

  // In Simple mode, preview the schedule the dropdowns currently describe rather than
  // the committed `value` — so an untouched form (e.g. editing an existing manual
  // workflow) shows an honest preview without onChange firing before the user acts,
  // which would otherwise silently write a cron string into an unrelated form save.
  const explanation =
    mode === "simple" ? explainCron(buildCronFromSimple(simple)) : explainCron(value);
  const validation = validateCron(value.trim());
  const showError = value.trim() !== "" && !validation.valid;

  function emitSimple(next: SimpleSchedule) {
    setSimple(next);
    onChange(buildCronFromSimple(next));
  }

  function handleModeChange(next: "simple" | "advanced") {
    if (next === "advanced") {
      setFallbackNotice(false);
      setMode("advanced");
      return;
    }
    const parsed = parseCronToSimple(value);
    if (!parsed && value.trim() !== "") {
      // Not representable by the builder — stay on Advanced with a raw-editor notice
      // rather than silently discarding step values/ranges/lists.
      setFallbackNotice(true);
      return;
    }
    setFallbackNotice(false);
    setSimple(parsed ?? DEFAULT_SIMPLE_SCHEDULE);
    if (!parsed) onChange(buildCronFromSimple(DEFAULT_SIMPLE_SCHEDULE));
    setMode("simple");
  }

  return (
    <div id={id} className={styles.wrapper} role="group" aria-labelledby={labelId}>
      <div className={styles.modeToggle} role="radiogroup" aria-label="Schedule entry mode">
        <label className={styles.modeOption}>
          <input
            type="radio"
            name={`${uid}-cron-mode`}
            checked={mode === "simple"}
            onChange={() => handleModeChange("simple")}
          />
          Simple
        </label>
        <label className={styles.modeOption}>
          <input
            type="radio"
            name={`${uid}-cron-mode`}
            checked={mode === "advanced"}
            onChange={() => handleModeChange("advanced")}
          />
          Advanced
        </label>
      </div>

      {fallbackNotice && (
        <div className={styles.notice} role="status">
          This expression uses step values, ranges, or lists the Simple builder can&apos;t
          represent — edit it directly below.
        </div>
      )}

      {mode === "simple" ? (
        <div className={styles.simpleRow}>
          <select
            className={styles.select}
            aria-label="Frequency"
            value={simple.frequency}
            onChange={(e) => emitSimple({ ...simple, frequency: e.target.value as SimpleFrequency })}
          >
            <option value="daily">Daily</option>
            <option value="weekdays">Every weekday</option>
            <option value="weekly">Weekly</option>
            <option value="monthly">Monthly</option>
          </select>

          {simple.frequency === "weekly" && (
            <select
              className={styles.select}
              aria-label="Day of week"
              value={simple.dayOfWeek ?? 1}
              onChange={(e) => emitSimple({ ...simple, dayOfWeek: parseInt(e.target.value, 10) })}
            >
              {DAY_LABELS.map((label, idx) => (
                <option key={label} value={idx}>
                  {label}
                </option>
              ))}
            </select>
          )}

          {simple.frequency === "monthly" && (
            <input
              className={styles.numberInput}
              type="number"
              aria-label="Day of month"
              min={1}
              max={31}
              value={simple.dayOfMonth ?? 1}
              onChange={(e) =>
                emitSimple({
                  ...simple,
                  dayOfMonth: Math.min(31, Math.max(1, parseInt(e.target.value, 10) || 1)),
                })
              }
            />
          )}

          <input
            className={styles.timeInput}
            type="time"
            aria-label="Time"
            value={fieldsToTime(simple.hour, simple.minute)}
            onChange={(e) => {
              const t = timeToFields(e.target.value);
              if (t) emitSimple({ ...simple, hour: t.hour, minute: t.minute });
            }}
          />
        </div>
      ) : (
        <input
          className={styles.rawInput}
          type="text"
          aria-label="Advanced cron expression"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="0 9 * * 1-5"
          aria-describedby={showError ? `${explanationId} ${errorId}` : explanationId}
          aria-invalid={showError}
        />
      )}

      <div id={explanationId} className={styles.explanation} aria-live="polite">
        {explanation}
      </div>
      {showError && (
        <div id={errorId} role="alert" data-testid="wf-cron-error" className={styles.error}>
          {validation.error}
        </div>
      )}
      <div className={styles.timezoneNote}>
        Runs in the server&apos;s local timezone (not your browser&apos;s)
      </div>
    </div>
  );
}
