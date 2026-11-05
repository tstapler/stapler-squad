import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/** Spans the whole info grid: the list is wider than a label/value cell. */
export const wide = style({ gridColumn: "1 / -1", minWidth: 0 });

export const legacyLink = style({ color: vars.color.primary });
