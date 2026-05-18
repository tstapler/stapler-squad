import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const pageWrapper = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  overflow: "hidden",
  background: vars.color.background,
});

export const pageHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: `${vars.space["4"]} ${vars.space["6"]}`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  gap: vars.space["3"],
  flexShrink: 0,
  flexWrap: "wrap",
});

export const pageTitle = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const newItemButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: "background 0.1s ease",
  ":hover": {
    background: vars.color.primaryHover,
  },
});

export const tabBar = style({
  display: "flex",
  gap: 0,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  padding: `0 ${vars.space["6"]}`,
  flexShrink: 0,
});

export const tab = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textMuted,
  textDecoration: "none",
  transition: "color 0.1s ease, border-color 0.1s ease",
  cursor: "pointer",
  background: "none",
  border: "none",
  borderBottom: "2px solid transparent",
  ":hover": {
    color: vars.color.textPrimary,
  },
});

export const tabActive = style({
  color: vars.color.primary,
  borderBottom: `2px solid ${vars.color.primary}`,
});

export const filterBar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["3"]} ${vars.space["6"]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  flexShrink: 0,
  flexWrap: "wrap",
});

export const searchInput = style({
  flex: 1,
  minWidth: "180px",
  maxWidth: "320px",
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  outline: "none",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
});

export const filterChipGroup = style({
  display: "flex",
  gap: vars.space["1"],
  flexWrap: "wrap",
});

export const filterChip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderMuted}`,
  background: vars.color.surfaceMuted,
  color: vars.color.textSecondary,
  transition: "background 0.1s ease, border-color 0.1s ease",
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderStrong,
  },
});

export const filterChipActive = style({
  background: vars.color.accentBg,
  color: vars.color.primary,
  borderColor: vars.color.primary,
});

export const contentArea = style({
  display: "flex",
  flex: 1,
  overflow: "hidden",
});

export const listPane = style({
  flex: 1,
  overflow: "auto",
  padding: vars.space["4"],
});

export const detailPane = style({
  width: "420px",
  borderLeft: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  "@media": {
    "(max-width: 768px)": {
      position: "fixed",
      inset: 0,
      width: "100%",
      zIndex: "500",
      background: vars.color.modalBackground,
    },
  },
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
});

export const tableHead = style({
  position: "sticky",
  top: 0,
  background: vars.color.background,
  zIndex: 1,
});

export const tableHeaderCell = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  textAlign: "left",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  whiteSpace: "nowrap",
});

export const tableRow = style({
  cursor: "pointer",
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  transition: "background 0.1s ease",
  ":hover": {
    background: vars.color.hoverBackground,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "-2px",
  },
});

export const tableRowActive = style({
  background: vars.color.accentBg,
});

export const tableCell = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  color: vars.color.textPrimary,
  verticalAlign: "middle",
});

export const titleCell = style({
  maxWidth: "280px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  fontWeight: vars.fontWeight.medium,
});

export const acProgressCell = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const emptyState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space["12"],
  gap: vars.space["3"],
  color: vars.color.textMuted,
  textAlign: "center",
});

export const emptyTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const emptySubtitle = style({
  fontSize: vars.fontSize.sm,
});

export const emptyActionButton = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  marginTop: vars.space["2"],
  ":hover": {
    background: vars.color.primaryHover,
  },
});

// Modal overlay for item form
export const modalOverlay = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  zIndex: "1000",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space["4"],
});

export const modalBox = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["6"],
  width: "100%",
  maxWidth: "580px",
  maxHeight: "90vh",
  overflowY: "auto",
  boxShadow: vars.shadow.lg,
});

export const modalTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
  marginBottom: vars.space["4"],
});

export const statusBadge = style({
  display: "inline-flex",
  alignItems: "center",
  borderRadius: vars.radii.sm,
  padding: `2px ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
});
export const statusIdea = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderMuted}`,
});
export const statusReady = style({
  background: vars.statusBadge.inputBg,
  color: vars.statusBadge.inputFg,
  border: `1px solid ${vars.statusBadge.inputBorder}`,
});
export const statusInProgress = style({
  background: vars.statusBadge.uncommittedBg,
  color: vars.statusBadge.uncommittedFg,
  border: `1px solid ${vars.statusBadge.uncommittedBorder}`,
});
export const statusReview = style({
  background: vars.statusBadge.approvalBg,
  color: vars.statusBadge.approvalFg,
  border: `1px solid ${vars.statusBadge.approvalBorder}`,
});
export const statusDone = style({
  background: vars.statusBadge.completeBg,
  color: vars.statusBadge.completeFg,
  border: `1px solid ${vars.statusBadge.completeBorder}`,
});
export const statusArchived = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textDisabled,
  border: `1px solid ${vars.color.borderMuted}`,
});

export const priorityBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: vars.radii.sm,
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  fontFamily: vars.font.mono,
  minWidth: "24px",
  background: vars.color.accentBg,
  color: vars.color.primary,
  border: `1px solid ${vars.color.borderMuted}`,
});
