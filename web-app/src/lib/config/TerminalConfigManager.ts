/**
 * Terminal Configuration Manager
 *
 * Centralized configuration management for terminal rendering parameters.
 *
 * This module provides a class-based parameter system for terminal configuration,
 * supporting validation, merging, and runtime updates. It replaces scattered
 * configuration logic with a structured approach to terminal parameter management.
 */

/**
 * Configuration data interface with all terminal rendering parameters.
 */
export interface TerminalConfigData {
  cols: number;
  rows: number;
  fontSize: number;
  fontFamily: string;
  theme: "dark" | "light" | "auto";
  scrollback: number;
  copyOnSelect: boolean;
  copyAsHtml: boolean;
  copyWithTimestamps: boolean;
  shell: string;
  cursorBlink: boolean;
  cursorStyle: "block" | "underline" | "bar";
  letterSpacing: number;
  lineHeight: number;
  enableVisualBell: boolean;
  scrollOnOutput: boolean;
  scrollSensitivity: "lazy" | "smooth" | "instant";
  // Index signature for dynamic property access
  [key: string]: unknown;
}

/**
 * Default configuration values.
 */
export const DEFAULT_TERMINAL_CONFIG_DATA: TerminalConfigData = {
  cols: 80,
  rows: 24,
  fontSize: 14,
  fontFamily: "monospace",
  theme: "dark",
  scrollback: 10000,
  copyOnSelect: true,
  copyAsHtml: false,
  copyWithTimestamps: false,
  shell: "bash",
  cursorBlink: true,
  cursorStyle: "block",
  letterSpacing: 0,
  lineHeight: 1.2,
  enableVisualBell: false,
  scrollOnOutput: true,
  scrollSensitivity: "smooth",
};

/**
 * Merge strategy for configuration updates.
 * - 'shallow': replaces top-level keys only
 * - 'deep': recursively merges nested objects
 */
export type MergeStrategy = "shallow" | "deep";

/**
 * Partial configuration for updates.
 */
export type PartialConfig = Partial<TerminalConfigData>;

/**
 * Result of validating a partial configuration.
 */
export interface ValidationResults {
  valid: boolean;
  data: TerminalConfigData;
  errors: string[];
}

/** Validator return type */
type ValidatorResult = { value: unknown; error?: string };

/** Validator functions for each field type. */
const validators: Record<keyof TerminalConfigData, (v: unknown, def: unknown) => ValidatorResult> = {
  cols: (v, def) => validateInt(v, 1, 1000, def as number),
  rows: (v, def) => validateInt(v, 1, 1000, def as number),
  fontSize: (v, def) => validateInt(v, 8, 72, def as number),
  fontFamily: (v, def) => validateStringField(v, def as string),
  scrollback: (v, def) => validateInt(v, 100, 100000, def as number),
  copyOnSelect: (v, def) => validateBool(v, def as boolean),
  copyAsHtml: (v, def) => validateBool(v, def as boolean),
  copyWithTimestamps: (v, def) => validateBool(v, def as boolean),
  shell: (v, def) => validateStringField(v, def as string),
  cursorBlink: (v, def) => validateBool(v, def as boolean),
  cursorStyle: (v, def) => validateEnumField(v, def as string, ["block", "underline", "bar"]),
  letterSpacing: (v, def) => validateFloat(v, 0, 10, def as number),
  lineHeight: (v, def) => validateFloat(v, 1, 3, def as number),
  enableVisualBell: (v, def) => validateBool(v, def as boolean),
  scrollOnOutput: (v, def) => validateBool(v, def as boolean),
  scrollSensitivity: (v, def) => validateEnumField(v, def as string, ["lazy", "smooth", "instant"]),
  theme: (v, def) => validateEnumField(v, def as string, ["dark", "light", "auto"]),
};

function validateInt(value: unknown, min: number, max: number, def: number): ValidatorResult {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return { value: def };
  }
  return { value: Math.max(min, Math.min(max, Math.round(value))) };
}

function validateFloat(value: unknown, min: number, max: number, def: number): ValidatorResult {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return { value: def };
  }
  return { value: Math.max(min, Math.min(max, value)) };
}

function validateStringField(value: unknown, def: string): ValidatorResult {
  if (typeof value === "string" && value.length > 0) {
    return { value: value };
  }
  return { value: def };
}

function validateBool(value: unknown, def: boolean): ValidatorResult {
  if (typeof value === "boolean") {
    return { value: value };
  }
  if (typeof value === "string") {
    if (value === "true") return { value: true };
    if (value === "false") return { value: false };
  }
  return { value: def };
}

function validateEnumField(
  value: unknown,
  def: string,
  allowed: readonly string[],
): ValidatorResult {
  if (typeof value === "string" && allowed.includes(value)) {
    return { value: value };
  }
  if (typeof value !== "string") {
    return { value: def };
  }
  return {
    value: def,
    error: `Invalid value "${value}", expected one of: ${allowed.join(", ")}`,
  };
}

/**
 * Validate a raw object against the terminal config schema,
 * merging with defaults. Returns validated data and any errors.
 */
export function validateTerminalConfig(
  partial: unknown,
  defaults: TerminalConfigData = DEFAULT_TERMINAL_CONFIG_DATA,
): ValidationResults {
  const errors: string[] = [];
  const result: Partial<TerminalConfigData> = { ...defaults };

  if (!partial || typeof partial !== "object" || Array.isArray(partial)) {
    if (partial !== undefined && partial !== null) {
      errors.push("Configuration must be an object");
    }
    return { valid: errors.length === 0, data: { ...defaults }, errors };
  }

  const obj = partial as Record<string, unknown>;

  for (const key of Object.keys(validators)) {
    if (key in obj) {
      const { value, error } = validators[key](obj[key], defaults[key]);
      (result as Record<string, unknown>)[key] = value;
      if (error) errors.push(error);
    }
  }

  return { valid: errors.length === 0, data: result as TerminalConfigData, errors };
}

/**
 * Merge two configuration objects deeply.
 * For each key in override, if both values are objects and neither is an array,
 * recursively merge them. Otherwise, override the value.
 */
function deepMerge(base: TerminalConfigData, override: PartialConfig): TerminalConfigData {
  const result: TerminalConfigData = { ...base };

  for (const key of Object.keys(override) as (keyof PartialConfig)[]) {
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
      (result as Record<string, unknown>)[key] = deepMerge(
        baseVal as TerminalConfigData,
        overrideVal as PartialConfig,
      );
    } else if (overrideVal !== undefined) {
      (result as Record<string, unknown>)[key] = overrideVal;
    }
  }

  return result;
}

/**
 * TerminalConfigManager — manages terminal configuration with validation,
 * merging, and runtime update capabilities.
 */
export class TerminalConfigManager {
  private config: TerminalConfigData;
  private readonly storageKey: string;
  private readonly defaults: TerminalConfigData;

  /**
   * @param sessionId - Session identifier for namespaced localStorage persistence
   * @param initialConfig - Optional partial configuration to merge with defaults
   * @param storageKey - Optional custom localStorage key
   */
  constructor(sessionId: string, initialConfig?: PartialConfig, storageKey?: string) {
    this.storageKey = storageKey ?? `terminal-config-${sessionId}`;
    this.defaults = { ...DEFAULT_TERMINAL_CONFIG_DATA };

    const persisted = this.loadFromStorage();
    const validated = validateTerminalConfig(
      { ...this.defaults, ...persisted, ...initialConfig },
      this.defaults,
    );
    this.config = validated.data;
  }

  getConfig(): TerminalConfigData {
    return { ...this.config };
  }

  get<K extends keyof TerminalConfigData>(key: K): TerminalConfigData[K] {
    return this.config[key];
  }

  update(partial: PartialConfig, strategy: MergeStrategy = "shallow"): TerminalConfigData {
    const merged =
      strategy === "deep"
        ? deepMerge(this.config, partial)
        : { ...this.config, ...partial };

    const validated = validateTerminalConfig(merged, this.defaults);
    this.config = validated.data;
    this.saveToStorage(this.config);
    return this.getConfig();
  }

  reset(keep?: (keyof TerminalConfigData)[]): TerminalConfigData {
    if (keep && keep.length > 0) {
      const preserved = keep.reduce(
        (acc, key) => ({ ...acc, [key]: this.config[key] }),
        {} as PartialConfig,
      );
      const validated = validateTerminalConfig(
        { ...this.defaults, ...preserved },
        this.defaults,
      );
      this.config = validated.data;
    } else {
      this.config = { ...this.defaults };
    }
    this.saveToStorage(this.config);
    return this.getConfig();
  }

  validate(partial: unknown): { valid: true; data: TerminalConfigData } | { valid: false; errors: string[] } {
    const result = validateTerminalConfig(partial, this.defaults);
    if (result.valid) {
      return { valid: true, data: result.data };
    }
    return { valid: false, errors: result.errors };
  }

  private loadFromStorage(): PartialConfig | undefined {
    if (typeof window === "undefined") return undefined;
    try {
      const raw = localStorage.getItem(this.storageKey);
      if (!raw) return undefined;
      const parsed = JSON.parse(raw) as PartialConfig;
      const validated = validateTerminalConfig(parsed, this.defaults);
      return validated.valid ? parsed : undefined;
    } catch {
      return undefined;
    }
  }

  private saveToStorage(config: TerminalConfigData): void {
    if (typeof window === "undefined") return;
    try {
      localStorage.setItem(this.storageKey, JSON.stringify(config));
    } catch {
      // Quota exceeded or storage unavailable — ignore silently
    }
  }

  toJSON(): TerminalConfigData {
    return this.getConfig();
  }

  static createForEnvironment(sessionId: string, envConfig?: PartialConfig): TerminalConfigManager {
    return new TerminalConfigManager(sessionId, envConfig);
  }
}

