import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const yamlTextarea = style({
  width: "100%",
  minHeight: 200,
  fontFamily: "monospace",
  fontSize: 12,
  padding: "10px 12px",
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: 6,
  resize: "vertical",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const yamlTextareaLoading = style([yamlTextarea, {
  borderColor: vars.color.primary,
  opacity: 0.85,
}]);

export const previewList = style({
  display: "flex",
  flexDirection: "column",
  gap: 8,
  maxHeight: 360,
  overflowY: "auto",
});

export const ruleCard = recipe({
  base: {
    padding: "10px 12px",
    borderRadius: 6,
    borderLeft: "3px solid transparent",
    background: vars.color.cardBackground,
    border: `1px solid ${vars.color.borderColor}`,
  },
  variants: {
    status: {
      valid: {
        borderLeftColor: vars.color.success,
      },
      error: {
        borderLeftColor: vars.color.error,
        background: vars.color.errorBg,
      },
      overwrite: {
        borderLeftColor: vars.color.warning,
        background: vars.color.warningBg,
      },
      skip: {
        borderLeftColor: vars.color.borderMuted,
        opacity: 0.7,
      },
    },
  },
});

export const ruleCardHeader = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  flexWrap: "wrap",
});

export const ruleCardName = style({
  fontWeight: 600,
  color: vars.color.textPrimary,
  fontSize: 13,
});

export const decisionBadge = recipe({
  base: {
    padding: "2px 6px",
    borderRadius: 4,
    fontSize: 11,
    fontWeight: 600,
    textTransform: "uppercase",
  },
  variants: {
    decision: {
      allow: {
        background: vars.color.successBg,
        color: vars.color.success,
      },
      deny: {
        background: vars.color.errorBg,
        color: vars.color.error,
      },
      escalate: {
        background: vars.color.warningBg,
        color: vars.color.warningText,
      },
      unknown: {
        background: vars.color.surfaceMuted,
        color: vars.color.textMuted,
      },
    },
  },
  defaultVariants: {
    decision: "unknown",
  },
});

export const statusBadge = recipe({
  base: {
    padding: "2px 6px",
    borderRadius: 4,
    fontSize: 11,
    fontWeight: 600,
  },
  variants: {
    type: {
      overwrite: {
        background: vars.color.warningBg,
        color: vars.color.warningText,
      },
      skip: {
        background: vars.color.surfaceMuted,
        color: vars.color.textMuted,
      },
    },
  },
});

export const matchChips = style({
  display: "flex",
  flexWrap: "wrap",
  gap: 4,
  marginTop: 4,
});

export const matchChip = style({
  padding: "2px 6px",
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: 4,
  fontSize: 11,
  color: vars.color.textSecondary,
  fontFamily: "monospace",
});

export const errorList = style({
  marginTop: 6,
  paddingLeft: 16,
  fontSize: 12,
  color: vars.color.error,
});

export const errorListItem = style({
  marginBottom: 2,
});

export const duplicateRadioGroup = style({
  display: "flex",
  gap: 16,
  alignItems: "center",
  fontSize: 13,
  color: vars.color.textSecondary,
});

export const duplicateRadioLabel = style({
  display: "flex",
  gap: 6,
  alignItems: "center",
  cursor: "pointer",
});

export const applyButton = style({
  width: "100%",
  padding: "10px 16px",
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: 8,
  fontSize: 14,
  fontWeight: 600,
  cursor: "pointer",
  transition: "background 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.primaryHover,
    },
    "&:disabled": {
      opacity: 0.4,
      cursor: "not-allowed",
    },
  },
});

export const noValidRulesMessage = style({
  color: vars.color.textMuted,
  fontSize: 13,
  textAlign: "center",
  padding: "8px 0",
});

export const exampleToggle = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  fontSize: 12,
  textDecoration: "underline",
  padding: 0,
});

export const exampleBlock = style({
  background: vars.color.surfaceMuted,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: 6,
  padding: "8px 12px",
  fontSize: 11,
  fontFamily: "monospace",
  color: vars.color.textSecondary,
  whiteSpace: "pre",
  overflowX: "auto",
  marginTop: 6,
});

export const sectionLabel = style({
  fontSize: 12,
  fontWeight: 600,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.04em",
  marginBottom: 4,
});

export const formStack = style({
  display: "flex",
  flexDirection: "column",
  gap: 16,
});

export const partialErrorBanner = style({
  padding: "8px 12px",
  background: vars.color.errorBg,
  color: vars.color.error,
  borderRadius: 6,
  fontSize: 13,
});
