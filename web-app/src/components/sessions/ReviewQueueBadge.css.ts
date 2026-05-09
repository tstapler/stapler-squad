import { style, keyframes } from "@vanilla-extract/css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.7 },
});

export const badge = style({
  display: "flex",
  gap: "8px",
  alignItems: "center",
  flexWrap: "wrap",
});

export const priorityAbbr = style({
  display: "none",
  fontSize: "10px",
  fontWeight: 700,
  letterSpacing: "0.02em",
  "@media": {
    "(min-width: 768px)": {
      display: "inline",
    },
  },
});

export const badgeCompact = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "24px",
  height: "24px",
  borderRadius: "50%",
  fontSize: "14px",
  cursor: "help",
  transition: "transform 0.2s ease",
  selectors: {
    "&:hover": {
      transform: "scale(1.2)",
    },
  },
});

const sharedBadge = style({
  padding: "4px 12px",
  borderRadius: "12px",
  fontSize: "12px",
  fontWeight: 600,
  textTransform: "uppercase",
  whiteSpace: "nowrap",
});

export const priority = style([
  sharedBadge,
  {
    display: "flex",
    alignItems: "center",
    gap: "4px",
  },
]);

export const reason = style([sharedBadge]);

export const priorityUrgent = style({
  background: "#fee2e2",
  color: "#991b1b",
  border: "1px solid #fca5a5",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#7f1d1d",
      color: "#fecaca",
      borderColor: "#991b1b",
    },
  },
});

export const priorityHigh = style({
  background: "#fef3c7",
  color: "#92400e",
  border: "1px solid #fcd34d",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#78350f",
      color: "#fef3c7",
      borderColor: "#92400e",
    },
  },
});

export const priorityMedium = style({
  background: "#dbeafe",
  color: "#1e40af",
  border: "1px solid #93c5fd",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#1e3a8a",
      color: "#dbeafe",
      borderColor: "#1e40af",
    },
  },
});

export const priorityLow = style({
  background: "#f3f4f6",
  color: "#374151",
  border: "1px solid #d1d5db",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#374151",
      color: "#d1d5db",
      borderColor: "#4b5563",
    },
  },
});

export const priorityUnspecified = style({
  background: "#f9fafb",
  color: "#4b5563",
  border: "1px solid #e5e7eb",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#1f2937",
      color: "#9ca3af",
      borderColor: "#374151",
    },
  },
});

export const reasonApproval = style({
  background: "#fef3c7",
  color: "#92400e",
  border: "1px solid #fcd34d",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#78350f",
      color: "#fef3c7",
      borderColor: "#92400e",
    },
  },
});

export const reasonInput = style({
  background: "#dbeafe",
  color: "#1e40af",
  border: "1px solid #93c5fd",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#1e3a8a",
      color: "#dbeafe",
      borderColor: "#1e40af",
    },
  },
});

export const reasonError = style({
  background: "#fee2e2",
  color: "#991b1b",
  border: "1px solid #fca5a5",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#7f1d1d",
      color: "#fecaca",
      borderColor: "#991b1b",
    },
  },
});

export const reasonIdle = style({
  background: "#e0e7ff",
  color: "#4338ca",
  border: "1px solid #a5b4fc",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#312e81",
      color: "#e0e7ff",
      borderColor: "#3730a3",
    },
  },
});

export const reasonComplete = style({
  background: "#dcfce7",
  color: "#166534",
  border: "1px solid #86efac",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#14532d",
      color: "#dcfce7",
      borderColor: "#166534",
    },
  },
});

export const reasonUnspecified = style({
  background: "#f3f4f6",
  color: "#4b5563",
  border: "1px solid #d1d5db",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#374151",
      color: "#9ca3af",
      borderColor: "#4b5563",
    },
  },
});
