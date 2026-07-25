import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Shared modal chrome (overlay, content, step indicator, headline, body,
// footer/buttons, kbd, checkbox row) now lives in a common location so other
// walkthrough modals (e.g. BacklogTourModal) don't have to reach into this
// feature's implementation-detail CSS module — see
// .claude/rules/css-architecture.md. Re-exported here so this file's existing
// imports keep working unchanged.
export * from "@/components/ui/ModalTour.css";

export const header = style({
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  marginBottom: vars.space["4"],
});

export const asciiDiagram = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  padding: vars.space["3"],
  background: vars.color.cardBackground,
  borderRadius: vars.radii.sm,
  whiteSpace: "pre",
  marginBottom: vars.space["4"],
  lineHeight: "1.5",
  border: `1px solid ${vars.color.borderColor}`,
});

export const shortcutTable = style({
  width: "100%",
  borderCollapse: "collapse" as const,
  marginBottom: vars.space["4"],
});

export const shortcutRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: `${vars.space["1"]} 0`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
  },
});

export const shortcutLabel = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const linkButton = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.primary,
  background: "none",
  border: "none",
  cursor: "pointer",
  textDecoration: "underline",
  transition: vars.transition.fast,
  selectors: {
    "&:hover": {
      color: vars.color.primaryHover,
    },
  },
});
