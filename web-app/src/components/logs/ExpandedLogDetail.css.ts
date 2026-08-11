import { style } from "@vanilla-extract/css";

export const detailPanel = style({
  padding: "8px 12px 12px",
  borderBottom: "2px solid rgba(255,255,255,0.1)",
  backgroundColor: "rgba(0,0,0,0.2)",
});

export const detailHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  marginBottom: 6,
});

export const detailLabel = style({
  fontSize: 10,
  fontWeight: 600,
  letterSpacing: "0.08em",
  textTransform: "uppercase",
  color: "rgba(156,163,175,0.7)",
});

export const copyButton = style({
  fontSize: 11,
  padding: "2px 10px",
  minHeight: 44, // mobile touch target
  borderRadius: 4,
  border: "1px solid rgba(255,255,255,0.2)",
  background: "transparent",
  color: "inherit",
  cursor: "pointer",
  touchAction: "manipulation",
});

export const jsonBlock = style({
  margin: 0,
  padding: "8px",
  fontSize: 12,
  lineHeight: 1.5,
  fontFamily: "monospace",
  whiteSpace: "pre-wrap", // wrap long JSON lines in detail view
  wordBreak: "break-word",
  overflowX: "auto",
  userSelect: "text", // always allow text selection in detail view
  backgroundColor: "rgba(0,0,0,0.15)",
  borderRadius: 4,
});

export const rawBlock = style({
  margin: 0,
  padding: "8px",
  fontSize: 12,
  lineHeight: 1.5,
  fontFamily: "monospace",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  overflowX: "auto",
  userSelect: "text",
  backgroundColor: "rgba(0,0,0,0.15)",
  borderRadius: 4,
});
