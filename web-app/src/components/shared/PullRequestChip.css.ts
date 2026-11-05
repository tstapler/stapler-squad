import { globalStyle, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { CHIP_TOKEN_PAIRS, type ChipTokenPair } from "@/lib/unfinished/prChipTokens";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";
import type { PrRowState } from "@/lib/pr/pullRequests";

// Same breakpoint as PRCard.css.ts (NARROW) so the card and the chip collapse together.
const NARROW = "screen and (max-width: 480px)";

const tokens = (pair: ChipTokenPair) => ({
  background: vars.color[pair.bg],
  color: vars.color[pair.fg],
  border: `1px solid ${vars.color[pair.fg]}`,
});

/** Colour for each state; the glyph shape and the visible label carry the state as well. */
export const stateClass: Record<PrRowState, string> = {
  open: style(tokens(CHIP_TOKEN_PAIRS.prOpen)),
  draft: style(tokens(CHIP_TOKEN_PAIRS.draft)),
  merged: style(tokens(CHIP_TOKEN_PAIRS.prMerged)),
  closed: style(tokens(CHIP_TOKEN_PAIRS.prClosed)),
  unknown: style(tokens(CHIP_TOKEN_PAIRS.prChecking)),
  unverified: style(tokens(CHIP_TOKEN_PAIRS.prClosed)),
};

export const staleClass = style(tokens(CHIP_TOKEN_PAIRS.prStale));

export const chipRow = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  flexWrap: "wrap",
  minWidth: 0,
});

export const chipLink = style({ display: "inline-flex" });

/** The GitHub link is a 44px target on a phone (WCAG 2.5.5). */
globalStyle(`${chipLink} > a`, {
  "@media": { [NARROW]: { minHeight: MIN_TOUCH_TARGET, minWidth: MIN_TOUCH_TARGET, justifyContent: "center" } },
});

export const moreButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["2"]}`,
  borderRadius: 12,
  fontSize: 12,
  fontWeight: 700,
  cursor: "pointer",
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  ":hover": { background: vars.color.hoverBackground },
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" },
});

export const staleMark = style({ fontWeight: 700, marginLeft: 2 });

/** The state word beside the glyph. Full opacity: GitHubBadge's `priorityLabel` dims to 0.9, which puts the open chip at 4.44:1 in dark. */
export const stateLabel = style({
  fontSize: 10,
  fontWeight: 700,
  letterSpacing: "0.03em",
  textTransform: "uppercase",
  paddingLeft: 2,
  borderLeft: "1px solid currentColor",
  marginLeft: 2,
});
