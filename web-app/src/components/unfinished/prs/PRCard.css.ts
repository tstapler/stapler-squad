import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";
import { CHIP_TOKEN_PAIRS, type ChipTokenPair } from "@/lib/unfinished/prChipTokens";

const NARROW = "screen and (max-width: 480px)";
const INTERACTIVE_MIN = MIN_TOUCH_TARGET;

export const prCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  transition: "border-color 0.15s",
  ":hover": {
    borderColor: vars.color.borderHover,
  },
});

export const prHeader = style({
  display: "flex",
  alignItems: "flex-start",
  flexWrap: "wrap",
  gap: vars.space["3"],
  "@media": { [NARROW]: { flexDirection: "column" } },
});

export const prTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
  textDecoration: "none",
  flexGrow: 1,
  // minWidth:0 overrides the flex item's default content-based automatic
  // minimum size; overflowWrap lets long unbreakable tokens (e.g.
  // "cache_read_input_tokens/cache_creation_input_tokens") wrap instead of
  // forcing this row (and every ancestor up to UnfinishedTab's .container)
  // wider than the viewport on mobile.
  minWidth: 0,
  overflowWrap: "break-word",
  lineHeight: 1.4,
  ":hover": {
    textDecoration: "underline",
    color: vars.color.primary,
  },
});

export const prMeta = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const prRepo = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const prBranch = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const chips = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "center",
  flexWrap: "wrap",
  marginLeft: "auto",
  minWidth: 0,
  "@media": { [NARROW]: { marginLeft: 0 } },
});

const chipBase = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  lineHeight: 1.5,
  whiteSpace: "nowrap",
});

const chipVariant = (pair: ChipTokenPair) =>
  style([
    chipBase,
    {
      background: vars.color[pair.bg],
      color: vars.color[pair.fg],
      border: `1px solid ${vars.color[pair.fg]}`,
    },
  ]);

export const chipDraft = chipVariant(CHIP_TOKEN_PAIRS.draft);
export const chipNeutral = chipVariant(CHIP_TOKEN_PAIRS.neutral);
export const chipSuccess = chipVariant(CHIP_TOKEN_PAIRS.success);
export const chipWarning = chipVariant(CHIP_TOKEN_PAIRS.warning);
export const chipError = chipVariant(CHIP_TOKEN_PAIRS.error);

/** Chip that is a button or link: needs a 44px target (WCAG 2.5.5) and may wrap. */
export const chipInteractive = style({
  minHeight: INTERACTIVE_MIN,
  cursor: "pointer",
  textDecoration: "none",
  whiteSpace: "normal",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" },
});

export const checkList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
});

export const checkLink = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: INTERACTIVE_MIN,
  minWidth: 0,
  overflowWrap: "anywhere",
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
});

export const sessionList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
});

export const sessionRow = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: vars.space["2"],
  minHeight: INTERACTIVE_MIN,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const sessionName = style({
  fontFamily: vars.font.mono,
  minWidth: 0,
  overflowWrap: "anywhere",
});

export const sessionStatus = style({ color: vars.color.textSecondary });

export const noSessionText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const changesRequestedNote = style({
  margin: 0,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  overflowWrap: "break-word",
});

export const hostAccountLabel = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  minWidth: 0,
  overflowWrap: "anywhere",
});

export const worktreeLink = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  textDecoration: "none",
  ":hover": {
    textDecoration: "underline",
  },
});

export const prActions = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: vars.space["2"],
  marginTop: vars.space["1"],
  "@media": { [NARROW]: { flexDirection: "column", alignItems: "stretch" } },
});

export const openSessionButton = style({
  minHeight: INTERACTIVE_MIN,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: vars.color.accentBg,
  color: vars.color.inputFocusBorder,
  border: `1px solid ${vars.color.inputFocusBorder}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  cursor: "pointer",
  textDecoration: "none",
  display: "inline-flex",
  alignItems: "center",
  whiteSpace: "nowrap",
  ":hover": {
    background: vars.color.accentHover,
  },
});

export const createSessionButton = style({
  minHeight: INTERACTIVE_MIN,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  cursor: "pointer",
  textDecoration: "none",
  display: "inline-flex",
  alignItems: "center",
  whiteSpace: "nowrap",
  ":hover": {
    borderColor: vars.color.primary,
    color: vars.color.primary,
  },
});

export const repoGroupSection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  paddingLeft: vars.space["4"],
});

export const repoGroupHeader = style({
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  color: vars.color.textMuted,
  fontWeight: 600,
  padding: `${vars.space["1"]} 0`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  marginTop: vars.space["1"],
});
