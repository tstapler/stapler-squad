import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[3],
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  margin: 0,
});

export const fileListContainer = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  overflowX: "auto",
});

const fileRowBase = {
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  width: "100%",
  minHeight: 44,
  padding: `${vars.space[1]} ${vars.space[2]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  textAlign: "left" as const,
  whiteSpace: "nowrap" as const,
};

export const fileRow = style({
  ...fileRowBase,
  border: "none",
  background: "transparent",
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const fileRowStatic = style(fileRowBase);

export const statusIcon = style({
  flexShrink: 0,
  color: vars.color.textSecondary,
});

export const filePath = style({
  overflow: "hidden",
  textOverflow: "ellipsis",
  fontFamily: vars.font.mono,
});

export const fileStats = style({
  display: "flex",
  gap: vars.space[1],
  marginLeft: "auto",
  flexShrink: 0,
});

export const additions = style({ color: vars.color.success });
export const deletions = style({ color: vars.color.errorText });

export const showAllButton = style({
  alignSelf: "flex-start",
  minHeight: 44,
  padding: `${vars.space[1]} ${vars.space[2]}`,
  border: "none",
  background: "transparent",
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
});
