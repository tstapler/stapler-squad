/**
 * Terminal configuration system with localStorage persistence
 *
 * Provides user-configurable settings for terminal behavior, appearance,
 * and performance. All settings persist across sessions.
 */

export interface TerminalConfig {
  /**
   * Number of lines to keep in terminal scrollback buffer
   * Higher values use more memory but preserve more history
   *
   * Recommended values:
   * - 1,000: Low memory (~5MB), basic usage
   * - 10,000: Default (~50MB), most sessions
   * - 50,000: High memory (~250MB), heavy debugging
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
   * Enable WebGL renderer for better performance
   * Falls back to canvas if WebGL unavailable
   */
  enableWebGL: boolean;

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
}

/**
 * Default terminal configuration
 * These values provide good performance and UX for most users
 */
export const DEFAULT_TERMINAL_CONFIG: TerminalConfig = {
  scrollbackLines: 10000,
  theme: "dark",
  fontSize: 14,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
  enableWebGL: true,
  enableBell: false,
  cursorStyle: "block",
  cursorBlink: true,
};

/**
 * Configuration presets for different use cases
 */
export const TERMINAL_CONFIG_PRESETS: Record<string, Partial<TerminalConfig>> = {
  "low-memory": {
    scrollbackLines: 1000,
    enableWebGL: true,
  },
  "default": DEFAULT_TERMINAL_CONFIG,
  "high-performance": {
    scrollbackLines: 5000,
    enableWebGL: true,
    cursorBlink: false, // Reduce repaints
  },
  "debugging": {
    scrollbackLines: 50000,
    enableWebGL: true,
  },
  "accessibility": {
    fontSize: 16,
    cursorStyle: "block",
    cursorBlink: false,
    theme: "light",
  },
};

const STORAGE_KEY = "claude-squad-terminal-config";

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
      // Ensure scrollbackLines is within reasonable bounds
      scrollbackLines: Math.max(
        100,
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
