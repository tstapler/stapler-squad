import { style } from "@vanilla-extract/css";

export const backdrop = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: "rgba(0, 0, 0, 0.5)",
  display: "flex",
  alignItems: "flex-end",
  justifyContent: "center",
  zIndex: 1000,
  opacity: 0,
  visibility: "hidden",
  transition: "opacity 0.2s ease, visibility 0.2s ease",
});

export const open = style({
  opacity: 1,
  visibility: "visible",
});

export const keyboard = style({
  background: "#252526",
  borderRadius: "12px 12px 0 0",
  width: "100%",
  maxWidth: "600px",
  maxHeight: "80vh",
  overflow: "hidden",
  boxShadow: "0 -4px 20px rgba(0, 0, 0, 0.3)",
  transform: "translateY(100%)",
  transition: "transform 0.3s ease",
  selectors: {
    [`${open} &`]: {
      transform: "translateY(0)",
    },
  },
});

export const keyboardOpen = style({
  transform: "translateY(0)",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "1rem",
  borderBottom: "1px solid #3e3e42",
});

export const title = style({
  color: "#d4d4d4",
  fontSize: "1.25rem",
  fontWeight: 600,
  margin: 0,
});

export const closeButton = style({
  background: "transparent",
  border: "none",
  color: "#cccccc",
  fontSize: "1.5rem",
  cursor: "pointer",
  width: "32px",
  height: "32px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: "50%",
  selectors: {
    "&:hover": {
      background: "#3e3e42",
      color: "#ffffff",
    },
  },
});

export const keysContainer = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
  padding: "1rem",
});

export const keyRow = style({
  display: "flex",
  gap: "0.3rem",
  flexWrap: "wrap",
});

export const key = style({
  flex: 1,
  minWidth: "32px",
  height: "40px",
  background: "#3c3c3c",
  border: "1px solid #555",
  borderRadius: "6px",
  color: "#d4d4d4",
  fontFamily: "inherit",
  fontSize: "0.9rem",
  fontWeight: 500,
  cursor: "pointer",
  userSelect: "none",
  WebkitUserSelect: "none",
  touchAction: "manipulation",
  textAlign: "center",
  transition: "background 0.1s, transform 0.1s",
  selectors: {
    "&:active": {
      background: "#555",
      transform: "translateY(2px)",
    },
  },
});

export const tabKey = style({
  flex: "0 0 60px",
});

export const capsLockKey = style({
  flex: "0 0 70px",
});

export const specialKey = style({
  flex: "0 0 50px",
});
