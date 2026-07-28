import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const sectionHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  userSelect: "none",
  outline: "none",
  ":hover": {
    background: vars.color.hoverBackground,
  },
  ":focus-visible": {
    boxShadow: `0 0 0 2px ${vars.color.inputFocusBorder}`,
  },
});

export const chevron = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  transition: "transform 0.15s",
  display: "inline-block",
});

export const chevronExpanded = style({
  transform: "rotate(90deg)",
});

export const sectionTitle = style({
  fontWeight: 600,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const badge = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceSubtle,
  borderRadius: vars.radii.full,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  marginLeft: vars.space["2"],
});

export const username = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginLeft: "auto",
  fontFamily: vars.font.mono,
});

export const prList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  paddingLeft: vars.space["4"],
});

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
  gap: vars.space["3"],
});

export const prTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
  textDecoration: "none",
  flexGrow: 1,
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

export const chipDraft = style([
  chipBase,
  {
    background: vars.color.surfaceSubtle,
    color: vars.color.textMuted,
    border: `1px solid ${vars.color.borderColor}`,
  },
]);

export const chipSuccess = style([
  chipBase,
  {
    background: vars.color.successBg,
    color: vars.color.success,
    border: `1px solid ${vars.color.success}`,
  },
]);

export const chipWarning = style([
  chipBase,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipError = style([
  chipBase,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const worktreeLink = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  textDecoration: "none",
  ":hover": {
    textDecoration: "underline",
  },
});

export const empty = style({
  padding: `${vars.space["4"]} ${vars.space["4"]}`,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const authError = style({
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  color: vars.color.warningText,
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  marginLeft: vars.space["4"],
});

export const authBanner = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  marginLeft: vars.space["4"],
});

export const authBannerText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.warningText,
});

export const authBannerSuccess = style({
  display: "flex",
  alignItems: "center",
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  background: vars.color.successBg,
  border: `1px solid ${vars.color.success}`,
  borderRadius: vars.radii.md,
  marginLeft: vars.space["4"],
  fontSize: vars.fontSize.sm,
  color: vars.color.success,
  fontWeight: 600,
});

export const connectButton = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.primary,
  color: vars.color.textInverse,
  border: "none",
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  alignSelf: "flex-start",
  ":hover": {
    background: vars.color.primaryHover,
  },
});

export const cancelButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: "transparent",
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  cursor: "pointer",
  ":hover": {
    borderColor: vars.color.borderHover,
    color: vars.color.textSecondary,
  },
});

export const deviceFlowCard = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  padding: `${vars.space["4"]} ${vars.space["4"]}`,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  marginLeft: vars.space["4"],
});

export const deviceFlowInstructions = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  margin: 0,
});

export const deviceFlowRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["4"],
  flexWrap: "wrap",
});

export const verificationLink = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.primary,
  textDecoration: "none",
  ":hover": {
    textDecoration: "underline",
  },
});

export const userCode = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.base,
  fontWeight: 700,
  letterSpacing: "0.2em",
  color: vars.color.textPrimary,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
});

export const pollingIndicator = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

// --- Account management ---

export const accountsRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
  padding: `${vars.space["1"]} ${vars.space["4"]}`,
});

export const accountChip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `2px ${vars.space["2"]}`,
  background: vars.color.accentBg,
  border: `1px solid ${vars.color.inputFocusBorder}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  color: vars.color.inputFocusBorder,
  fontFamily: vars.font.mono,
  whiteSpace: "nowrap",
});

export const accountChipEnv = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `2px ${vars.space["2"]}`,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontFamily: vars.font.mono,
  whiteSpace: "nowrap",
});

export const hostBadge = style({
  fontSize: "0.85em",
  opacity: 0.7,
});

export const hostInput = style({
  padding: `2px ${vars.space["2"]}`,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  background: vars.color.surfaceSubtle,
  color: vars.color.textSecondary,
});

export const disconnectAccountButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  background: "transparent",
  border: "none",
  color: "inherit",
  cursor: "pointer",
  padding: 0,
  lineHeight: 1,
  fontSize: vars.fontSize.xs,
  opacity: 0.6,
  ":hover": {
    opacity: 1,
  },
});

export const addAccountButton = style({
  padding: `2px ${vars.space["2"]}`,
  background: "transparent",
  border: `1px dashed ${vars.color.borderMuted}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  cursor: "pointer",
  whiteSpace: "nowrap",
  ":hover": {
    borderColor: vars.color.primary,
    color: vars.color.primary,
  },
});

// --- Stats bar ---

export const statsBar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["4"],
  padding: `${vars.space["1"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const statItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
});

export const statCount = style({
  fontWeight: 700,
  color: vars.color.textSecondary,
});

export const statCountWarning = style({
  fontWeight: 700,
  color: vars.color.warningText,
});

export const statCountError = style({
  fontWeight: 700,
  color: vars.color.errorText,
});

// --- Repo groups ---

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

// --- Filter / sort toolbar ---

export const filterBar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
  padding: `${vars.space["1"]} ${vars.space["4"]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  marginBottom: vars.space["1"],
});

export const filterChipGroup = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
});

const filterChipBase = style({
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: 500,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderMuted}`,
  background: "transparent",
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
  lineHeight: 1.6,
  ":hover": {
    borderColor: vars.color.borderHover,
    color: vars.color.textSecondary,
  },
});

export const filterChip = style([filterChipBase, {}]);

export const filterChipActive = style([
  filterChipBase,
  {
    background: vars.color.accentBg,
    borderColor: vars.color.inputFocusBorder,
    color: vars.color.inputFocusBorder,
    fontWeight: 600,
  },
]);

export const sortGroup = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
  marginLeft: "auto",
});

export const sortLabel = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const sortSelect = style({
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  border: `1px solid ${vars.color.borderMuted}`,
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  cursor: "pointer",
  outline: "none",
  ":hover": {
    borderColor: vars.color.borderHover,
  },
  ":focus-visible": {
    boxShadow: `0 0 0 2px ${vars.color.inputFocusBorder}`,
  },
});

export const searchInput = style({
  padding: `2px ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  border: `1px solid ${vars.color.borderMuted}`,
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  outline: "none",
  width: "140px",
  "::placeholder": {
    color: vars.color.textMuted,
  },
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    width: "200px",
    transition: "width 0.2s",
  },
});

// --- Session action buttons ---

export const prActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  marginTop: vars.space["1"],
});

export const openSessionButton = style({
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
