import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: `0 ${vars.space["2"]}`,
  background: vars.color.error,
  color: vars.color.textInverse,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  borderRadius: vars.radii.full,
  lineHeight: 1,
  pointerEvents: "none",
});

export const inline = style({
  marginLeft: vars.space["2"],
  verticalAlign: "middle",
});

// Neutral pulse/skeleton placeholder shown before the first successful fetch
// resolves — never a bare "0" (design/ux.md Surface 1 / AC 24).
export const skeleton = style({
  display: "inline-flex",
  width: "1.25rem",
  height: "1.25rem",
  borderRadius: vars.radii.full,
  background: vars.color.surfaceMuted,
  animation: "stuckBadgePulse 1.4s ease-in-out infinite",
  verticalAlign: "middle",
  "@keyframes": {
    stuckBadgePulse: {
      "0%, 100%": { opacity: 0.4 },
      "50%": { opacity: 0.9 },
    },
  },
} as Parameters<typeof import("@vanilla-extract/css").style>[0]);
