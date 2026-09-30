import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  // Narrower than PlanVerdictBox.css.ts's space["4"] intentionally: this
  // section has no title/sub-heading, so it doesn't need that extra gap.
  gap: vars.space["2"],
});

// Button/form primitives shared with ../PlanVerdictBox.css.ts, see ../sharedFormStyles.css.ts.
export { secondaryButton, primaryButton as submitButton, form, formLabel, formTextarea, formActions } from "../sharedFormStyles.css";
