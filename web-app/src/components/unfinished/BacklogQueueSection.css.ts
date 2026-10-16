import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export {
  section,
  sectionHeader,
  chevron,
  chevronExpanded,
  sectionTitle,
  badgeBase as badge,
} from "@/styles/collapsibleSection.css";

export const importButton = style({
  marginLeft: "auto",
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  cursor: "pointer",
  whiteSpace: "nowrap",
  ":hover": {
    borderColor: vars.color.primary,
    color: vars.color.primary,
  },
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  paddingLeft: vars.space["4"],
});

export const card = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  textDecoration: "none",
  transition: "border-color 0.15s",
  ":hover": {
    borderColor: vars.color.borderHover,
  },
});

export const cardTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
  flexGrow: 1,
});

export const statusChip = style({
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  color: vars.color.textMuted,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: `2px ${vars.space["2"]}`,
  whiteSpace: "nowrap",
});

export const priorityChip = style({
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const empty = style({
  padding: `${vars.space["4"]} ${vars.space["4"]}`,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const errorBox = style({
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  marginLeft: vars.space["4"],
  color: vars.color.errorText,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
});

// --- Import modal ---

export const overlay = style({
  position: "fixed",
  inset: 0,
  backgroundColor: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: zIndex.modal,
});

export const dialog = style({
  backgroundColor: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  boxShadow: vars.shadow.lg,
  padding: vars.space["6"],
  maxWidth: "480px",
  width: "calc(100% - 2rem)",
  maxHeight: "80vh",
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const dialogHeading = style({
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  color: vars.color.textPrimary,
  margin: 0,
});
