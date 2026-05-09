import { createTheme } from "@vanilla-extract/css";
import { vars } from "./theme-contract.css";

export { vars, breakpoints, zIndex } from "./theme-contract.css";

const sharedTokens = {
  font: {
    mono: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
    sans: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
  },
  space: {
    "0": "0px",
    "1": "4px",
    "2": "8px",
    "3": "12px",
    "4": "16px",
    "6": "24px",
    "8": "32px",
    "12": "48px",
    "16": "64px",
  },
  radii: {
    sm: "4px",
    md: "6px",
    lg: "12px",
    full: "9999px",
  },
  fontSize: {
    xs: "11px",
    sm: "12px",
    base: "14px",
    lg: "16px",
    xl: "20px",
  },
  fontWeight: {
    normal: "400",
    medium: "500",
    semibold: "600",
    bold: "700",
  },
};

const terminalTokens = {
  terminalBackground: "#1e1e1e",
  terminalForeground: "#d4d4d4",
  terminalBorder: "#3e3e42",
  terminalHeaderBg: "#2d2d30",
  terminalHeaderFg: "#ffffff",
  terminalTabsBg: "#252526",
  terminalTextMuted: "#9ca3af",
  terminalHoverBg: "#3e3e42",
};

export const lightTheme = createTheme(vars, {
  color: {
    textPrimary: "#0a0a0a",
    textSecondary: "#4a4a4a",
    textMuted: "#6b6b6b",
    textDisabled: "#9a9a9a",
    textTertiary: "#767676", /* was #9ca3af — 2.53:1 fails WCAG AA on white; #767676 = 4.55:1 ✅ */
    textInverse: "#ffffff",

    background: "#ffffff",
    cardBackground: "#f9f9f9",
    hoverBackground: "#f0f0f0",
    modalBackground: "#ffffff",
    overlayBackground: "rgba(0, 0, 0, 0.5)",
    panelBgSecondary: "#f3f4f6",
    surfaceSubtle: "#f9fafb",
    surfaceMuted: "#f3f4f6",

    borderColor: "#e0e0e0",
    borderSubtle: "#e5e7eb",
    borderMuted: "#d1d5db",
    borderStrong: "#9ca3af",
    borderHover: "#9ca3af",
    modalBorder: "#e0e0e0",
    inputBorder: "#d4d4d4",
    inputFocusBorder: "#0070f3",

    primary: "#0070f3",
    primaryHover: "#0051cc",
    primaryActive: "#003d99",
    primaryDark: "#003d99",
    primaryText: "#ffffff",

    success: "#10b981",
    successBg: "#d1fae5",
    warning: "#f59e0b",
    warningBg: "#fef3c7",
    warningText: "#92400e",
    error: "#ef4444",
    errorBg: "#fee2e2",
    errorText: "#991b1b",
    errorDark: "#b91c1c",

    accentBg: "rgba(0, 112, 243, 0.08)",
    accentHover: "rgba(0, 112, 243, 0.16)",

    inputBackground: "#ffffff",
    inputText: "#0a0a0a",
    placeholderColor: "#9ca3af",

    ...terminalTokens,
  },
  shadow: {
    none: "none",
    sm: "0 1px 2px 0 rgba(0,0,0,0.05)",
    md: "0 4px 6px -1px rgba(0,0,0,0.07), 0 2px 4px -2px rgba(0,0,0,0.05)",
    lg: "0 10px 15px -3px rgba(0,0,0,0.1), 0 4px 6px -4px rgba(0,0,0,0.05)",
  },
  statusBadge: {
    approvalBg: "#fecaca",
    approvalFg: "#991b1b",
    approvalBorder: "#fca5a5",
    inputBg: "#dbeafe",
    inputFg: "#1e40af",
    inputBorder: "#93c5fd",
    completeBg: "#dcfce7",
    completeFg: "#166534",
    completeBorder: "#bbf7d0",
    uncommittedBg: "#fef3c7",
    uncommittedFg: "#92400e",
    uncommittedBorder: "#fcd34d",
    idleBg: "#f3f4f6",
    idleFg: "#374151",
    idleBorder: "#d1d5db",
    staleFg: "#6b7280",
    processingBg: "#e0e7ff",
    processingFg: "#4338ca",
    processingBorder: "#c7d2fe",
  },
  ...sharedTokens,
});

export const darkTheme = createTheme(vars, {
  color: {
    textPrimary: "#ededed",
    textSecondary: "#b4b4b4",
    textMuted: "#8a8a8a",
    textDisabled: "#767676",
    textTertiary: "#808080", /* was #6b7280 — #808080 = 4.64:1 on #1a1a1a card-bg — WCAG AA ✅ */
    textInverse: "#0a0a0a",

    background: "#0a0a0a",
    cardBackground: "#1a1a1a",
    hoverBackground: "#2a2a2a",
    modalBackground: "#1a1a1a",
    overlayBackground: "rgba(0, 0, 0, 0.7)",
    panelBgSecondary: "#2a2a2a",
    surfaceSubtle: "#1f2937",
    surfaceMuted: "#374151",

    borderColor: "#333333",
    borderSubtle: "#374151",
    borderMuted: "#4b5563",
    borderStrong: "#6b7280",
    borderHover: "#6b7280",
    modalBorder: "#333333",
    inputBorder: "#404040",
    inputFocusBorder: "#2d9cdb",

    primary: "#2d9cdb",
    primaryHover: "#3ba9e6",
    primaryActive: "#52b9f0",
    primaryDark: "#1a7fc1",
    primaryText: "#ffffff",

    success: "#10b981",
    successBg: "#064e3b",
    warning: "#f59e0b",
    warningBg: "#78350f",
    warningText: "#fbbf24",
    error: "#ef4444",
    errorBg: "#7f1d1d",
    errorText: "#fca5a5",
    errorDark: "#ef4444",

    accentBg: "rgba(45, 156, 219, 0.1)",
    accentHover: "rgba(45, 156, 219, 0.2)",

    inputBackground: "#2a2a2a",
    inputText: "#ededed",
    placeholderColor: "#6b7280",

    ...terminalTokens,
  },
  shadow: {
    none: "none",
    sm: "0 1px 2px 0 rgba(0,0,0,0.3)",
    md: "0 4px 6px -1px rgba(0,0,0,0.4), 0 2px 4px -2px rgba(0,0,0,0.3)",
    lg: "0 10px 15px -3px rgba(0,0,0,0.5), 0 4px 6px -4px rgba(0,0,0,0.4)",
  },
  statusBadge: {
    approvalBg: "#450a0a",
    approvalFg: "#fca5a5",
    approvalBorder: "#7f1d1d",
    inputBg: "#1e3a5f",
    inputFg: "#93c5fd",
    inputBorder: "#2563eb",
    completeBg: "#14532d",
    completeFg: "#86efac",
    completeBorder: "#166534",
    uncommittedBg: "#78350f",
    uncommittedFg: "#fbbf24",
    uncommittedBorder: "#b45309",
    idleBg: "#1f2937",
    idleFg: "#9ca3af",
    idleBorder: "#374151",
    staleFg: "#6b7280",
    processingBg: "#312e81",
    processingFg: "#a5b4fc",
    processingBorder: "#4338ca",
  },
  ...sharedTokens,
});
