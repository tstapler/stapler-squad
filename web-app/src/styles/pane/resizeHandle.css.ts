import { recipe } from "@vanilla-extract/recipes";
import { vars, zIndex } from "@/styles/theme.css";

/**
 * Resize handle: 6px visual width/height, but with negative margins + padding to expand
 * the hit target to ~20px for mobile touch usability (US-5).
 *
 * The ::after pseudo-element renders a subtle indicator bar that brightens on hover.
 */
export const resizeHandle = recipe({
  base: {
    position: "relative",
    flexShrink: 0,
    zIndex: zIndex.raised,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    userSelect: "none",
    // touchAction: none must be inline style — vanilla-extract doesn't support it in recipes
    transition: "background 120ms ease",
    background: vars.color.cardBackground,
    "::after": {
      content: "''",
      display: "block",
      borderRadius: vars.radii.full,
      background: vars.color.borderColor,
      opacity: 0.6,
      transition: "opacity 120ms ease, background 120ms ease",
    },
    selectors: {
      "&:hover::after": {
        opacity: 1,
        background: vars.color.primary,
      },
      "&:active::after": {
        opacity: 1,
        background: vars.color.primaryHover,
      },
    },
  },
  variants: {
    direction: {
      vertical: {
        // Visual: 6px wide column; hit target: 20px via negative margins
        width: "6px",
        cursor: "col-resize",
        marginLeft: "-7px",
        marginRight: "-7px",
        paddingLeft: "7px",
        paddingRight: "7px",
        "::after": {
          width: "2px",
          height: "24px",
        },
      },
      horizontal: {
        // Visual: 6px tall row; hit target: 20px via negative margins
        height: "6px",
        cursor: "row-resize",
        marginTop: "-7px",
        marginBottom: "-7px",
        paddingTop: "7px",
        paddingBottom: "7px",
        "::after": {
          width: "24px",
          height: "2px",
        },
      },
    },
  },
});
