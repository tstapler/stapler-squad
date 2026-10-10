"use client";

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  ReactNode,
} from "react";
import {
  matrixTheme,
  cyberpunk77Theme,
  wh40kTheme,
  cleanTheme,
  lightTheme,
  darkTheme,
} from "@/styles/theme.css";
import { vars } from "@/styles/theme-contract.css";

const STORAGE_KEY = "stapler-theme";

export type ThemeName = "matrix" | "cyberpunk77" | "wh40k" | "clean" | "light" | "dark";

/** Maps theme names to their vanilla-extract CSS class strings */
export const THEME_CLASSES: Record<ThemeName, string> = {
  matrix: matrixTheme,
  cyberpunk77: cyberpunk77Theme,
  wh40k: wh40kTheme,
  clean: cleanTheme,
  light: lightTheme,
  dark: darkTheme,
};

const ALL_THEME_CLASSES = Object.values(THEME_CLASSES);

const CUSTOM_PREFIX = "custom:";

/** A server-supplied theme (`GET /api/themes`): a built-in base plus dotted-path token overrides. */
export interface UserTheme {
  id: string;
  label: string;
  description?: string;
  base: ThemeName;
  tokens: Record<string, string>;
}

interface ThemeContextValue {
  /** A built-in ThemeName, or `custom:<id>` for a user theme. */
  theme: string;
  setTheme: (name: string) => void;
  availableThemes: ThemeName[];
  customThemes: UserTheme[];
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    // Return a no-op fallback when used outside a ThemeProvider.
    // This allows components to be rendered in test environments or embedded contexts.
    return {
      theme: "clean" as ThemeName,
      setTheme: () => {},
      availableThemes: [],
      customThemes: [],
    };
  }
  return ctx;
}

interface ThemeProviderProps {
  children: ReactNode;
  /** Initial theme applied during SSR (defaults to "clean"). Must match the FOUC script. */
  initialTheme?: ThemeName;
}

export function ThemeProvider({ children, initialTheme = "clean" }: ThemeProviderProps) {
  const [theme, setThemeState] = useState<string>(initialTheme);
  const [customThemes, setCustomThemes] = useState<UserTheme[]>([]);
  const customThemesRef = useRef<UserTheme[]>([]);
  // Latest selection, so the async /api/themes response never clobbers a choice made while it was in flight.
  const themeRef = useRef<string>(initialTheme);
  const initialized = useRef(false);

  // On first mount, read localStorage and apply the persisted theme
  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;

    let persisted: string = initialTheme;
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored && (stored in THEME_CLASSES || stored.startsWith(CUSTOM_PREFIX))) {
        persisted = stored;
      }
    } catch {
      // localStorage unavailable
    }

    // A persisted custom theme can't apply until /api/themes answers; show the default meanwhile.
    applyTheme(persisted in THEME_CLASSES ? persisted : initialTheme, []);
    themeRef.current = persisted;
    setThemeState(persisted);

    fetch("/api/themes")
      .then((res) => (res.ok ? res.json() : { themes: [] }))
      .then((body: { themes?: UserTheme[] }) => {
        const themes = body.themes ?? [];
        customThemesRef.current = themes;
        setCustomThemes(themes);
        const current = themeRef.current;
        if (!current.startsWith(CUSTOM_PREFIX)) return;
        if (themes.some((t) => `${CUSTOM_PREFIX}${t.id}` === current)) {
          applyTheme(current, themes);
        } else {
          // Persisted custom theme no longer exists (file removed): fall back rather than show nothing selected.
          themeRef.current = initialTheme;
          setThemeState(initialTheme);
        }
      })
      .catch(() => {
        // server without /api/themes: built-in themes only
      });
  }, [initialTheme]);

  const setTheme = useCallback((name: string) => {
    themeRef.current = name;
    applyTheme(name, customThemesRef.current);
    setThemeState(name);
    try {
      localStorage.setItem(STORAGE_KEY, name);
    } catch {
      // ignore
    }
  }, []);

  return (
    <ThemeContext.Provider
      value={{
        theme,
        setTheme,
        availableThemes: Object.keys(THEME_CLASSES) as ThemeName[],
        customThemes,
      }}
    >
      {children}
    </ThemeContext.Provider>
  );
}

let appliedOverrides: string[] = [];

/** `var(--color-primary__x1)` -> `--color-primary__x1`, walking a dotted path through the contract. */
function cssVarName(path: string): string | null {
  let node: unknown = vars;
  for (const key of path.split(".")) {
    if (typeof node !== "object" || node === null || !(key in node)) return null;
    node = (node as Record<string, unknown>)[key];
  }
  const match = typeof node === "string" ? /^var\((--[^),\s]+)/.exec(node) : null;
  return match ? match[1] : null;
}

function applyTheme(name: string, custom: UserTheme[]) {
  if (typeof document === "undefined") return;
  const style = document.documentElement.style;
  appliedOverrides.forEach((prop) => style.removeProperty(prop));
  appliedOverrides = [];

  const user = name.startsWith(CUSTOM_PREFIX)
    ? custom.find((t) => t.id === name.slice(CUSTOM_PREFIX.length))
    : undefined;
  if (!user) {
    applyThemeClass(name in THEME_CLASSES ? (name as ThemeName) : "clean");
    return;
  }
  applyThemeClass(user.base in THEME_CLASSES ? user.base : "clean");
  for (const [path, value] of Object.entries(user.tokens)) {
    const prop = cssVarName(path);
    if (!prop) continue;
    style.setProperty(prop, value);
    appliedOverrides.push(prop);
  }
}

function applyThemeClass(name: ThemeName) {
  if (typeof document === "undefined") return;
  const html = document.documentElement;
  // Remove all known theme classes, add the new one
  html.classList.remove(...ALL_THEME_CLASSES);
  html.classList.add(THEME_CLASSES[name]);
}
