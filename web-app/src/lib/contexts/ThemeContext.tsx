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

interface ThemeContextValue {
  theme: ThemeName;
  setTheme: (name: ThemeName) => void;
  availableThemes: ThemeName[];
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    throw new Error("useTheme must be used within a ThemeProvider");
  }
  return ctx;
}

interface ThemeProviderProps {
  children: ReactNode;
  /** Initial theme applied during SSR (defaults to "matrix"). Must match the FOUC script. */
  initialTheme?: ThemeName;
}

export function ThemeProvider({ children, initialTheme = "matrix" }: ThemeProviderProps) {
  const [theme, setThemeState] = useState<ThemeName>(initialTheme);
  const initialized = useRef(false);

  // On first mount, read localStorage and apply the persisted theme
  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;

    let persisted: ThemeName = initialTheme;
    try {
      const stored = localStorage.getItem(STORAGE_KEY) as ThemeName | null;
      if (stored && stored in THEME_CLASSES) {
        persisted = stored;
      }
    } catch {
      // localStorage unavailable
    }

    applyThemeClass(persisted);
    setThemeState(persisted);
  }, [initialTheme]);

  const setTheme = useCallback((name: ThemeName) => {
    applyThemeClass(name);
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
      }}
    >
      {children}
    </ThemeContext.Provider>
  );
}

function applyThemeClass(name: ThemeName) {
  if (typeof document === "undefined") return;
  const html = document.documentElement;
  // Remove all known theme classes, add the new one
  html.classList.remove(...ALL_THEME_CLASSES);
  html.classList.add(THEME_CLASSES[name]);
}
