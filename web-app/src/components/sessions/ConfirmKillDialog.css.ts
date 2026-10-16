import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import {
  animatedOverlay,
  animatedModal,
  animatedHeader,
  animatedTitle,
  animatedSubtitleBase,
} from "@/styles/modalChrome.css";

export const overlay = animatedOverlay;
export const modal = animatedModal;
export const header = animatedHeader;
export const title = animatedTitle;

export const subtitle = style([animatedSubtitleBase, { lineHeight: 1.5 }]);

export const body = style({
  padding: "24px",
  overflowY: "auto",
  flex: 1,
  display: "flex",
  flexDirection: "column",
  gap: "12px",
});

export const errorState = style({
  padding: "12px 14px",
  background: vars.color.errorBg,
  color: vars.color.errorText,
  borderRadius: "6px",
  fontSize: "13px",
  lineHeight: 1.5,
});

export const optionCard = style({
  display: "flex",
  flexDirection: "column",
  gap: "4px",
  padding: "12px 14px",
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: "6px",
});

export const optionTitle = style({
  fontSize: "13px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const optionDescription = style({
  fontSize: "13px",
  color: vars.color.textSecondary,
  lineHeight: 1.5,
});

export const footer = style({
  padding: "16px 24px",
  borderTop: `1px solid ${vars.color.modalBorder}`,
  display: "flex",
  justifyContent: "flex-end",
  gap: "12px",
});

export const cancelButton = style({
  padding: "10px 24px",
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: "6px",
  fontSize: "14px",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderStrong,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const killButton = style({
  padding: "10px 24px",
  background: vars.color.error,
  color: vars.color.textInverse,
  border: "none",
  borderRadius: "6px",
  fontSize: "14px",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      filter: "brightness(1.1)",
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});
