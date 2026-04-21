import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  maxWidth: "700px",
  margin: "0 auto",
  padding: `${vars.space["8"]} ${vars.space["4"]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["8"],
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: "600",
  color: vars.color.textPrimary,
  margin: 0,
  paddingBottom: vars.space["2"],
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const card = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["4"],
});

// Credential list
export const credentialList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const credentialRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  gap: vars.space["4"],
});

export const credentialInfo = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  minWidth: 0,
});

export const credentialName = style({
  fontSize: vars.fontSize.base,
  fontWeight: "500",
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const credentialMeta = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

// Buttons
export const primaryButton = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: vars.color.primary,
  color: vars.color.textInverse,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: "600",
  cursor: "pointer",
  transition: "opacity 0.15s",
  selectors: {
    "&:hover:not(:disabled)": { opacity: 0.85 },
    "&:disabled": { opacity: 0.45, cursor: "not-allowed" },
  },
});

export const dangerButton = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: "transparent",
  color: vars.color.error,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.xs,
  fontWeight: "500",
  cursor: "pointer",
  transition: "all 0.15s",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.errorBg,
    },
    "&:disabled": { opacity: 0.45, cursor: "not-allowed" },
  },
});

export const ghostButton = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: "500",
  cursor: "pointer",
  transition: "all 0.15s",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

// Modal overlay
export const modalOverlay = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  zIndex: 1200,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space["4"],
});

export const modal = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["6"],
  width: "100%",
  maxWidth: "480px",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const modalTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: "600",
  color: vars.color.textPrimary,
  margin: 0,
});

export const modalActions = style({
  display: "flex",
  gap: vars.space["3"],
  justifyContent: "flex-end",
});

// Invite modal specifics
export const stepList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const step = style({
  display: "flex",
  gap: vars.space["3"],
  alignItems: "flex-start",
});

export const stepNumber = style({
  flexShrink: 0,
  width: "1.5rem",
  height: "1.5rem",
  borderRadius: vars.radii.full,
  background: vars.color.primary,
  color: vars.color.textInverse,
  fontSize: vars.fontSize.xs,
  fontWeight: "700",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
});

export const stepContent = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const stepTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: "600",
  color: vars.color.textPrimary,
  margin: 0,
});

export const stepDesc = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  lineHeight: "1.5",
  margin: 0,
});

export const qrRow = style({
  display: "flex",
  gap: vars.space["4"],
  alignItems: "flex-start",
  flexWrap: "wrap",
});

export const qrBlock = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: vars.space["1"],
});

export const qrImage = style({
  width: "120px",
  height: "120px",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: "#fff",
});

export const qrLabel = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  textAlign: "center",
});

export const urlRow = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "center",
});

export const urlText = style({
  flex: 1,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  fontFamily: vars.font.mono,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
});

export const countdown = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  textAlign: "center",
});

export const countdownExpired = style({
  color: vars.color.error,
  fontWeight: "600",
});

export const warningText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.warning,
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  lineHeight: "1.5",
});

export const emptyState = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  textAlign: "center",
  padding: `${vars.space["6"]} ${vars.space["4"]}`,
});

export const errorText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.error,
  background: vars.color.errorBg,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
});
