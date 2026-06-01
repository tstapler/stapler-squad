// +feature: insights-dashboard
"use client";

import { filterBar, presetGroup, presetButton, customRange, dateInput, rangeError } from "./TimeRangeFilter.css";

export type TimeRangePreset = "today" | "7d" | "30d" | "90d" | "all" | "custom";

export interface TimeRangeValue {
  preset: TimeRangePreset;
  from?: Date;
  to?: Date;
}

interface Props {
  value: TimeRangeValue;
  onChange: (v: TimeRangeValue) => void;
}

const PRESETS: { value: TimeRangePreset; label: string }[] = [
  { value: "today", label: "Today" },
  { value: "7d", label: "Last 7 days" },
  { value: "30d", label: "Last 30 days" },
  { value: "90d", label: "Last 90 days" },
  { value: "all", label: "All time" },
  { value: "custom", label: "Custom" },
];

function toDateInputValue(d: Date | undefined): string {
  if (!d) return "";
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function TimeRangeFilter({ value, onChange }: Props) {
  function handlePreset(preset: TimeRangePreset) {
    if (preset === "custom") {
      onChange({ preset, from: value.from, to: value.to });
      return;
    }
    onChange({ preset });
  }

  function handleFromChange(e: React.ChangeEvent<HTMLInputElement>) {
    const from = e.target.value ? new Date(e.target.value + "T00:00:00") : undefined;
    onChange({ preset: "custom", from, to: value.to });
  }

  function handleToChange(e: React.ChangeEvent<HTMLInputElement>) {
    const to = e.target.value ? new Date(e.target.value + "T23:59:59") : undefined;
    onChange({ preset: "custom", from: value.from, to });
  }

  return (
    <div className={filterBar}>
      <div className={presetGroup}>
        {PRESETS.map((p) => (
          <button
            key={p.value}
            type="button"
            className={presetButton({ active: value.preset === p.value })}
            onClick={() => handlePreset(p.value)}
          >
            {p.label}
          </button>
        ))}
      </div>

      {value.preset === "custom" && (
        <div className={customRange}>
          <input
            type="date"
            className={dateInput}
            value={toDateInputValue(value.from)}
            onChange={handleFromChange}
            aria-label="From date"
          />
          <span>–</span>
          <input
            type="date"
            className={dateInput}
            value={toDateInputValue(value.to)}
            onChange={handleToChange}
            aria-label="To date"
          />
          {value.from && value.to && value.from > value.to && (
            <p className={rangeError}>&apos;From&apos; date must be before &apos;To&apos; date</p>
          )}
        </div>
      )}
    </div>
  );
}

/** Derives stable Date objects from a TimeRangeValue so hook deps don't trigger on every render. */
export function resolveTimeRangeDates(preset: TimeRangePreset, fromParam?: string, toParam?: string): { from?: Date; to?: Date } {
  const now = new Date();
  switch (preset) {
    case "today": {
      const start = new Date(now);
      start.setHours(0, 0, 0, 0);
      return { from: start, to: undefined };
    }
    case "7d": {
      const d = new Date(now);
      d.setDate(d.getDate() - 7);
      return { from: d, to: undefined };
    }
    case "30d": {
      const d = new Date(now);
      d.setDate(d.getDate() - 30);
      return { from: d, to: undefined };
    }
    case "90d": {
      const d = new Date(now);
      d.setDate(d.getDate() - 90);
      return { from: d, to: undefined };
    }
    case "custom":
      return {
        from: fromParam ? new Date(fromParam + "T00:00:00") : undefined,
        to: toParam ? new Date(toParam + "T23:59:59") : undefined,
      };
    case "all":
    default:
      return { from: undefined, to: undefined };
  }
}
