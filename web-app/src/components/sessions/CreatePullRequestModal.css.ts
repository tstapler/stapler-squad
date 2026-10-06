// Styles for CreatePullRequestModal — re-exported from SessionCard.css.ts
// so the component file has a single, co-located CSS import, following the
// SessionActionsOverflow.css.ts precedent.
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export {
  confirmDialog,
  dialogContent,
  dialogActions,
  submitButton,
  cancelButton,
  errorMessage,
} from "./SessionCard.css";

// LLM-generated PR bodies run several paragraphs; a single-line-height
// textarea would truncate visually (ux.md Surface 4).
export const bodyTextarea = style({
  width: "100%",
  minHeight: "10em",
  resize: "vertical",
  padding: `10px 12px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  fontFamily: "inherit",
  marginBottom: vars.space["2"],
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: `0 0 0 3px rgba(0, 112, 243, 0.1)`,
    },
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});

export const fieldLabel = style({
  display: "block",
  fontSize: "0.875rem",
  color: vars.color.textSecondary,
  marginBottom: vars.space["1"],
  fontWeight: 500,
});

export const textInput = style({
  width: "100%",
  padding: `10px 12px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  marginBottom: vars.space["2"],
  transition: "border-color 0.2s ease",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: `0 0 0 3px rgba(0, 112, 243, 0.1)`,
    },
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});

export const branchContext = style({
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
  margin: `0 0 ${vars.space["4"]} 0`,
  fontFamily: "monospace",
});

export const loadingState = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["2"],
  padding: `${vars.space["6"]} 0`,
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
});

export const successMessage = style({
  color: vars.color.successText,
  fontWeight: 600,
  fontSize: "0.9375rem",
  margin: `${vars.space["3"]} 0`,
});

export const prLink = style({
  display: "inline-block",
  color: vars.color.primary,
  fontSize: "0.875rem",
  margin: `0 0 ${vars.space["3"]} 0`,
  wordBreak: "break-all",
});

// Warning banner (persist-failure sub-state) — deliberately distinct from
// errorMessage's red/error styling; announced via role="alert" but visually
// amber/warning, not error, per ux.md Surface 7 Variant C.
export const warningMessage = style({
  display: "block",
  color: vars.color.warningText,
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.lg,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  margin: `${vars.space["2"]} 0`,
});
