"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

/**
 * Config for a single persisted field. `isValid` should be provided for
 * fields with a fixed set of legal values (enums, known columns) so a
 * stale/removed value left over from a previous release falls back to
 * `defaultValue` instead of propagating into app state.
 */
export interface PersistedFieldConfig<V> {
  key: string;
  defaultValue: V;
  serialize?: (value: V) => unknown;
  deserialize?: (raw: unknown) => V;
  isValid?: (value: unknown) => boolean;
}

export type PersistedFieldsConfig<T extends object> = {
  [K in keyof T]: PersistedFieldConfig<T[K]>;
};

export type PersistedViewStateSetters<T extends object> = {
  [K in keyof T]: (value: T[K] | ((prev: T[K]) => T[K])) => void;
};

export interface UsePersistedViewStateReturn<T extends object> {
  state: T;
  setters: PersistedViewStateSetters<T>;
  isHydrated: boolean;
  /** Resets every field to its default in memory and clears its localStorage key. */
  resetToDefaults: () => void;
  /** Removes every field's localStorage key without touching in-memory state. */
  clearStorage: () => void;
}

function readRaw(key: string): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeRaw(key: string, value: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Ignore storage errors (private browsing / quota exceeded) — state
    // still lives in memory, it just won't survive a reload.
  }
}

function removeRaw(key: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(key);
  } catch {
    // Ignore storage errors
  }
}

function loadField<V>(config: PersistedFieldConfig<V>): V {
  const raw = readRaw(config.key);
  if (raw === null) return config.defaultValue;

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return config.defaultValue;
  }

  const value = config.deserialize ? config.deserialize(parsed) : (parsed as V);
  if (config.isValid && !config.isValid(value)) return config.defaultValue;
  return value;
}

function saveField<V>(config: PersistedFieldConfig<V>, value: V): void {
  const serialized = config.serialize ? config.serialize(value) : value;
  writeRaw(config.key, JSON.stringify(serialized));
}

function buildDefaults<T extends object>(fields: PersistedFieldsConfig<T>): T {
  const out = {} as T;
  (Object.keys(fields) as (keyof T)[]).forEach((name) => {
    out[name] = fields[name].defaultValue;
  });
  return out;
}

/**
 * Generic hydration-safe localStorage persistence for a page/component's view
 * state (search, filters, sort, grouping, etc). State always starts at the
 * hardcoded defaults so SSR/static-export first paint can never mismatch a
 * client-only localStorage read; a mount-only effect then loads persisted
 * values and flips `isHydrated`.
 */
export function usePersistedViewState<T extends object>(
  fields: PersistedFieldsConfig<T>
): UsePersistedViewStateReturn<T> {
  const fieldsRef = useRef(fields);
  fieldsRef.current = fields;

  const defaults = useMemo(() => buildDefaults(fields), [fields]);
  const defaultsRef = useRef(defaults);
  defaultsRef.current = defaults;

  const [state, setState] = useState<T>(defaults);
  const [isHydrated, setIsHydrated] = useState(false);

  // Skips the persistence effect's next run once, so resetToDefaults's
  // removeRaw calls aren't immediately undone by re-saving the new (default)
  // state back into localStorage.
  const skipPersistRef = useRef(false);

  useEffect(() => {
    const loaded = {} as T;
    (Object.keys(fieldsRef.current) as (keyof T)[]).forEach((name) => {
      loaded[name] = loadField(fieldsRef.current[name]);
    });
    setState(loaded);
    setIsHydrated(true);
    // Mount-only: hydration must run exactly once regardless of prop identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!isHydrated) return;
    if (skipPersistRef.current) {
      skipPersistRef.current = false;
      return;
    }
    (Object.keys(fieldsRef.current) as (keyof T)[]).forEach((name) => {
      saveField(fieldsRef.current[name], state[name]);
    });
  }, [state, isHydrated]);

  const setters = useMemo(() => {
    const out = {} as PersistedViewStateSetters<T>;
    (Object.keys(fields) as (keyof T)[]).forEach((name) => {
      out[name] = (value) => {
        setState((prev) => ({
          ...prev,
          [name]: typeof value === "function" ? (value as (prev: T[typeof name]) => T[typeof name])(prev[name]) : value,
        }));
      };
    });
    return out;
  }, [fields]);

  const clearStorage = useCallback(() => {
    (Object.keys(fieldsRef.current) as (keyof T)[]).forEach((name) => {
      removeRaw(fieldsRef.current[name].key);
    });
  }, []);

  const resetToDefaults = useCallback(() => {
    skipPersistRef.current = true;
    clearStorage();
    setState(defaultsRef.current);
  }, [clearStorage]);

  return { state, setters, isHydrated, resetToDefaults, clearStorage };
}
