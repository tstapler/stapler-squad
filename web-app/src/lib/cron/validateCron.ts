/**
 * Validates a cron expression against the exact grammar the backend accepts:
 * robfig/cron/v3 configured with `cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow`
 * (server/workflows/scheduler.go). That means: exactly 5 whitespace-separated fields, no
 * seconds field, no `@` descriptors (the Descriptor parser option is not enabled server-side),
 * and no Quartz `?`/`L`/`W`/`#` extensions.
 */

export interface CronValidationResult {
  valid: boolean;
  error?: string;
}

interface FieldBounds {
  min: number;
  max: number;
  names?: Record<string, number>;
}

const MINUTE: FieldBounds = { min: 0, max: 59 };
const HOUR: FieldBounds = { min: 0, max: 23 };
const DOM: FieldBounds = { min: 1, max: 31 };
const MONTH: FieldBounds = {
  min: 1,
  max: 12,
  names: { jan: 1, feb: 2, mar: 3, apr: 4, may: 5, jun: 6, jul: 7, aug: 8, sep: 9, oct: 10, nov: 11, dec: 12 },
};
const DOW: FieldBounds = {
  min: 0,
  max: 6,
  names: { sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6 },
};

const FIELD_NAMES = ["minute", "hour", "day-of-month", "month", "day-of-week"];

function parseIntOrName(token: string, names?: Record<string, number>): number | null {
  if (names) {
    const named = names[token.toLowerCase()];
    if (named !== undefined) return named;
  }
  if (!/^\d+$/.test(token)) return null;
  return parseInt(token, 10);
}

function validateRangeExpr(expr: string, bounds: FieldBounds): string | null {
  const slashParts = expr.split("/");
  if (slashParts.length > 2) return `too many slashes: "${expr}"`;
  const [rangePart, stepPart] = slashParts;

  const hyphenParts = rangePart.split("-");
  if (hyphenParts.length > 2) return `too many hyphens: "${expr}"`;

  let start: number;
  let end: number;
  if (hyphenParts[0] === "*") {
    start = bounds.min;
    end = bounds.max;
  } else {
    const s = parseIntOrName(hyphenParts[0], bounds.names);
    if (s === null) return `invalid value "${hyphenParts[0]}" in "${expr}"`;
    start = s;
    if (hyphenParts.length === 2) {
      const e = parseIntOrName(hyphenParts[1], bounds.names);
      if (e === null) return `invalid value "${hyphenParts[1]}" in "${expr}"`;
      end = e;
    } else {
      end = start;
    }
  }

  if (stepPart !== undefined) {
    if (!/^\d+$/.test(stepPart)) return `invalid step "${stepPart}" in "${expr}"`;
    const step = parseInt(stepPart, 10);
    if (step === 0) return `step must be a positive number: "${expr}"`;
    if (hyphenParts.length === 1 && hyphenParts[0] !== "*") {
      // robfig/cron: "N/step" means "N-max/step"
      end = bounds.max;
    }
  }

  if (start < bounds.min) return `"${expr}" starts below the minimum of ${bounds.min}`;
  if (end > bounds.max) return `"${expr}" ends above the maximum of ${bounds.max}`;
  if (start > end) return `"${expr}" has a start greater than its end`;

  return null;
}

function validateField(field: string, bounds: FieldBounds): string | null {
  const items = field.split(",");
  for (const item of items) {
    if (item === "") return "empty range expression";
    const err = validateRangeExpr(item, bounds);
    if (err) return err;
  }
  return null;
}

export function validateCron(expr: string): CronValidationResult {
  const trimmed = expr.trim();
  if (trimmed === "") return { valid: false, error: "Expression is empty" };

  // The backend parser strips an optional TZ=/CRON_TZ= prefix before parsing fields,
  // regardless of which fields are configured — mirror that so we don't over-reject.
  const tzMatch = trimmed.match(/^(?:TZ|CRON_TZ)=\S+\s+(.+)$/);
  const spec = tzMatch ? tzMatch[1] : trimmed;

  if (spec.startsWith("@")) {
    return {
      valid: false,
      error: "Descriptors like @daily aren't supported by this server's cron parser — use 5 numeric fields instead",
    };
  }

  const fields = spec.split(/\s+/).filter(Boolean);
  if (fields.length !== 5) {
    return {
      valid: false,
      error: `Expected exactly 5 fields (minute hour day-of-month month day-of-week), found ${fields.length}`,
    };
  }

  for (const field of fields) {
    if (/[?LW#]/i.test(field)) {
      return {
        valid: false,
        error: `Unsupported syntax "${field}" — Quartz extensions (?, L, W, #) aren't supported`,
      };
    }
  }

  const bounds = [MINUTE, HOUR, DOM, MONTH, DOW];
  for (let i = 0; i < 5; i++) {
    const err = validateField(fields[i], bounds[i]);
    if (err) return { valid: false, error: `${FIELD_NAMES[i]} field: ${err}` };
  }

  return { valid: true };
}
