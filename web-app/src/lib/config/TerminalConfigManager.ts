/**
 * Terminal Configuration Manager
 *
 * Centralized configuration management for terminal rendering parameters.
 *
 * This module provides a class-based parameter system for terminal configuration,
 * supporting validation, merging, and runtime updates. It replaces scattered
 * configuration logic with a structured approach to terminal parameter management.
 *
 * Key features:
 * - Default configuration management
 * - Parameter validation and normalization
 * - Configuration merging and inheritance
 * - Runtime configuration updates
 * - Environment-specific configurations
 */

import { z } from "zod";

/**
 * Schema for terminal configuration validation.
 * Centralizes all terminal rendering parameters with type-safe validation.
 */
const TerminalConfigSchema = z.object({
  // Core terminal dimensions
  cols: z.number().int().min(1).max(1000).default(80),
  rows: z.number().int().min(1).max(1000).default(24),

  // Font settings
  fontSize: z.number().min(8).max(72).default(14),
  fontFamily: z.string().default("monospace"),

  // Color scheme
  theme: z.enum(["dark", "light", "auto"]).default("dark"),

  // Scrollback buffer
  scrollback: z.number().int().min(100).max(100000).default(10000),

  // Copy-paste settings
  copyOnSelect: z.boolean().default(true),
  copyAsHtml: z.boolean().default(false),
  copyWithTimestamps: z.boolean().default(false),

  // Shell settings
  shell: z.string().default("bash"),

  // Terminal behavior
  cursorBlink: z.boolean().default(true),
  cursorStyle: z.enum(["block", "underline", "bar"]).default("block"),

  // Rendering options
  letterSpacing: z.number().min(0).max(10).default(0),
  lineHeight: z.number().min(1).max(3).default(1.2),

  // Advanced settings
  enableVisualBell: z.boolean().default(false),
  scrollOnOutput: z.boolean().default(true),
  scrollSensitivity: z.enum(["lazy", "smooth", "instant"]).default("smooth"),
});

/**
 * Type inferred from the schema - represents a fully validated terminal configuration.
 */
export type TerminalConfigData = z.infer<typeof TerminalConfigSchema>;

/**
 * Merge strategy for configuration updates.
 * - 'shallow': replaces top-level keys only
 * - 'deep': recursively merges nested objects
 */
export type MergeStrategy = "shallow" | "deep";

/**
 * TerminalConfigManager — manages terminal configuration with validation,
 * merging, and runtime update capabilities.
 *
 * This class provides a single source of truth for terminal rendering parameters,
 * replacing scattered configuration logic across components. It supports:
 * - Loading defaults from the schema
 * - Merging partial configurations
 * - Runtime updates with validation
 * - Environment-specific overrides
 * - Persistence via localStorage
 */
export class TerminalConfigManager {
  private config: TerminalConfigData;
  private readonly storageKey: string;
  private readonly defaults: TerminalConfigData;

  /**
   * Create a new TerminalConfigManager.
   *
   * @param sessionId - Session identifier used for namespaced localStorage persistence
   * @param initialConfig - Optional partial configuration to merge with defaults
   * @param storageKey - Optional custom localStorage key
   */
  constructor(
    sessionId: string,
    initialConfig?: Partial<TerminalConfigData>,
    storageKey?: string,
  ) {
    this.storageKey = storageKey ?? `terminal-config-${sessionId}`;
    this.defaults = TerminalConfigSchema.parse({});

    // Load persisted config if available, merge with initial, fall back to defaults
    const persisted = this.loadFromStorage();
    this.config = TerminalConfigSchema.parse({
      ...this.defaults,
      ...persisted,
      ...initialConfig,
    });
  }

  /**
   * Get the full current configuration.
   * Returns a copy to prevent external mutation.
   */
  getConfig(): TerminalConfigData {
    return { ...this.config };
  }

  /**
   * Get a specific configuration value by key.
   */
  get<K extends keyof TerminalConfigData>(key: K): TerminalConfigData[K] {
    return this.config[key];
  }

  /**
   * Update configuration with a partial override, validated against the schema.
   *
   * @param partial - Partial configuration to merge
   * @param strategy - Merge strategy ('shallow' or 'deep')
   * @returns The new merged configuration
   * @throws If validation fails
   */
  update(
    partial: Partial<TerminalConfigData>,
    strategy: MergeStrategy = "shallow",
  ): TerminalConfigData {
    const merged =
      strategy === "deep"
        ? this.deepMerge(this.config, partial)
        : { ...this.config, ...partial };

    this.config = TerminalConfigSchema.parse(merged);
    this.saveToStorage(this.config);
    return this.getConfig();
  }

  /**
   * Reset configuration to defaults.
   *
   * @param keep - Optional list of keys to preserve from current config
   */
  reset(keep?: (keyof TerminalConfigData)[]): TerminalConfigData {
    if (keep && keep.length > 0) {
      const preserved = keep.reduce(
        (acc, key) => ({ ...acc, [key]: this.config[key] }),
        {} as Partial<TerminalConfigData>,
      );
      this.config = TerminalConfigSchema.parse({
        ...this.defaults,
        ...preserved,
      });
    } else {
      this.config = { ...this.defaults };
    }
    this.saveToStorage(this.config);
    return this.getConfig();
  }

  /**
   * Validate a partial configuration without applying it.
   *
   * @returns `{ valid: true, data }` if valid, `{ valid: false, errors }` otherwise
   */
  validate(partial: unknown):
    | { valid: true; data: TerminalConfigData }
    | { valid: false; errors: string[] } {
    const result = TerminalConfigSchema.safeParse(
      partial instanceof Object && partial !== null
        ? { ...this.defaults, ...partial }
        : this.defaults,
    );

    if (!result.success) {
      return {
        valid: false,
        errors: result.error.errors.map((e) => e.message),
      };
    }
    return { valid: true, data: result.data };
  }

  /**
   * Load persisted configuration from localStorage.
   * Only available in browser environments.
   */
  private loadFromStorage(): Partial<TerminalConfigData> | undefined {
    if (typeof window === "undefined") return undefined;
    try {
      const raw = localStorage.getItem(this.storageKey);
      if (!raw) return undefined;
      const parsed = JSON.parse(raw);
      // Validate persisted data against schema, return only valid subset
      const validated = TerminalConfigSchema.partial().safeParse(parsed);
      return validated.success ? validated.data : undefined;
    } catch {
      return undefined;
    }
  }

  /**
   * Persist configuration to localStorage.
   * Only available in browser environments.
   */
  private saveToStorage(config: TerminalConfigData): void {
    if (typeof window === "undefined") return;
    try {
      localStorage.setItem(this.storageKey, JSON.stringify(config));
    } catch {
      // Ignore storage errors (quota exceeded, etc.)
    }
  }

  /**
   * Deep merge two configuration objects.
   * Arrays are replaced, nested objects are merged recursively.
   */
  private deepMerge(
    base: TerminalConfigData,
    override: Partial<TerminalConfigData>,
  ): TerminalConfigData {
    const result: TerminalConfigData = { ...base };

    for (const key of Object.keys(override) as (keyof TerminalConfigData)[]):
      const baseVal = base[key];
      const overrideVal = override[key];

      if (
        baseVal !== null &&
        typeof baseVal === "object" &&
        !Array.isArray(baseVal) &&
        overrideVal !== null &&
        typeof overrideVal === "object" &&
        !Array.isArray(overrideVal)
      ) {
        (result as Record<string, unknown>)[key] = this.deepMerge(
          baseVal as TerminalConfigData,
          overrideVal as Partial<TerminalConfigData>,
        );
      } else if (overrideVal !== undefined) {
        (result as Record<string, unknown>)[key] = overrideVal;
      }

    return result;
  }

  /**
   * Export the current configuration as a plain object.
   * Useful for serialization or passing to xterm.js constructors.
   */
  toJSON(): TerminalConfigData {
    return this.getConfig();
  }

  /**
   * Create a TerminalConfigManager from environment variables and stored config.
   * Convenience factory for SSR/hydration scenarios.
   */
  static createForEnvironment(
    sessionId: string,
    envConfig?: Partial<TerminalConfigData>,
  ): TerminalConfigManager {
    return new TerminalConfigManager(sessionId, envConfig);
  }
}
