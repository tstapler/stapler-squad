import { style, keyframes } from "@vanilla-extract/css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.7 },
});

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "var(--viewport-height, 100dvh)",
  overflow: "hidden",
  background: "#1e1e1e",
  color: "#d4d4d4",
  fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Oxygen', 'Ubuntu', 'Cantarell', 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "1rem 2rem",
  background: "#2d2d30",
  borderBottom: "1px solid #3e3e42",
});

export const stats = style({
  display: "flex",
  gap: "1.5rem",
  fontSize: "0.9rem",
  color: "#cccccc",
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
  borderRight: "1px solid #3e3e42",
  background: "#252526",
  "@media": {
    "screen and (max-width: 1400px)": {
      width: "350px",
    },
    "screen and (max-width: 1024px)": {
      width: "100%",
      border: "none",
      borderBottom: "1px solid #3e3e42",
      maxHeight: "300px",
    },
  },
});

export const panelHeader = style({
  padding: "1rem",
  borderBottom: "1px solid #3e3e42",
});

export const filters = style({
  display: "flex",
  gap: "0.5rem",
});

export const select = style({
  flex: 1,
  padding: "0.5rem",
  background: "#3e3e42",
  border: "1px solid #555555",
  borderRadius: "4px",
  color: "#cccccc",
  fontSize: "0.875rem",
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: "#505050",
      borderColor: "#6e6e6e",
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const presets = style({
  padding: "1rem",
  borderBottom: "1px solid #3e3e42",
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
  background: "#3e3e42",
  border: "1px solid #555555",
  borderRadius: "4px",
  color: "#cccccc",
  fontSize: "0.8rem",
  cursor: "pointer",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      background: "#4e4e4e",
      borderColor: "#6e6e6e",
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
  background: "#2d2d30",
  border: "1px solid #3e3e42",
  borderRadius: "4px",
  cursor: "pointer",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      background: "#3e3e42",
      borderColor: "#555555",
    },
    "&.selected": {
      background: "#3e3e42",
      borderColor: "#4ec9b0",
      boxShadow: "0 0 0 1px #4ec9b0 inset",
    },
  },
});

export const selected = style({
  background: "#3e3e42",
  borderColor: "#4ec9b0",
  boxShadow: "0 0 0 1px #4ec9b0 inset",
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
  color: "#ffffff",
});

export const priority = style({
  padding: "0.125rem 0.5rem",
  borderRadius: "3px",
  fontSize: "0.75rem",
  fontWeight: 600,
  textTransform: "uppercase",
});

export const critical = style({
  background: "#d73a49",
  color: "#ffffff",
});

export const high = style({
  background: "#fb8500",
  color: "#ffffff",
});

export const medium = style({
  background: "#219ebc",
  color: "#ffffff",
});

export const low = style({
  background: "#595959",
  color: "#cccccc",
});

export const codeDetails = style({
  display: "flex",
  gap: "0.75rem",
  fontSize: "0.8rem",
  color: "#999999",
  paddingLeft: "1.5rem",
});

export const category = style({
  padding: "0.125rem 0.375rem",
  background: "#1e1e1e",
  borderRadius: "3px",
  color: "#cccccc",
});

export const count = style({
  color: "#999999",
});

export const sequence = style({
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace",
  color: "#d7ba7d",
  fontSize: "0.75rem",
});

export const middlePanel = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  background: "#1e1e1e",
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
  background: "#2d2d30",
  borderBottom: "1px solid #3e3e42",
});

export const currentCode = style({
  padding: "0.25rem 0.75rem",
  background: "#3e3e42",
  borderRadius: "4px",
  fontSize: "0.875rem",
  color: "#4ec9b0",
  animationName: pulse,
  animationDuration: "1s",
  animationIterationCount: "infinite",
});

export const terminal = style({
  flex: 1,
  padding: "1rem",
  background: "#1e1e1e",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace",
  fontSize: "14px",
  lineHeight: "1.5",
  overflowY: "auto",
  color: "#d4d4d4",
});

export const placeholder = style({
  color: "#666666",
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
  borderLeft: "1px solid #3e3e42",
  background: "#252526",
  "@media": {
    "screen and (max-width: 1400px)": {
      width: "300px",
    },
    "screen and (max-width: 1024px)": {
      width: "100%",
      border: "none",
      borderTop: "1px solid #3e3e42",
    },
  },
});

export const controls = style({
  padding: "1rem",
  borderBottom: "1px solid #3e3e42",
});

export const controlGroup = style({
  marginBottom: "1rem",
});

export const input = style({
  width: "100%",
  padding: "0.5rem",
  background: "#3e3e42",
  border: "1px solid #555555",
  borderRadius: "4px",
  color: "#cccccc",
  fontSize: "0.875rem",
  selectors: {
    "&:hover": {
      background: "#505050",
      borderColor: "#6e6e6e",
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
  background: "#4ec9b0",
  color: "#1e1e1e",
  selectors: {
    "&:hover": {
      background: "#5ed9c0",
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
  background: "#f48771",
  color: "#1e1e1e",
  selectors: {
    "&:hover": {
      background: "#ff9780",
      transform: "translateY(-1px)",
    },
  },
});

export const metrics = style({
  padding: "1rem",
  borderBottom: "1px solid #3e3e42",
});

export const metricItem = style({
  marginBottom: "1rem",
});

export const metricValue = style({
  display: "block",
  fontSize: "1.25rem",
  fontWeight: 600,
  color: "#ffffff",
  marginBottom: "0.5rem",
});

export const progressBar = style({
  height: "8px",
  background: "#3e3e42",
  borderRadius: "4px",
  overflow: "hidden",
});

export const progressFill = style({
  height: "100%",
  background: "#4ec9b0",
  transition: "width 0.3s ease",
});

export const errors = style({
  marginTop: "1rem",
  padding: "0.75rem",
  background: "rgba(244, 135, 113, 0.1)",
  border: "1px solid #f48771",
  borderRadius: "4px",
});

export const error = style({
  fontSize: "0.8rem",
  color: "#f48771",
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
  color: "#cccccc",
});

export const categoryCount = style({
  fontSize: "0.75rem",
  color: "#999999",
});
