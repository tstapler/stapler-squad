/**
 * Terminal configuration system with localStorage persistence
 *
 * Provides user-configurable settings for terminal behavior, appearance,
 * and performance. All settings persist across sessions.
 */

export interface TerminalConfig {
  /**
   * Number of lines to keep in terminal scrollback buffer
   *
   * NOTE: Since we use tmux for session management, tmux handles scrollback.
   * Setting this to 0 disables xterm.js scrollback to avoid duplicate buffering.
   * If you need to scroll, use tmux's scroll mode (prefix + [) instead.
   */
  scrollbackLines: number;

  /**
   * Terminal theme (light or dark mode)
   */
  theme: "light" | "dark";

  /**
   * Font size in pixels
   */
  fontSize: number;

  /**
   * Font family for terminal
   */
  fontFamily: string;

  /**
   * Enable terminal bell (audio feedback)
   */
  enableBell: boolean;

  /**
   * Cursor style
   */
  cursorStyle: "block" | "underline" | "bar";

  /**
   * Enable cursor blinking
   */
  cursorBlink: boolean;

  /**
   * Mouse event tracking mode
   */
  mouseTracking: "none" | "x10" | "vt200" | "drag" | "any";
}

/**
 * Default terminal configuration
 * These values provide good performance and UX for most users
 */
export const DEFAULT_TERMINAL_CONFIG: TerminalConfig = {
  scrollbackLines: 0, // tmux handles scrollback, no need for xterm.js buffer
  theme: "dark",
  fontSize: 14,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
  enableBell: false,
  cursorStyle: "block",
  cursorBlink: true,
  mouseTracking: "none",
};

/**
 * Configuration presets for different use cases
 */
export const TERMINAL_CONFIG_PRESETS: Record<string, Partial<TerminalConfig>> = {
  "default": DEFAULT_TERMINAL_CONFIG,
  "high-performance": {
    scrollbackLines: 0,
    cursorBlink: false, // Reduce repaints
  },
  "accessibility": {
    fontSize: 16,
    cursorStyle: "block",
    cursorBlink: false,
    theme: "light",
  },
};

/**
 * Named dark terminal theme for xterm.js
 */
export const darkTerminalTheme: import("@xterm/xterm").ITheme = {
  background: '#1e1e1e',
  foreground: '#cccccc',
  cursor: '#cccccc',
  cursorAccent: '#1e1e1e',
  selectionBackground: 'rgba(255, 255, 255, 0.3)',
  black: '#000000',
  red: '#cd3131',
  green: '#0dbc79',
  yellow: '#e5e510',
  blue: '#2472c8',
  magenta: '#bc3fbc',
  cyan: '#11a8cd',
  white: '#e5e5e5',
  brightBlack: '#666666',
  brightRed: '#f14c4c',
  brightGreen: '#23d18b',
  brightYellow: '#f5f543',
  brightBlue: '#3b8eea',
  brightMagenta: '#d670d6',
  brightCyan: '#29b8db',
  brightWhite: '#ffffff',
};

/**
 * Named light terminal theme for xterm.js
 */
export const lightTerminalTheme: import("@xterm/xterm").ITheme = {
  background: '#ffffff',
  foreground: '#1a1a1a',
  cursor: '#333333',
  selectionBackground: 'rgba(0, 112, 243, 0.3)',
  black: '#000000',
  red: '#c0392b',
  green: '#27ae60',
  yellow: '#e67e22',
  blue: '#2980b9',
  magenta: '#8e44ad',
  cyan: '#16a085',
  white: '#bdc3c7',
  brightBlack: '#7f8c8d',
  brightRed: '#e74c3c',
  brightGreen: '#2ecc71',
  brightYellow: '#f39c12',
  brightBlue: '#3498db',
  brightMagenta: '#9b59b6',
  brightCyan: '#1abc9c',
  brightWhite: '#ecf0f1',
};

const STORAGE_KEY = "stapler-squad-terminal-config";

/**
 * Load terminal configuration from localStorage
 * Falls back to default config if not found or invalid
 */
export function loadTerminalConfig(): TerminalConfig {
  if (typeof window === "undefined") {
    return DEFAULT_TERMINAL_CONFIG;
  }

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) {
      return DEFAULT_TERMINAL_CONFIG;
    }

    const config = JSON.parse(stored) as Partial<TerminalConfig>;

    // Validate and merge with defaults
    return {
      ...DEFAULT_TERMINAL_CONFIG,
      ...config,
      // Ensure scrollbackLines is within reasonable bounds (0 = disabled, tmux handles it)
      scrollbackLines: Math.max(
        0,
        Math.min(config.scrollbackLines ?? DEFAULT_TERMINAL_CONFIG.scrollbackLines, 100000)
      ),
      // Ensure fontSize is within reasonable bounds
      fontSize: Math.max(8, Math.min(config.fontSize ?? DEFAULT_TERMINAL_CONFIG.fontSize, 32)),
    };
  } catch (err) {
    console.error("[terminalConfig] Failed to load config:", err);
    return DEFAULT_TERMINAL_CONFIG;
  }
}

/**
 * Save terminal configuration to localStorage
 */
export function saveTerminalConfig(config: Partial<TerminalConfig>): void {
  if (typeof window === "undefined") {
    return;
  }

  try {
    const current = loadTerminalConfig();
    const updated = { ...current, ...config };

    localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));

    // Dispatch custom event for other components to react
    window.dispatchEvent(
      new CustomEvent("terminal-config-changed", { detail: updated })
    );
  } catch (err) {
    console.error("[terminalConfig] Failed to save config:", err);
  }
}

/**
 * Reset configuration to defaults
 */
export function resetTerminalConfig(): void {
  if (typeof window === "undefined") {
    return;
  }

  try {
    localStorage.removeItem(STORAGE_KEY);
    window.dispatchEvent(
      new CustomEvent("terminal-config-changed", { detail: DEFAULT_TERMINAL_CONFIG })
    );
  } catch (err) {
    console.error("[terminalConfig] Failed to reset config:", err);
  }
}

/**
 * Apply a configuration preset
 */
export function applyConfigPreset(presetName: keyof typeof TERMINAL_CONFIG_PRESETS): void {
  const preset = TERMINAL_CONFIG_PRESETS[presetName];
  if (preset) {
    saveTerminalConfig(preset);
  }
}

/**
 * Get memory usage estimate for a given scrollback size
 * @param scrollbackLines Number of lines in scrollback buffer
 * @returns Estimated memory usage in MB
 */
export function estimateMemoryUsage(scrollbackLines: number): number {
  // Rough estimate: ~5KB per line (including xterm.js overhead)
  const bytesPerLine = 5 * 1024;
  return (scrollbackLines * bytesPerLine) / (1024 * 1024);
}

/**
 * React hook for terminal configuration with live updates
 */
export function useTerminalConfig(): [
  TerminalConfig,
  (config: Partial<TerminalConfig>) => void
] {
  if (typeof window === "undefined") {
    return [DEFAULT_TERMINAL_CONFIG, () => {}];
  }

  const [config, setConfig] = React.useState<TerminalConfig>(loadTerminalConfig);

  React.useEffect(() => {
    const handleConfigChange = (event: Event) => {
      const customEvent = event as CustomEvent<TerminalConfig>;
      setConfig(customEvent.detail);
    };

    window.addEventListener("terminal-config-changed", handleConfigChange);

    return () => {
      window.removeEventListener("terminal-config-changed", handleConfigChange);
    };
  }, []);

  const updateConfig = React.useCallback((partial: Partial<TerminalConfig>) => {
    saveTerminalConfig(partial);
    setConfig(loadTerminalConfig());
  }, []);

  return [config, updateConfig];
}

// Re-export React for the hook
import * as React from "react";
