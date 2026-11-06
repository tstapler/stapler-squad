import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  height: "100%",
  overflow: "hidden",
  background: vars.color.terminalBackground,
  containerType: "inline-size",
  containerName: "filesTab",
  selectors: { "&[data-layout='mobile']": { flexDirection: "column" } },
});

// The mobile layout is chosen by `chooseFilesLayout` (container width OR phone-class touch device)
// and written to the container's data attributes, not by a container query, so a landscape phone
// 844px wide still gets it. `data-pane` says which of the three nav-stack panes is showing.
const mobileLayout = `${container}[data-layout='mobile']`;
const mobileLegacy = `${mobileLayout}[data-chrome='legacy']`;

export const treePane = style({
  display: "flex",
  flexDirection: "column",
  flexShrink: 0,
  borderRight: `1px solid ${vars.color.borderColor}`,
  overflow: "hidden",
  selectors: {
    // Inline width (panel.width px) would leave a black gap — override to fill.
    [`${mobileLayout} &`]: { width: "100% !important", borderRight: "none", flex: 1, minHeight: 0 },
    [`${mobileLayout}[data-pane='editor'] &`]: { display: "none" },
  },
});

export const treePaneCollapsed = style({
  width: "0 !important",
  overflow: "hidden",
  borderRight: "none",
  minWidth: "0 !important",
  // The toolbar's own expand button is clipped to nothing here; hide the pane so only the strip below answers to it.
  visibility: "hidden",
});

export const expandStrip = style({
  flexShrink: 0,
  width: "24px",
  border: "none",
  borderRight: `1px solid ${vars.color.borderColor}`,
  background: vars.color.terminalBackground,
  color: vars.color.textPrimary,
  cursor: "pointer",
  selectors: { [`${mobileLayout} &`]: { display: "none" } },
});

export const mobileBackButton = style({
  display: "none",
  selectors: {
    [`${mobileLegacy} &`]: {
      display: "flex",
      alignItems: "center",
      gap: 4,
      padding: "8px 12px",
      background: "transparent",
      border: "none",
      cursor: "pointer",
      color: vars.color.primary,
      fontSize: 13,
      borderBottom: `1px solid ${vars.color.borderColor}`,
    },
  },
});

export const contentPane = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  selectors: {
    [`${mobileLayout}:not([data-pane='editor']) &`]: { display: "none" },
    [`${mobileLayout} &`]: { width: "100%", minHeight: 0 },
  },
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  gap: 6,
  padding: "5px 8px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.terminalTabsBg,
  flexShrink: 0,
  selectors: {
    // Left-handed: flip the toolbar row so action buttons (collapse, expand, refresh)
    // land on the left thumb side; search input follows naturally
    ':root[data-left-handed] &': {
      flexDirection: "row-reverse",
    },
  },
});

export const searchInput = style({
  flex: 1,
  minWidth: 0,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 4,
  color: vars.color.terminalForeground,
  fontSize: 12,
  padding: "3px 8px",
  outline: "none",
  selectors: {
    "&:focus": {
      borderColor: vars.color.primary,
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
});

export const toolbarLabel = style({
  display: "flex",
  alignItems: "center",
  gap: 3,
  fontSize: 11,
  color: vars.color.textMuted,
  cursor: "pointer",
  flexShrink: 0,
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      color: vars.color.terminalForeground,
    },
  },
});

globalStyle(`${toolbarLabel} input[type='checkbox']`, {
  cursor: "pointer",
  accentColor: vars.color.primary,
});

export const toolbarButton = style({
  flexShrink: 0,
  padding: "2px 7px",
  fontSize: 13,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 3,
  color: vars.color.textMuted,
  cursor: "pointer",
  lineHeight: 1.4,
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.terminalHoverBg,
      color: vars.color.terminalForeground,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "default",
    },
  },
});

export const searchCount = style({
  flexShrink: 0,
  fontSize: 11,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const treeWrapper = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
});

// Fix 2: hide collapse/expand panel buttons on mobile — toolbar has too many buttons
export const toolbarButtonMobileHidden = style({
  selectors: { [`${mobileLayout} &`]: { display: "none" } },
});

// Fix 5: search icon button shown only on mobile, in the content pane header
export const mobileSearchButton = style({
  display: "none",
  selectors: {
    [`${mobileLegacy} &`]: {
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      padding: "8px 12px",
      background: "transparent",
      border: "none",
      cursor: "pointer",
      color: vars.color.primary,
      fontSize: 16,
      borderBottom: `1px solid ${vars.color.borderColor}`,
    },
  },
});

// Fix 6: visual separator between ⊟ and ⊠/⊞ buttons
export const toolbarDivider = style({
  width: 1,
  alignSelf: "stretch",
  background: vars.color.borderColor,
  flexShrink: 0,
  margin: "2px 2px",
});

// A left-pane view (Explorer or Search); `hidden` keeps its state mounted while the other shows.
export const leftView = style({
  display: "flex",
  flexDirection: "column",
  flex: 1,
  minHeight: 0,
  selectors: { "&[hidden]": { display: "none" } },
});
