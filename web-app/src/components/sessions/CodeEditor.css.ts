import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const codeEditorRoot = style({ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 });

export const codeEditor = style({
  flex: 1,
  minHeight: 0,
  height: "100%",
});

export const degradeNotice = style({
  flexShrink: 0,
  padding: "4px 12px",
  fontSize: 12,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  borderBottom: `1px solid ${vars.color.warning}`,
});

// The font size is set by the engine (EngineHandle ModeConfig.fontPx; 16px floor on touch, so focus never zooms the page).
globalStyle(`${codeEditor} .cm-editor`, {
  height: "100%",
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

// A gradient layer, not backgroundColor: it composes over the theme's opaque .cm-activeLine background instead of losing to it.
const tint = (c: string) => ({ backgroundImage: `linear-gradient(${c}, ${c})` });
export const lineBackgroundAdd = style(tint(vars.color.gitAddedBg));
export const lineBackgroundDelete = style(tint(vars.color.gitDeletedBg));
export const lineBackgroundModify = style(tint(vars.color.gitModifiedBg));
