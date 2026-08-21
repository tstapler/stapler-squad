import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const previewContainer = style({
  marginTop: "8px",
  borderTop: `1px solid ${vars.color.borderColor}`,
  paddingTop: "8px",
  display: "flex",
  flexDirection: "column",
  gap: "6px",
});

export const previewLoading = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const previewError = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.warning,
});

export const previewEmpty = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const previewMessage = style({
  display: "flex",
  gap: "6px",
  alignItems: "flex-start",
  fontSize: vars.fontSize.xs,
  lineHeight: "1.4",
  padding: "4px 6px",
  borderRadius: vars.radii.sm,
});

export const userMessage = style({
  background: vars.color.accentBg,
});

export const assistantMessage = style({
  background: "transparent",
});

export const previewRole = style({
  flexShrink: 0,
  fontSize: "11px",
  lineHeight: "1.4",
});

export const previewContent = style({
  color: vars.color.textSecondary,
  wordBreak: "break-word",
  overflow: "hidden",
  display: "-webkit-box",
  WebkitBoxOrient: "vertical",
  WebkitLineClamp: 2,
});
