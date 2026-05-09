import { style, keyframes, globalStyle, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const cardFadeSlideIn = keyframes({
  from: { opacity: 0, transform: "translateY(8px)" },
  to: { opacity: 1, transform: "translateY(0)" },
});

const attentionPulse = keyframes({
  "0%, 100%": { boxShadow: "0 0 0 0 rgba(239, 68, 68, 0)" },
  "50%": { boxShadow: "0 0 0 4px rgba(239, 68, 68, 0.3)" },
});

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const slideUp = keyframes({
  from: { transform: "translateY(20px)", opacity: 0 },
  to: { transform: "translateY(0)", opacity: 1 },
});

export const card = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["4"],
  marginBottom: vars.space["3"],
  cursor: "pointer",
  transition: "border-color 0.2s ease, box-shadow 0.2s ease, opacity 0.3s ease, transform 0.3s ease, max-height 0.3s ease",
  position: "relative",
  WebkitTapHighlightColor: "transparent",
  animationName: cardFadeSlideIn,
  animationDuration: "0.35s",
  animationTimingFunction: "ease",
  animationFillMode: "both",
  // Runtime: animation-delay set via --card-index inline style
  // Cap stagger at 5 cards (300ms max) to prevent long lists from having excessive delays
  animationDelay: "calc(min(var(--card-index, 0), 5) * 60ms)",
  selectors: {
    "&:hover": {
      borderColor: vars.color.borderHover,
      boxShadow: "0 2px 8px rgba(0, 0, 0, 0.1)",
    },
  },
});

export const cardDeleting = style({
  opacity: 0.4,
  transform: "scale(0.97)",
  borderColor: vars.color.error,
  boxShadow: "0 0 0 2px rgba(239, 68, 68, 0.25)",
  pointerEvents: "none",
});

export const cardSelectMode = style({
  paddingLeft: "52px",
});

export const cardSelected = style({
  borderColor: vars.color.primary,
  background: "rgba(0, 112, 243, 0.05)", // design-token: pending (no alpha-variant token)
});

export const cardExternal = style({
  borderLeft: "4px solid #6366f1", // TODO: add vars.color.externalIndicator token
  backgroundImage: `linear-gradient(to right, rgba(99, 102, 241, 0.05), ${vars.color.cardBackground})`,
});

export const checkbox = style({
  position: "absolute",
  left: vars.space["4"],
  top: "50%",
  transform: "translateY(-50%)",
});

globalStyle(`${checkbox} input[type='checkbox']`, { width: "20px", height: "20px", cursor: "pointer" });

export const header = style({
  marginBottom: vars.space["3"],
});

export const titleRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: vars.space["2"],
});

export const title = style({
  margin: 0,
  fontSize: "1.125rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  "@media": {
    "(max-width: 768px)": { fontSize: "1rem" },
  },
});

export const inlineTitleInput = style({
  margin: 0,
  fontSize: "1.125rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.primary}`,
  borderRadius: vars.radii.sm,
  padding: "2px 6px",
  outline: "none",
  width: "100%",
  "@media": {
    "(max-width: 768px)": { fontSize: "1rem" },
  },
});

export const badges = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const externalBadge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} 10px`,
  background: "#6366f1", // TODO: add vars.color.externalIndicator token
  color: "white",
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  border: "1px solid #4f46e5", // TODO: add vars.color.externalIndicator token
});

export const muxIndicator = style({
  fontSize: "0.625rem",
  background: "rgba(255, 255, 255, 0.3)",
  padding: `2px ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
  marginLeft: vars.space["1"],
});

export const reviewInfo = style({
  marginTop: vars.space["2"],
  padding: vars.space["2"],
  background: vars.color.warningBg,
  borderLeft: `3px solid ${vars.color.warning}`,
  borderRadius: vars.radii.sm,
  display: "flex",
  flexDirection: "column",
  gap: "6px",
});

export const reviewContext = style({
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
  fontStyle: "italic",
});

export const status = style({
  padding: `${vars.space["1"]} 12px`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  textTransform: "uppercase",
});

export const statusRunning = style({
  background: "#dcfce7",
  color: "#14532d", // bumped from #166534 (~4.6:1) for better contrast safety margin
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#166534",
      color: "#dcfce7",
    },
  },
});

export const statusReady = style({
  background: "#dbeafe",
  color: "#1e40af",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#1e40af",
      color: "#dbeafe",
    },
  },
});

export const statusPaused = style({
  background: "#fef3c7",
  color: "#78350f", // bumped from #92400e (~4.5:1) for better contrast safety margin
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#92400e",
      color: "#fef3c7",
    },
  },
});

export const statusLoading = style({
  background: "#e0e7ff",
  color: "#4338ca",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#3730a3",
      color: "#e0e7ff",
    },
  },
});

export const statusNeedsApproval = style({
  background: "#fecaca",
  color: "#991b1b",
  animationName: attentionPulse,
  animationDuration: "2s",
  animationTimingFunction: "ease-in-out",
  animationIterationCount: "infinite",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#991b1b",
      color: "#fecaca",
    },
    "(prefers-reduced-motion: reduce)": {
      animationName: "none",
    },
  },
});

export const statusUnknown = style({
  background: "#f3f4f6",
  color: "#374151",
});

export const category = style({
  display: "inline-block",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.surfaceSubtle,
  color: vars.color.textSecondary,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
});

export const tagsContainer = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  marginTop: vars.space["2"],
  flexWrap: "wrap",
});

export const tags = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "6px",
});

export const tag = style({
  display: "inline-block",
  padding: `${vars.space["1"]} 10px`,
  fontSize: "0.6875rem",
  fontWeight: 500,
  background: "#1e40af",
  color: "#dbeafe",
  borderRadius: vars.radii.full,
  transition: "background 0.2s ease",
  selectors: {
    "&:hover": { background: "#1e3a8a" },
  },
  "@media": {
    "(prefers-color-scheme: dark)": {
      color: "#dbeafe",
    },
  },
});

export const editTagsButton = style({
  padding: `${vars.space["1"]} 12px`,
  fontSize: "0.6875rem",
  fontWeight: 600,
  background: "transparent",
  color: "#1e40af",
  border: "1px solid #1e40af",
  borderRadius: vars.radii.full,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": { background: "#1e40af", color: "#dbeafe" },
  },
  "@media": {
    "(prefers-color-scheme: dark)": {
      color: "#dbeafe",
      borderColor: "#1e40af",
    },
  },
});

export const body = style({
  marginBottom: vars.space["3"],
});

export const info = style({
  display: "flex",
  flexDirection: "column",
  gap: "6px",
});

export const infoRow = style({
  display: "flex",
  gap: vars.space["2"],
  fontSize: "0.875rem",
});

export const label = style({
  color: vars.color.textSecondary,
  fontWeight: 500,
  minWidth: "100px",
});

export const value = style({
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const githubLink = style({
  color: "#0969da",
  textDecoration: "none",
  fontWeight: 500,
  transition: "color 0.2s ease",
  selectors: {
    "&:hover": { color: "#0550ae", textDecoration: "underline" },
  },
  "@media": {
    "(prefers-color-scheme: dark)": {
      color: "#58a6ff",
      selectors: {
        "&:hover": { color: "#79c0ff" },
      },
    },
  },
});

export const diffStats = style({
  display: "flex",
  gap: vars.space["3"],
  marginTop: vars.space["2"],
  fontFamily: "monospace",
  fontSize: "0.875rem",
});

export const diffAdded = style({
  color: vars.color.success, // was #16a34a
  fontWeight: 600,
});

export const diffRemoved = style({
  color: vars.color.error, // was #dc2626
  fontWeight: 600,
});

export const lastActivityRow = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  marginTop: "6px",
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const lastActivityLabel = style({
  fontWeight: 600,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontSize: vars.fontSize.xs,
});

export const lastActivityTime = style({
  color: vars.color.textSecondary,
});

export const footer = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  paddingTop: vars.space["3"],
  borderTop: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "(max-width: 768px)": {
      flexDirection: "column",
      alignItems: "stretch",
      gap: vars.space["2"],
    },
  },
});

export const timestamps = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const timestamp = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const desktopActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  "@media": {
    "(max-width: 768px)": {
      display: "none",
    },
  },
});

export const overflowContainer = style({
  position: "relative",
});

export const overflowButton = style({
  padding: `6px 10px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "1.1rem",
  fontWeight: 700,
  cursor: "pointer",
  letterSpacing: "2px",
  lineHeight: 1,
  minHeight: "44px",
  transition: "background 0.2s ease",
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const overflowMenu = style({
  position: "absolute",
  right: 0,
  top: "calc(100% + 4px)",
  minWidth: "180px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  boxShadow: "0 8px 24px rgba(0, 0, 0, 0.15)",
  zIndex: 200,
  padding: "4px",
  display: "flex",
  flexDirection: "column",
});

export const overflowMenuItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `8px 12px`,
  border: "none",
  borderRadius: vars.radii.md,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  textAlign: "left",
  cursor: "pointer",
  width: "100%",
  transition: "background 0.15s ease",
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
    "&:disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
});

export const overflowMenuItemDanger = style({
  color: vars.color.errorText, // was #991b1b
  selectors: {
    "&:hover": { background: vars.color.errorBg }, // was #fee2e2
  },
});

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
  "@media": {
    "(max-width: 768px)": {
      display: "none",
      width: "100%",
    },
  },
});

export const actionsOpen = style({
  "@media": {
    "(max-width: 768px)": {
      display: "grid",
      gridTemplateColumns: "1fr 1fr",
      gap: vars.space["2"],
    },
  },
});

export const actionsToggle = style({
  display: "none",
  "@media": {
    "(max-width: 768px)": {
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
      width: "100%",
      minHeight: "44px",
      padding: `10px ${vars.space["4"]}`,
      border: `1px solid ${vars.color.borderColor}`,
      borderRadius: vars.radii.md,
      background: vars.color.cardBackground,
      color: vars.color.textPrimary,
      fontSize: "0.875rem",
      fontWeight: 500,
      cursor: "pointer",
      transition: "background 0.2s ease",
      selectors: {
        "&:hover": { background: vars.color.hoverBackground },
      },
    },
  },
});

export const actionButton = style({
  padding: `6px ${vars.space["4"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
  "@media": {
    "(max-width: 768px)": {
      minHeight: "44px",
      padding: `10px ${vars.space["2"]}`,
      justifyContent: "center",
      fontSize: "0.8125rem",
      textAlign: "center",
    },
  },
});

export const deleteButton = style({
  background: vars.color.errorBg, // was #fee2e2
  color: vars.color.errorText, // was #991b1b
  borderColor: "#fca5a5", // design-token: pending (no light-error-border token)
  selectors: {
    "&:hover:not(:disabled)": { background: "#fecaca", borderColor: "#f87171" },
    "&:disabled": {
      background: "#fca5a5",
      color: "#7f1d1d",
      borderColor: vars.color.error, // was #ef4444
      opacity: 0.8,
      cursor: "not-allowed",
    },
  },
  "@media": {
    "(max-width: 768px)": {
      gridColumn: "1 / -1",
    },
    "(prefers-color-scheme: dark)": {
      background: "#7f1d1d",
      color: "#fecaca",
      borderColor: "#991b1b",
      selectors: {
        "&:hover:not(:disabled)": { background: "#991b1b", borderColor: "#b91c1c" },
      },
    },
  },
});

export const restartButton = style({
  background: "#fef3c7",
  color: "#92400e",
  borderColor: "#fde68a",
  selectors: {
    "&:hover": { background: "#fcd34d", borderColor: "#fbbf24" },
  },
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#713f12",
      color: "#fef3c7",
      borderColor: "#92400e",
      selectors: {
        "&:hover": { background: "#92400e", borderColor: "#a16207" },
      },
    },
  },
});

export const renameDialog = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: "rgba(0, 0, 0, 0.5)",
  display: "flex",
  justifyContent: "center",
  alignItems: "center",
  zIndex: 1000,
  animationName: fadeIn,
  animationDuration: "0.2s",
  animationTimingFunction: "ease",
});

export const confirmDialog = style([renameDialog]);

export const dialogContent = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["6"],
  maxWidth: "400px",
  width: "90%",
  boxShadow: "0 10px 40px rgba(0, 0, 0, 0.2)",
  animationName: slideUp,
  animationDuration: "0.3s",
  animationTimingFunction: "ease",
});

globalStyle(`${dialogContent} h3`, { margin: `0 0 ${vars.space["4"]} 0`, color: vars.color.textPrimary, fontSize: "1.25rem" });
globalStyle(`${dialogContent} p`, { margin: `${vars.space["3"]} 0`, color: vars.color.textSecondary, fontSize: "0.875rem" });

export const warningText = style({
  color: `${vars.color.error} !important` as string,
  fontWeight: 500,
  fontSize: `0.8125rem !important` as string,
  "@media": {
    "(prefers-color-scheme: dark)": {
      color: `#fca5a5 !important` as string,
    },
  },
});

export const renameInput = style({
  width: "100%",
  padding: `10px 12px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  marginBottom: vars.space["2"],
  transition: "border-color 0.2s ease",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: `0 0 0 3px rgba(0, 112, 243, 0.1)`,
    },
  },
});

// renameLabel used for fork dialog
export const renameLabel = style({
  display: "block",
  fontSize: "0.875rem",
  color: vars.color.textSecondary,
  marginBottom: vars.space["1"],
});

export const errorMessage = style({
  display: "block",
  color: vars.color.error,
  fontSize: vars.fontSize.sm,
  margin: `${vars.space["2"]} 0`,
});

export const dialogActions = style({
  display: "flex",
  gap: vars.space["3"],
  marginTop: "20px",
  justifyContent: "flex-end",
});

export const submitButton = style({
  padding: `${vars.space["2"]} 20px`,
  borderRadius: vars.radii.lg,
  fontSize: "0.875rem",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  border: `1px solid ${vars.color.primary}`,
  background: vars.color.primary,
  color: "white",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.primaryHover,
      borderColor: vars.color.primaryHover,
    },
    "&:disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
});

export const cancelButton = style({
  padding: `${vars.space["2"]} 20px`,
  borderRadius: vars.radii.lg,
  fontSize: "0.875rem",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
});

export const dangerButton = style({
  padding: `${vars.space["2"]} 20px`,
  borderRadius: vars.radii.lg,
  fontSize: "0.875rem",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  background: "#ef4444",
  color: "white",
  border: "1px solid #ef4444",
  selectors: {
    "&:hover:not(:disabled)": { background: "#dc2626", borderColor: "#dc2626" },
    "&:disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#991b1b",
      color: "#fecaca",
      borderColor: "#b91c1c",
      selectors: {
        "&:hover:not(:disabled)": { background: "#b91c1c", borderColor: "#dc2626" },
      },
    },
  },
});

// Fork dialog specific
export const forkEmptyMessage = style({
  color: vars.color.textMuted,
  fontSize: "0.875rem",
  fontStyle: "italic",
  margin: `${vars.space["2"]} 0`,
});

export const forkCheckpointList = style({
  listStyle: "none",
  padding: 0,
  margin: `${vars.space["2"]} 0`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const forkCheckpointItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const forkCheckpointLabel = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  cursor: "pointer",
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
});

export const forkGitSha = style({
  fontFamily: "monospace",
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceSubtle,
  padding: `1px ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
});

// ── Terminal snapshot preview (from upstream) ────────────────────────────────

/** Container for the terminal snapshot preview section */
export const snapshotSection = style({
  margin: "8px 0 0",
  borderRadius: 6,
  overflow: "hidden",
  border: `1px solid ${vars.color.borderColor}`,
});

/** Toggle button row */
export const snapshotToggle = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "4px 10px",
  background: vars.color.cardBackground,
  border: "none",
  cursor: "pointer",
  width: "100%",
  fontSize: "0.7rem",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  color: vars.color.textSecondary,
  userSelect: "none",
  selectors: {
    "&:hover": { opacity: 0.85 },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "-2px",
    },
  },
});

export const snapshotToggleIcon = style({
  fontSize: "0.65rem",
  lineHeight: 1,
});

/** Fixed-height preview pane */
export const snapshotPane = style({
  height: 120,
  overflowY: "hidden",
  padding: "6px 10px",
  fontFamily: '"Menlo", "Monaco", "Courier New", monospace',
  fontSize: "0.72rem",
  lineHeight: 1.5,
  background: "#1e1e1e",
  color: "#d4d4d4",
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
});

/** Placeholder shown when terminal was cleared */
export const snapshotEmpty = style({
  height: 120,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  background: vars.color.cardBackground,
  fontStyle: "italic",
});

/** Loading skeleton */
export const snapshotLoading = style({
  height: 120,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  background: vars.color.cardBackground,
});

/** Error state */
export const snapshotError = styleVariants({
  base: {
    height: 120,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    fontSize: "0.72rem",
    color: vars.color.error,
    background: vars.color.cardBackground,
    padding: "0 10px",
    textAlign: "center",
  },
});
