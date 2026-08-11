import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.7 },
});

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "var(--viewport-height, 100dvh)",
  overflow: "hidden",
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Oxygen', 'Ubuntu', 'Cantarell', 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "1rem 2rem",
  background: vars.color.terminalHeaderBg,
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
});

export const stats = style({
  display: "flex",
  gap: "1.5rem",
  fontSize: "0.9rem",
  color: vars.color.terminalForeground,
});

export const mainContent = style({
  display: "flex",
  flex: 1,
  minHeight: 0,
  "@media": {
    "screen and (max-width: 1024px)": {
      flexDirection: "column",
    },
  },
});

export const leftPanel = style({
  width: "400px",
  display: "flex",
  flexDirection: "column",
  borderRight: `1px solid ${vars.color.terminalBorder}`,
  background: vars.color.terminalHeaderBg,
  "@media": {
    "screen and (max-width: 1400px)": {
      width: "350px",
    },
    "screen and (max-width: 1024px)": {
      width: "100%",
      border: "none",
      borderBottom: `1px solid ${vars.color.terminalBorder}`,
      maxHeight: "300px",
    },
  },
});

export const panelHeader = style({
  padding: "1rem",
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
});

export const filters = style({
  display: "flex",
  gap: "0.5rem",
});

export const select = style({
  flex: 1,
  padding: "0.5rem",
  background: vars.color.terminalHeaderBg,
  border: `1px solid ${vars.color.terminalBorder}`,
  borderRadius: "4px",
  color: vars.color.terminalForeground,
  fontSize: "0.875rem",
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
      borderColor: vars.color.terminalBorder,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const presets = style({
  padding: "1rem",
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
});

export const presetButtons = style({
  display: "grid",
  gridTemplateColumns: "repeat(2, 1fr)",
  gap: "0.5rem",
  "@media": {
    "screen and (max-width: 1200px)": {
      gridTemplateColumns: "1fr",
    },
  },
});

export const presetButton = style({
  padding: "0.5rem",
  background: vars.color.terminalHeaderBg,
  border: `1px solid ${vars.color.terminalBorder}`,
  borderRadius: "4px",
  color: vars.color.terminalForeground,
  fontSize: "0.8rem",
  cursor: "pointer",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
      borderColor: vars.color.terminalBorder,
      transform: "translateY(-1px)",
    },
  },
});

export const codeList = style({
  flex: 1,
  overflowY: "auto",
  padding: "0.5rem",
});

export const codeItem = style({
  marginBottom: "0.5rem",
  padding: "0.75rem",
  background: vars.color.terminalHeaderBg,
  border: `1px solid ${vars.color.terminalBorder}`,
  borderRadius: "4px",
  cursor: "pointer",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
      borderColor: vars.color.terminalBorder,
    },
    "&.selected": {
      background: vars.color.terminalHoverBg,
      borderColor: vars.color.success,
      boxShadow: `0 0 0 1px ${vars.color.success} inset`,
    },
  },
});

export const selected = style({
  background: vars.color.terminalHoverBg,
  borderColor: vars.color.success,
  boxShadow: `0 0 0 1px ${vars.color.success} inset`,
});

export const codeHeader = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  marginBottom: "0.5rem",
});

export const codeTitle = style({
  flex: 1,
  fontSize: "0.9rem",
  fontWeight: 500,
  color: vars.color.terminalForeground,
});

export const priority = style({
  padding: "0.125rem 0.5rem",
  borderRadius: "3px",
  fontSize: "0.75rem",
  fontWeight: 600,
  textTransform: "uppercase",
});

export const critical = style({
  background: vars.color.error,
  color: vars.color.primaryText,
});

export const high = style({
  background: vars.color.warning,
  color: vars.color.primaryText,
});

export const medium = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
});

export const low = style({
  background: vars.color.panelBgSecondary,
  color: vars.color.textSecondary,
});

export const codeDetails = style({
  display: "flex",
  gap: "0.75rem",
  fontSize: "0.8rem",
  color: vars.color.terminalTextMuted,
  paddingLeft: "1.5rem",
});

export const category = style({
  padding: "0.125rem 0.375rem",
  background: vars.color.terminalBackground,
  borderRadius: "3px",
  color: vars.color.terminalForeground,
});

export const count = style({
  color: vars.color.terminalTextMuted,
});

export const sequence = style({
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace",
  color: vars.color.warningText,
  fontSize: "0.75rem",
});

export const middlePanel = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  background: vars.color.terminalBackground,
  "@media": {
    "screen and (max-width: 1024px)": {
      minHeight: "400px",
    },
  },
});

export const terminalHeader = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "1rem",
  background: vars.color.terminalHeaderBg,
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
});

export const currentCode = style({
  padding: "0.25rem 0.75rem",
  background: vars.color.terminalHoverBg,
  borderRadius: "4px",
  fontSize: "0.875rem",
  color: vars.color.success,
  animationName: pulse,
  animationDuration: "1s",
  animationIterationCount: "infinite",
});

export const terminal = style({
  flex: 1,
  padding: "1rem",
  background: vars.color.terminalBackground,
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace",
  fontSize: "14px",
  lineHeight: "1.5",
  overflowY: "auto",
  color: vars.color.terminalForeground,
});

export const placeholder = style({
  color: vars.color.terminalTextMuted,
  fontStyle: "italic",
  textAlign: "center",
  marginTop: "2rem",
});

export const terminalLine = style({
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
});

export const rightPanel = style({
  width: "350px",
  display: "flex",
  flexDirection: "column",
  borderLeft: `1px solid ${vars.color.terminalBorder}`,
  background: vars.color.terminalHeaderBg,
  "@media": {
    "screen and (max-width: 1400px)": {
      width: "300px",
    },
    "screen and (max-width: 1024px)": {
      width: "100%",
      border: "none",
      borderTop: `1px solid ${vars.color.terminalBorder}`,
    },
  },
});

export const controls = style({
  padding: "1rem",
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
});

export const controlGroup = style({
  marginBottom: "1rem",
});

export const input = style({
  width: "100%",
  padding: "0.5rem",
  background: vars.color.terminalHoverBg,
  border: `1px solid ${vars.color.terminalBorder}`,
  borderRadius: "4px",
  color: vars.color.terminalForeground,
  fontSize: "0.875rem",
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
      borderColor: vars.color.terminalBorder,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const actionButtons = style({
  display: "flex",
  gap: "0.5rem",
  marginTop: "1rem",
});

export const startButton = style({
  flex: 1,
  padding: "0.75rem",
  border: "none",
  borderRadius: "4px",
  fontSize: "0.9rem",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s",
  background: vars.color.success,
  color: vars.color.terminalBackground,
  selectors: {
    "&:hover": {
      background: vars.color.successBg,
      transform: "translateY(-1px)",
    },
  },
});

export const stopButton = style({
  flex: 1,
  padding: "0.75rem",
  border: "none",
  borderRadius: "4px",
  fontSize: "0.9rem",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s",
  background: vars.color.error,
  color: vars.color.terminalBackground,
  selectors: {
    "&:hover": {
      background: vars.color.errorBg,
      transform: "translateY(-1px)",
    },
  },
});

export const metrics = style({
  padding: "1rem",
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
});

export const metricItem = style({
  marginBottom: "1rem",
});

export const metricValue = style({
  display: "block",
  fontSize: "1.25rem",
  fontWeight: 600,
  color: vars.color.terminalForeground,
  marginBottom: "0.5rem",
});

export const progressBar = style({
  height: "8px",
  background: vars.color.terminalHoverBg,
  borderRadius: "4px",
  overflow: "hidden",
});

export const progressFill = style({
  height: "100%",
  background: vars.color.success,
  transition: "width 0.3s ease",
});

export const errors = style({
  marginTop: "1rem",
  padding: "0.75rem",
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "4px",
});

export const error = style({
  fontSize: "0.8rem",
  color: vars.color.error,
  marginBottom: "0.25rem",
});

export const categoryStats = style({
  padding: "1rem",
});

export const categoryStat = style({
  marginBottom: "0.75rem",
});

export const categoryName = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "0.25rem",
  fontSize: "0.85rem",
  color: vars.color.terminalForeground,
});

export const categoryCount = style({
  fontSize: "0.75rem",
  color: vars.color.terminalTextMuted,
});
