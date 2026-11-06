import { createThemeContract, createTheme, style, globalStyle, keyframes } from "@vanilla-extract/css";

export const vars = createThemeContract({ color: { bg: null } });
export const lightTheme = createTheme(vars, { color: { bg: "#fff" } });
export const darkTheme = createTheme(vars, { color: { bg: "#000" } });
export const matrixTheme = createTheme(vars, { color: { bg: "#010" } });
export const cyberpunk77Theme = createTheme(vars, { color: { bg: "#220" } });
export const wh40kTheme = createTheme(vars, { color: { bg: "#200" } });
export const cleanTheme = createTheme(vars, { color: { bg: "#eee" } });

const spin = keyframes({ from: { opacity: 0 }, to: { opacity: 1 } });
export const box = style({ background: vars.color.bg, animation: `${spin} 1s` });
globalStyle("body", { margin: 0 });
