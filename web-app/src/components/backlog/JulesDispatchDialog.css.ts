import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme-contract.css";

export const overlay = style({
  position: "fixed",
  inset: 0,
  backgroundColor: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: zIndex.modal,
  padding: vars.space[4],
});

export const dialog = style({
  backgroundColor: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  boxShadow: vars.shadow.lg,
  padding: vars.space[6],
  maxWidth: "520px",
  width: "calc(100% - 2rem)",
  maxHeight: "90vh",
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: vars.space[4],
});

export const heading = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const egressBlock = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
  color: vars.color.warningText,
  backgroundColor: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  padding: vars.space[3],
  fontSize: vars.fontSize.sm,
});

export const egressCheckboxRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
});

export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
});

export const label = style({
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
});

export const input = style({
  padding: `${vars.space[2]} ${vars.space[3]}`,
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const textarea = style([
  input,
  {
    resize: "vertical",
    minHeight: "96px",
    fontFamily: "inherit",
  },
]);

export const hint = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  margin: 0,
});

export const errorBanner = style({
  color: vars.color.errorText,
  backgroundColor: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space[2]} ${vars.space[3]}`,
  fontSize: vars.fontSize.sm,
  margin: 0,
});

export {
  confirmActions as actions,
  confirmPrimaryButton as primaryButton,
  confirmSecondaryButton as secondaryButton,
} from "@/styles/modalChrome.css";
