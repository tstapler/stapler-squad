export type SimpleFrequency = "daily" | "weekdays" | "weekly" | "monthly";

export interface SimpleSchedule {
  frequency: SimpleFrequency;
  hour: number; // 0-23
  minute: number; // 0-59
  dayOfWeek?: number; // 0-6 (Sun-Sat), required for "weekly"
  dayOfMonth?: number; // 1-31, required for "monthly"
}

export const DEFAULT_SIMPLE_SCHEDULE: SimpleSchedule = {
  frequency: "daily",
  hour: 9,
  minute: 0,
};

/** Builds the cron string the backend expects from a Simple-mode selection. */
export function buildCronFromSimple(s: SimpleSchedule): string {
  const { hour, minute } = s;
  switch (s.frequency) {
    case "daily":
      return `${minute} ${hour} * * *`;
    case "weekdays":
      return `${minute} ${hour} * * 1-5`;
    case "weekly":
      return `${minute} ${hour} * * ${s.dayOfWeek ?? 1}`;
    case "monthly":
      return `${minute} ${hour} ${s.dayOfMonth ?? 1} * *`;
  }
}

/**
 * Recovers a SimpleSchedule from a cron string only if it exactly matches a shape the
 * builder itself produces. Returns null (not an error) for anything else — step values,
 * ranges other than the exact weekday "1-5", lists, multi-field restrictions, etc. — so
 * the caller can fall back to a raw-editor notice instead of guessing.
 */
export function parseCronToSimple(expr: string): SimpleSchedule | null {
  const fields = expr.trim().split(/\s+/).filter(Boolean);
  if (fields.length !== 5) return null;
  const [minuteStr, hourStr, dom, month, dow] = fields;

  if (!/^\d{1,2}$/.test(minuteStr) || !/^\d{1,2}$/.test(hourStr)) return null;
  const minute = parseInt(minuteStr, 10);
  const hour = parseInt(hourStr, 10);
  if (minute > 59 || hour > 23) return null;
  if (month !== "*") return null;

  if (dom === "*" && dow === "1-5") {
    return { frequency: "weekdays", hour, minute };
  }
  if (dom === "*" && dow === "*") {
    return { frequency: "daily", hour, minute };
  }
  if (dom === "*" && /^[0-6]$/.test(dow)) {
    return { frequency: "weekly", hour, minute, dayOfWeek: parseInt(dow, 10) };
  }
  if (dow === "*" && /^([1-9]|[12]\d|3[01])$/.test(dom)) {
    return { frequency: "monthly", hour, minute, dayOfMonth: parseInt(dom, 10) };
  }

  return null;
}
