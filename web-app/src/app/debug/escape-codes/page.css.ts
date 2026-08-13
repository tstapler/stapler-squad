import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  padding: "2rem",
  maxWidth: "1400px",
  margin: "0 auto",
  minHeight: "var(--viewport-height, 100dvh)",
  background: vars.color.background,
  color: vars.color.textPrimary,
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1rem",
    },
  },
});

export const header = style({
  marginBottom: "2rem",
});

export const subtitle = style({
  color: vars.color.textSecondary,
  margin: 0,
});

export const controls = style({
  display: "flex",
  gap: "0.75rem",
  marginBottom: "1.5rem",
  flexWrap: "wrap",
  "@media": {
    "screen and (max-width: 768px)": {
      flexDirection: "column",
    },
  },
});

export const button = style({
  padding: "0.5rem 1rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.background,
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontSize: "0.875rem",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      width: "100%",
    },
  },
});

export const buttonActive = style({
  background: vars.color.success,
  borderColor: vars.color.success,
  color: vars.color.primaryText,
  selectors: {
    "&:hover": {
      background: vars.color.successBg,
    },
  },
});

export const buttonDanger = style({
  borderColor: vars.color.error,
  color: vars.color.error,
  selectors: {
    "&:hover": {
      background: vars.color.error,
      color: vars.color.primaryText,
    },
  },
});

export const error = style({
  padding: "1rem",
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "6px",
  color: vars.color.errorText,
  marginBottom: "1.5rem",
});

export const statsGrid = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(120px, 1fr))",
  gap: "1rem",
  marginBottom: "1.5rem",
  "@media": {
    "screen and (max-width: 768px)": {
      gridTemplateColumns: "repeat(2, 1fr)",
    },
  },
});

export const statCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  padding: "1rem",
  textAlign: "center",
});

export const statValue = style({
  fontSize: "1.5rem",
  fontWeight: 700,
  color: vars.color.primary,
});

export const statLabel = style({
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  marginTop: "0.25rem",
});

export const filters = style({
  display: "flex",
  gap: "1rem",
  marginBottom: "1rem",
  flexWrap: "wrap",
});

export const select = style({
  padding: "0.375rem 0.75rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.background,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
});

export const loading = style({
  textAlign: "center",
  padding: "3rem",
  color: vars.color.textSecondary,
});

export const tableContainer = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  overflow: "hidden",
  "@media": {
    "screen and (max-width: 768px)": {
      overflowX: "auto",
    },
  },
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: "0.875rem",
});

export const hexCode = style({
  fontFamily: "monospace",
  fontSize: "0.8rem",
  background: vars.color.terminalBackground,
  padding: "0.25rem 0.5rem",
  borderRadius: "4px",
  cursor: "pointer",
  display: "inline-block",
  wordBreak: "break-all",
  maxWidth: "200px",
  selectors: {
    "&:hover": {
      background: vars.color.borderMuted,
    },
  },
});

export const badge = style({
  display: "inline-block",
  padding: "0.125rem 0.5rem",
  borderRadius: "9999px",
  fontSize: "0.75rem",
  fontWeight: 500,
  textTransform: "capitalize",
});

export const badgeCSI = style({ background: "#3b82f6", color: "white" });
export const badgeOSC = style({ background: "#8b5cf6", color: "white" });
export const badgeDCS = style({ background: "#ec4899", color: "white" });
export const badgeDECPriv = style({ background: "#f59e0b", color: "black" });
export const badgeSGR = style({ background: "#10b981", color: "white" });
export const badgeCursor = style({ background: "#06b6d4", color: "white" });
export const badgeErase = style({ background: "#ef4444", color: "white" });
export const badgeScroll = style({ background: "#84cc16", color: "black" });
export const badgeSimple = style({ background: "#64748b", color: "white" });
export const badgeCharset = style({ background: "#a855f7", color: "white" });
export const badgeUnknown = style({ background: "#374151", color: "white" });

export const countCell = style({
  fontWeight: 600,
  textAlign: "right",
  fontFamily: "monospace",
});

export const dateCell = style({
  whiteSpace: "nowrap",
  color: vars.color.textSecondary,
});

export const sessionsCell = style({
  textAlign: "center",
});

export const emptyState = style({
  textAlign: "center",
  padding: "3rem",
  color: vars.color.textSecondary,
});
