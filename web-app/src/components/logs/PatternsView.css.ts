import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  flex: 1,
  overflow: "auto",
  minHeight: 0,
  backgroundColor: vars.color.background,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
});

export const row = style({
  borderBottom: `1px solid ${vars.color.cardBackground}`,
});

export const rowHeader = style({
  display: "flex",
  alignItems: "center",
  gap: "0.75rem",
  padding: "0.6rem 0.75rem",
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.cardBackground,
    },
  },
});

export const disclosure = style({
  width: "1rem",
  flexShrink: 0,
  color: vars.color.textMuted,
  transition: "transform 0.1s",
});

export const disclosureOpen = style({
  transform: "rotate(90deg)",
});

export const count = style({
  flexShrink: 0,
  minWidth: "3.5rem",
  textAlign: "right",
  padding: "0.1rem 0.4rem",
  borderRadius: vars.radii.sm,
  backgroundColor: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontWeight: 600,
});

export const level = style({
  flexShrink: 0,
  minWidth: "4.5rem",
  fontWeight: 600,
  textTransform: "uppercase",
});

export const levelDebug = style({ color: vars.color.textMuted });
export const levelInfo = style({ color: vars.color.primary });
export const levelWarning = style({ color: vars.color.warning });
export const levelError = style({ color: vars.color.error });

export const pattern = style({
  flex: 1,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  color: vars.color.textPrimary,
});

export const placeholder = style({
  color: vars.color.primary,
  fontWeight: 500,
});

export const examples = style({
  padding: "0 0.75rem 0.75rem 2.75rem",
  display: "flex",
  flexDirection: "column",
  gap: "0.35rem",
});

export const example = style({
  display: "flex",
  gap: "0.75rem",
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  color: vars.color.textMuted,
});

export const exampleTimestamp = style({
  flexShrink: 0,
  whiteSpace: "nowrap",
});

export const exampleMessage = style({
  color: vars.color.textPrimary,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
});

export const examplesMore = style({
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const empty = style({
  padding: "3rem 2rem",
  textAlign: "center",
  color: vars.color.textMuted,
});
