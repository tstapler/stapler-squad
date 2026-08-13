import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const wizard = style({
  display: "flex",
  flexDirection: "column",
  gap: "2rem",
  width: "100%",
  maxWidth: "800px",
  margin: "0 auto",
  "@media": {
    "screen and (max-width: 768px)": {
      gap: "1.5rem",
    },
  },
});

export const steps = style({
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  position: "relative",
  padding: "0 2rem",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0 1rem",
    },
    "screen and (max-width: 640px)": {
      padding: "0 0.5rem",
    },
  },
});

export const step = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: "0.5rem",
  flex: 1,
  position: "relative",
});

export const stepNumber = style({
  width: "40px",
  height: "40px",
  borderRadius: "50%",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  fontWeight: 600,
  fontSize: "1rem",
  background: vars.color.borderSubtle,
  color: vars.color.textSecondary,
  transition: "all 0.3s",
  zIndex: 2,
  "@media": {
    "screen and (max-width: 768px)": {
      width: "32px",
      height: "32px",
      fontSize: "0.875rem",
    },
  },
});

export const stepLabel = style({
  fontSize: "0.875rem",
  fontWeight: 500,
  color: vars.color.textSecondary,
  textAlign: "center",
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "0.75rem",
    },
    "screen and (max-width: 640px)": {
      display: "none",
    },
  },
});

export const stepConnector = style({
  position: "absolute",
  top: "20px",
  left: "50%",
  right: "-50%",
  height: "2px",
  background: vars.color.borderMuted,
  zIndex: 1,
  "@media": {
    "screen and (max-width: 768px)": {
      top: "16px",
    },
  },
});

export const active = style({});

export const activeStepNumber = style({
  background: `${vars.color.primary} !important` as "inherit",
  color: "white !important" as "inherit",
  boxShadow: "0 0 0 4px rgba(0, 112, 243, 0.2)",
});

export const activeStepLabel = style({
  color: `${vars.color.primary} !important` as "inherit",
  fontWeight: "600 !important" as "inherit",
});

export const completed = style({});

export const completedStepNumber = style({
  background: `${vars.color.success} !important` as "inherit",
  color: "white !important" as "inherit",
});

export const completedStepLabel = style({
  color: `${vars.color.success} !important` as "inherit",
});

export const completedStepConnector = style({
  background: `${vars.color.success} !important` as "inherit",
});

export const pending = style({});

export const content = style({
  background: vars.color.cardBackground,
  borderRadius: "8px",
  padding: "2rem",
  boxShadow: "0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1.5rem",
    },
    "screen and (max-width: 640px)": {
      padding: "1rem",
    },
  },
});

export const wizardActions = style({
  display: "flex",
  gap: "1rem",
  justifyContent: "flex-end",
  paddingTop: "1.5rem",
  borderTop: `1px solid ${vars.color.borderColor}`,
  marginTop: "1.5rem",
  "@media": {
    "screen and (max-width: 768px)": {
      flexDirection: "column-reverse",
    },
  },
});
