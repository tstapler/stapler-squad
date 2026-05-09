import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  padding: "16px",
  overflowY: "auto",
  background: vars.color.background,
  color: vars.color.textPrimary,
});

export const loading = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: vars.color.textSecondary,
});

export const error = style({
  display: "flex",
  alignItems: "center",
  gap: "12px",
  padding: "16px",
  background: "rgba(255, 100, 100, 0.1)",
  border: "1px solid rgba(255, 100, 100, 0.3)",
  borderRadius: "8px",
  color: "#ff6b6b",
});

export const errorIcon = style({
  fontSize: "20px",
});

export const retryButton = style({
  marginLeft: "auto",
  padding: "6px 12px",
  background: "transparent",
  border: "1px solid currentColor",
  borderRadius: "4px",
  color: "inherit",
  cursor: "pointer",
  fontSize: "12px",
  selectors: {
    "&:hover": {
      background: "rgba(255, 100, 100, 0.2)",
    },
  },
});

export const empty = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: vars.color.textSecondary,
  textAlign: "center",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "16px",
  paddingBottom: "12px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const vcsType = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
});

export const vcsIcon = style({
  fontSize: "20px",
});

export const vcsName = style({
  fontSize: "18px",
  fontWeight: 600,
});

export const refreshButton = style({
  padding: "6px 10px",
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "14px",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
});

export const githubSection = style({
  marginBottom: "16px",
  padding: "10px 12px",
  background: vars.color.cardBackground,
  borderRadius: "8px",
  display: "flex",
  flexDirection: "column",
  gap: "6px",
});

export const githubRow = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  flexWrap: "wrap",
  fontSize: "13px",
});

export const githubIcon = style({
  color: vars.color.textSecondary,
  fontSize: "14px",
  flexShrink: 0,
});

export const githubRepo = style({
  fontFamily: "monospace",
  fontWeight: 600,
  color: vars.color.textPrimary,
  textDecoration: "none",
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const githubPrLink = style({
  fontFamily: "monospace",
  color: "#3b82f6",
  textDecoration: "none",
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const githubPrState = style({
  color: vars.color.textSecondary,
  fontFamily: "sans-serif",
  fontSize: "12px",
});

export const githubDraft = style({
  color: "#888",
  fontFamily: "sans-serif",
  fontSize: "12px",
});

export const githubReviews = style({
  marginLeft: "auto",
  display: "flex",
  gap: "8px",
  fontSize: "12px",
});

export const githubApproved = style({
  color: "#7ee787",
  fontWeight: 500,
});

export const githubChangesReq = style({
  color: "#f97583",
  fontWeight: 500,
});

export const githubCi = style({
  fontSize: "12px",
  fontFamily: "monospace",
});

export const branchInfo = style({
  marginBottom: "16px",
  padding: "12px",
  background: vars.color.cardBackground,
  borderRadius: "8px",
});

export const branchRow = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  flexWrap: "wrap",
});

export const branchIcon = style({
  color: "#7ee787",
  fontWeight: "bold",
});

export const branchName = style({
  fontFamily: "monospace",
  fontSize: "14px",
  fontWeight: 600,
  color: "#7ee787",
});

export const commitHash = style({
  fontFamily: "monospace",
  fontSize: "12px",
  color: vars.color.textSecondary,
  background: vars.color.background,
  padding: "2px 8px",
  borderRadius: "4px",
});

export const commitMessage = style({
  marginTop: "8px",
  fontSize: "13px",
  color: vars.color.textSecondary,
  lineHeight: 1.4,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const syncStatus = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  marginBottom: "16px",
  padding: "8px 12px",
  background: vars.color.cardBackground,
  borderRadius: "6px",
  fontSize: "13px",
});

export const syncIcon = style({
  opacity: 0.7,
});

export const upstream = style({
  fontFamily: "monospace",
  color: vars.color.textSecondary,
});

export const ahead = style({
  color: "#7ee787",
  fontWeight: 500,
});

export const behind = style({
  color: "#f97583",
  fontWeight: 500,
});

export const workdirStatus = style({
  marginBottom: "16px",
});

export const cleanStatus = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "12px",
  background: "rgba(126, 231, 135, 0.1)",
  border: "1px solid rgba(126, 231, 135, 0.3)",
  borderRadius: "8px",
  color: "#7ee787",
});

export const cleanIcon = style({
  fontWeight: "bold",
});

export const dirtyStatus = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "8px",
});

const sharedBadge = style({
  padding: "4px 10px",
  borderRadius: "12px",
  fontSize: "12px",
  fontWeight: 500,
});

export const conflictBadge = style([
  sharedBadge,
  {
    background: "rgba(249, 117, 131, 0.2)",
    color: "#f97583",
    border: "1px solid rgba(249, 117, 131, 0.4)",
  },
]);

export const stagedBadge = style([
  sharedBadge,
  {
    background: "rgba(126, 231, 135, 0.2)",
    color: "#7ee787",
    border: "1px solid rgba(126, 231, 135, 0.4)",
  },
]);

export const unstagedBadge = style([
  sharedBadge,
  {
    background: "rgba(247, 203, 104, 0.2)",
    color: "#f7cb68",
    border: "1px solid rgba(247, 203, 104, 0.4)",
  },
]);

export const untrackedBadge = style([
  sharedBadge,
  {
    background: "rgba(136, 136, 136, 0.2)",
    color: "#888",
    border: "1px solid rgba(136, 136, 136, 0.4)",
  },
]);

export const fileLists = style({
  display: "flex",
  flexDirection: "column",
  gap: "16px",
});

export const fileSection = style({
  background: vars.color.cardBackground,
  borderRadius: "8px",
  overflow: "hidden",
});

export const fileSectionTitle = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  margin: 0,
  padding: "10px 12px",
  fontSize: "13px",
  fontWeight: 600,
  background: vars.color.hoverBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const sectionIcon = style({
  fontSize: "14px",
});

export const fileList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
});

export const fileItem = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "8px 12px",
  fontFamily: "monospace",
  fontSize: "13px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
  },
});

export const fileStatus = style({
  width: "16px",
  textAlign: "center",
  fontWeight: "bold",
});

export const filePath = style({
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const modified = style({});
globalStyle(`${modified} .${fileStatus}`, { color: "#f7cb68" });
globalStyle(`${modified} .${filePath}`, { color: "#f7cb68" });

export const added = style({});
globalStyle(`${added} .${fileStatus}`, { color: "#7ee787" });
globalStyle(`${added} .${filePath}`, { color: "#7ee787" });

export const deleted = style({});
globalStyle(`${deleted} .${fileStatus}`, { color: "#f97583" });
globalStyle(`${deleted} .${filePath}`, { color: "#f97583" });

export const renamed = style({});
globalStyle(`${renamed} .${fileStatus}`, { color: "#d2a8ff" });
globalStyle(`${renamed} .${filePath}`, { color: "#d2a8ff" });

export const untracked = style({});
globalStyle(`${untracked} .${fileStatus}`, { color: "#888" });
globalStyle(`${untracked} .${filePath}`, { color: "#888" });

export const conflict = style({});
globalStyle(`${conflict} .${fileStatus}`, { color: "#f97583", fontWeight: "bold" });
globalStyle(`${conflict} .${filePath}`, { color: "#f97583", fontWeight: "bold" });

export const filePathClickable = style({
  cursor: "pointer",
  textDecoration: "underline",
  textDecorationStyle: "dotted",
  selectors: {
    "&:hover": {
      textDecorationStyle: "solid",
      opacity: 0.85,
    },
  },
});
