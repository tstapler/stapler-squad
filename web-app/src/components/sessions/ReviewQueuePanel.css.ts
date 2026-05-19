import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

const queueFadeIn = keyframes({
  from: { opacity: 0, transform: "translateY(4px)" },
  to: { opacity: 1, transform: "translateY(0)" },
});

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: "20px",
  height: "100%",
  display: "flex",
  flexDirection: "column",
});

export const header = style({
  marginBottom: "20px",
});

export const titleRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: vars.space["3"],
});

export const title = style({
  margin: 0,
  fontSize: "24px",
  fontWeight: 700,
  color: vars.color.textPrimary,
});

export const count = style({
  fontWeight: 600,
  color: vars.color.textSecondary,
  fontSize: "20px",
});

export const refreshButton = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: `${vars.space["2"]} 12px`,
  fontSize: "20px",
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
      transform: "rotate(180deg)",
    },
    "&:disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
});

export const stats = style({
  display: "flex",
  gap: "16px",
  fontSize: vars.fontSize.base,
});

export const stat = style({
  color: vars.color.textSecondary,
  fontWeight: 500,
});

export const filters = style({
  display: "flex",
  flexDirection: "column",
  gap: "16px",
  marginBottom: "20px",
  paddingBottom: "20px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const filterGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const filterLabel = style({
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const filterButtons = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const filterButton = style({
  padding: `6px 12px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "13px",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
    "&:disabled": { opacity: 0.4, cursor: "not-allowed" },
  },
  "@media": {
    "(max-width: 768px)": {
      padding: "12px 16px",
      minHeight: "44px",
    },
  },
});

export const filterButtonActive = style({
  background: vars.color.primary,
  color: "white",
  borderColor: vars.color.primary,
});

export const items = style({
  flex: 1,
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  selectors: {
    "&::-webkit-scrollbar": { width: "8px" },
    "&::-webkit-scrollbar-track": { background: vars.color.cardBackground },
    "&::-webkit-scrollbar-thumb": {
      background: vars.color.borderColor,
      borderRadius: vars.radii.sm,
    },
    "&::-webkit-scrollbar-thumb:hover": { background: vars.color.borderHover },
  },
});

export const item = style({
  background: vars.color.background,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  display: "flex",
  gap: vars.space["3"],
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      borderColor: vars.color.borderHover,
      boxShadow: "0 2px 8px rgba(0, 0, 0, 0.1)",
    },
  },
});

export const itemClickable = style({
  flex: 1,
  padding: "16px",
  cursor: "pointer",
});

export const currentItem = style({
  background: vars.color.accentBg,
  borderLeft: `3px solid ${vars.color.primary}`,
});

export const itemActions = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: "6px",
  padding: `12px 12px 12px 0`,
  borderLeft: `1px solid ${vars.color.borderColor}`,
});

export const itemHeader = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: vars.space["3"],
});

export const itemTitle = style({
  margin: 0,
  fontSize: "16px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const itemBody = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  marginBottom: vars.space["3"],
});

export const itemContext = style({
  margin: 0,
  fontSize: vars.fontSize.base,
  color: vars.color.textSecondary,
  fontStyle: "italic",
});

export const commandPreview = style({
  margin: 0,
  padding: `${vars.space["2"]} 10px`,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontFamily: "monospace",
  fontSize: vars.fontSize.sm,
  lineHeight: 1.5,
  color: vars.color.textPrimary,
  maxHeight: "6em",
  overflowY: "auto",
  wordBreak: "break-all",
  whiteSpace: "pre-wrap",
});

export const expiredBadge = style({
  display: "inline-block",
  padding: `2px ${vars.space["2"]}`,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.sm,
  fontSize: "11px",
  fontWeight: 600,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const itemPattern = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textTertiary,
  fontFamily: "monospace",
});

export const sessionDetails = style({
  display: "flex",
  flexDirection: "column",
  gap: "6px",
  marginTop: vars.space["3"],
  paddingTop: vars.space["3"],
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const detailRow = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "baseline",
  fontSize: "13px",
});

export const detailLabel = style({
  color: vars.color.textSecondary,
  fontWeight: 500,
  minWidth: "80px",
});

export const detailValue = style({
  color: vars.color.textPrimary,
  fontFamily: "monospace",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  flex: 1,
});

export const tags = style({
  display: "flex",
  gap: "6px",
  flexWrap: "wrap",
});

export const tag = style({
  padding: `2px ${vars.space["2"]}`,
  background: vars.color.accentBg,
  border: `1px solid ${vars.color.primary}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  color: vars.color.primary,
  fontWeight: 500,
});

export const itemFooter = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  paddingTop: vars.space["2"],
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const itemAge = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const diffStats = style({
  display: "flex",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  fontFamily: "monospace",
});

export const diffAdded = style({
  color: vars.color.success,
});

export const diffRemoved = style({
  color: vars.color.error,
});

export const loading = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "40px 20px",
  textAlign: "center",
  color: vars.color.textSecondary,
});

export const empty = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "40px 20px",
  textAlign: "center",
  color: vars.color.textSecondary,
});

globalStyle(`${empty} p`, { margin: 0, fontSize: "16px" });

export const error = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "40px 20px",
  textAlign: "center",
  color: vars.color.errorText,
});

export const emptySubtext = style({
  marginTop: `${vars.space["2"]} !important` as string,
  fontSize: `${vars.fontSize.base} !important` as string,
  // textTertiary (#767676) is calibrated for pure white; on cardBackground (#f9f9f9)
  // contrast drops to 4.31:1 (below WCAG AA 4.5:1). textMuted (#6b6b6b) = 4.92:1 ✅
  color: vars.color.textMuted,
});

export const completionState = style({
  animationName: queueFadeIn,
  animationDuration: "0.4s",
  animationTimingFunction: "ease-in",
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      animationName: "none",
    },
  },
});

export const completionIcon = style({
  fontSize: "20px",
  fontWeight: "bold",
  marginBottom: `${vars.space["1"]} !important` as string,
  color: vars.color.success,
});

export const retryButton = style({
  marginTop: "16px",
  padding: `${vars.space["2"]} 16px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.base,
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
});

export const visuallyHidden = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  padding: 0,
  margin: "-1px",
  overflow: "hidden",
  clipPath: "inset(50%)",
  whiteSpace: "nowrap",
  border: 0,
});

export const oldestCallout = style({
  marginTop: vars.space["2"],
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  fontWeight: 500,
});

export const newItemsBanner = style({
  display: "block",
  width: "100%",
  marginTop: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.accentBg,
  border: `1px solid ${vars.color.primary}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  color: vars.color.primary,
  fontWeight: 600,
  textAlign: "center",
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.primary,
      color: "white",
    },
  },
});

export const filterToggleRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  marginBottom: vars.space["3"],
});

export const filterToggle = style({
  padding: `6px 12px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.base,
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
});

export const filterToggleActive = style({
  background: vars.color.accentBg,
  borderColor: vars.color.primary,
  color: vars.color.primary,
});

export const filterClear = style({
  padding: `6px 10px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: "transparent",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const modalOverlay = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: zIndex.modal,
});

export const modalContent = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: "1.5rem",
  maxWidth: "520px",
  width: "90%",
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
});

export const ruleModalContent = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: "1.5rem",
  maxWidth: "600px",
  width: "90%",
  maxHeight: "90vh",
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
});

export const divergedBadge = style({
  fontSize: vars.fontSize.xs,
  padding: `2px 6px`,
  background: vars.color.warningBg,
  color: vars.color.warning,
  borderRadius: vars.radii.sm,
  whiteSpace: "nowrap",
});
