/**
 * Global Terminal Settings
 *
 * Provides a centralized settings panel for terminal-wide configuration.
 * Manages default settings that apply to all terminal sessions, with the
 * ability to override per-session via TerminalConfigManager.
 *
 * Key features:
 * - Global default terminal configuration
 * - Theme settings (light/dark/auto)
 * - Font configuration (family, size, spacing)
 * - Cursor settings (style, blink)
 * - Scrollback length
 * - Copy/paste behavior defaults
 *
 * Settings are persisted to localStorage under "global-terminal-settings".
 * Per-session overrides use TerminalConfigManager with session-scoped keys.
 */

import { useState, useCallback, useEffect } from "react";
import {
  DEFAULT_TERMINAL_CONFIG,
  type TerminalConfig,
} from "@/lib/config/terminalConfig";

/**
 * Global terminal settings key for localStorage.
 */
export const GLOBAL_SETTINGS_KEY = "global-terminal-settings";

/**
 * Settings that apply globally across all terminal sessions.
 * Separated from TerminalConfig to distinguish global defaults from
 * any per-session overrides.
 */
export interface GlobalTerminalSettings extends TerminalConfig {
  /** When true, new sessions inherit global settings rather than per-host defaults */
  applyToNewSessions: boolean;
  /** When true, the settings panel is collapsed by default */
  collapsedByDefault: boolean;
}

/**
 * Default global settings — extends TerminalConfig with global-only fields.
 */
export const DEFAULT_GLOBAL_SETTINGS: GlobalTerminalSettings = {
  ...DEFAULT_TERMINAL_CONFIG,
  applyToNewSessions: true,
  collapsedByDefault: false,
};

/**
 * Load global terminal settings from localStorage.
 * Falls back to defaults if not set or invalid.
 */
export function loadGlobalTerminalSettings(): GlobalTerminalSettings {
  if (typeof window === "undefined") return DEFAULT_GLOBAL_SETTINGS;

  try {
    const raw = localStorage.getItem(GLOBAL_SETTINGS_KEY);
    if (!raw) return DEFAULT_GLOBAL_SETTINGS;

    const parsed = JSON.parse(raw) as Partial<GlobalTerminalSettings>;
    return { ...DEFAULT_GLOBAL_SETTINGS, ...parsed };
  } catch {
    return DEFAULT_GLOBAL_SETTINGS;
  }
}

/**
 * Save global terminal settings to localStorage.
 */
export function saveGlobalTerminalSettings(
  settings: GlobalTerminalSettings
): void {
  if (typeof window === "undefined") return;

  try {
    localStorage.setItem(GLOBAL_SETTINGS_KEY, JSON.stringify(settings));
  } catch {
    // Quota exceeded or storage unavailable — ignore silently
  }
}

/**
 * React hook for managing global terminal settings state.
 * Provides load/save to localStorage with React state management.
 */
export function useGlobalTerminalSettings(): {
  settings: GlobalTerminalSettings;
  updateSetting: <K extends keyof GlobalTerminalSettings>(
    key: K,
    value: GlobalTerminalSettings[K]
  ) => void;
  resetToDefaults: () => void;
  saveSettings: (settings: GlobalTerminalSettings) => void;
} {
  const [settings, setSettings] = useState<GlobalTerminalSettings>(
    () => loadGlobalTerminalSettings()
  );

  // Persist to localStorage whenever settings change
  useEffect(() => {
    saveGlobalTerminalSettings(settings);
  }, [settings]);

  const updateSetting = useCallback(
    <K extends keyof GlobalTerminalSettings>(
      key: K,
      value: GlobalTerminalSettings[K]
    ) => {
      setSettings((prev) => ({ ...prev, [key]: value }));
    },
    []
  );

  const resetToDefaults = useCallback(() => {
    setSettings(DEFAULT_GLOBAL_SETTINGS);
  }, []);

  const saveSettings = useCallback((newSettings: GlobalTerminalSettings) => {
    setSettings(newSettings);
  }, []);

  return { settings, updateSetting, resetToDefaults, saveSettings };
}

/**
 * Export default settings for external consumption.
 */
export { DEFAULT_TERMINAL_CONFIG };
export type { TerminalConfig };

/**
 * Re-export terminal config defaults from the original module.
 */
export { DEFAULT_TERMINAL_CONFIG as DEFAULTS };
