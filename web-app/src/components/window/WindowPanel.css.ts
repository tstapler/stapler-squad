import { style } from "@vanilla-extract/css";

// Fills the page's flex column exactly as the cockpit container did before the wrapper existed.
export const windowPanel = style({
  flex: 1,
  minHeight: 0,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  outline: "none",
  // An author `display` beats the UA `[hidden]` rule, so a keep-alive hidden window would stay visible without this.
  selectors: { "&[hidden]": { display: "none" } },
});
