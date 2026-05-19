import { style } from "@vanilla-extract/css";
import { recipe, type RecipeVariants } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

// ── Card container ────────────────────────────────────────────────────────────

export const card = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 14,
});

// ── Confidence badge ──────────────────────────────────────────────────────────

export const confidenceBadge = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    padding: `${vars.space["1"]} ${vars.space["3"]}`,
    borderRadius: vars.radii.full,
    fontSize: vars.fontSize.xs,
    fontWeight: vars.fontWeight.semibold,
    whiteSpace: "nowrap",
  },
  variants: {
    level: {
      high: {
        background: vars.color.successBg,
        color: vars.color.success,
      },
      medium: {
        background: vars.color.warningBg,
        color: vars.color.warning,
      },
      low: {
        background: vars.color.errorBg,
        color: vars.color.errorText,
      },
    },
  },
  defaultVariants: {
    level: "medium",
  },
});

export type ConfidenceBadgeVariants = RecipeVariants<typeof confidenceBadge>;

// ── Explanation block ─────────────────────────────────────────────────────────

export const explanationBlock = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: 1.5,
});

// ── Source commands block ─────────────────────────────────────────────────────

export const sourceCommandsDetails = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const sourceCommandsSummary = style({
  cursor: "pointer",
  padding: `${vars.space["1"]} 0`,
  fontWeight: vars.fontWeight.medium,
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
  },
});

export const sourceCommandsPre = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["2"],
  margin: `${vars.space["2"]} 0 0`,
  overflowX: "auto",
  color: vars.color.textPrimary,
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
});

// ── Warning banners ───────────────────────────────────────────────────────────

export const conflictBanner = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  color: vars.color.warningText,
  fontSize: vars.fontSize.sm,
});

export const shadowBanner = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
});

// ── Form fields ───────────────────────────────────────────────────────────────

export const fieldGrid = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
  gap: 12,
  "@media": {
    "screen and (max-width: 640px)": {
      gridTemplateColumns: "1fr",
    },
  },
});

export const fieldRow = style({
  display: "flex",
  flexDirection: "column",
  gap: 4,
});

export const fieldLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const fieldInput = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  width: "100%",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const fieldSelect = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  width: "100%",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const fieldTextarea = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  width: "100%",
  boxSizing: "border-box",
  minHeight: 60,
  resize: "vertical",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

// ── Action row ────────────────────────────────────────────────────────────────

export const actions = style({
  display: "flex",
  gap: 10,
  justifyContent: "flex-end",
  borderTop: `1px solid ${vars.color.borderColor}`,
  paddingTop: vars.space["3"],
  marginTop: vars.space["2"],
});

export const acceptButton = style({
  background: vars.color.primary,
  border: "none",
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.primaryText,
  cursor: "pointer",
  transition: "opacity 0.15s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      opacity: 0.85,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const discardButton = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.accentHover,
      color: vars.color.textPrimary,
    },
  },
});

// ── Header row (badge + label) ────────────────────────────────────────────────

export const cardHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const cardTitle = style({
  margin: 0,
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});
