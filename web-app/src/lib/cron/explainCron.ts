import cronstrue from "cronstrue";
import { validateCron } from "./validateCron";

/**
 * Human-readable explanation of a 5-field cron expression, or a status/error
 * message suitable for direct display in the UI.
 */
export function explainCron(expr: string): string {
  const trimmed = expr.trim();
  if (trimmed === "") return "Enter a schedule above";

  const fields = trimmed.split(/\s+/).filter(Boolean);
  if (!trimmed.startsWith("@") && fields.length < 5) {
    // User is still mid-keystroke; don't flash a hard error on every partial input.
    return "Still typing…";
  }

  const validation = validateCron(trimmed);
  if (!validation.valid) {
    return `Invalid: ${validation.error}`;
  }

  const [minute, hour, dom, month, dow] = fields;

  try {
    if (dom !== "*" && dow !== "*") {
      // ponytail: cronstrue always joins a restricted day-of-month + day-of-week with
      // "and" (confirmed via its logicalAndDayFields option — neither setting produces
      // "or"). Standard cron (and robfig/cron/v3) semantics are OR when both are
      // restricted, so build the explanation from two single-field expressions and
      // join them ourselves. Ceiling: phrasing can get wordy when combined with
      // step/range hour or month fields — good enough for the common cases this
      // widget is meant to cover.
      const domText = cronstrue.toString(`${minute} ${hour} ${dom} ${month} *`);
      const dowText = cronstrue.toString(`${minute} ${hour} * ${month} ${dow}`);
      const [, ...dowRestParts] = dowText.split(", ");
      const dowClause = (dowRestParts.join(", ") || dowText).replace(/^only /, "");
      return `${domText}, or ${dowClause}`;
    }
    return cronstrue.toString(trimmed);
  } catch {
    return "Unable to explain this expression";
  }
}
