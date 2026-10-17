import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const codeEditor = style({
  height: "100%",
});

// Soft keyboards zoom the page when a focused field renders below 16px.
globalStyle(`${codeEditor} .cm-editor`, {
  height: "100%",
  fontSize: 13,
  "@media": {
    "(pointer: coarse)": { fontSize: 16 },
  },
});

globalStyle(`${codeEditor} .cm-changeGutter`, {
  width: 4,
});

const gutterMarker = style({
  width: 4,
  height: "100%",
});

export const gutterMarkerAdd = style([gutterMarker, { background: vars.color.gitAdded }]);
export const gutterMarkerDelete = style([gutterMarker, { background: vars.color.gitDeleted }]);
export const gutterMarkerModify = style([gutterMarker, { background: vars.color.gitModified }]);
