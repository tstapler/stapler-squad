import { createTheme, style } from "@vanilla-extract/css";

export const [themeClass, vars] = createTheme({ canary: { color: "#123456" } });

export const box = style({ color: vars.canary.color, padding: 4 });
