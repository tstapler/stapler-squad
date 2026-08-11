"use client";
// +feature: settings-theme-picker

import { useTheme, ThemeName } from "@/lib/contexts/ThemeContext";
import {
  container,
  sectionTitle,
  sectionDescription,
  grid,
  themeButton,
  themeButtonActive,
  previewSwatch,
  themeName,
  themeDescription,
  activeCheckmark,
} from "./ThemePicker.css";

/** Human-readable metadata for each theme. */
const THEME_META: Record<ThemeName, { label: string; description: string }> = {
  matrix: {
    label: "Matrix",
    description: "Green on black, JetBrains Mono",
  },
  cyberpunk77: {
    label: "Cyberpunk 77",
    description: "Neon yellow + pink on navy",
  },
  wh40k: {
    label: "WH40K",
    description: "Grimdark parchment + gold",
  },
  clean: {
    label: "Clean",
    description: "Purple accent, deep charcoal",
  },
  light: {
    label: "Light",
    description: "Classic light mode",
  },
  dark: {
    label: "Dark",
    description: "Classic dark mode",
  },
};

/**
 * Story 1.4.5 — Settings theme picker.
 *
 * Renders a grid of theme preview swatches. The active theme gets a primary-colored
 * border and checkmark. Switching themes is instant via ThemeContext.setTheme.
 */
export function ThemePicker() {
  const { theme, setTheme, availableThemes } = useTheme();

  return (
    <div className={container}>
      <div>
        <h3 className={sectionTitle}>Appearance</h3>
        <p className={sectionDescription}>
          Choose a theme. Your preference is saved locally and applied on next load.
        </p>
      </div>
      <div className={grid} role="radiogroup" aria-label="Theme selection">
        {availableThemes.map((name) => {
          const meta = THEME_META[name];
          const isActive = theme === name;
          const swatchKey = name as keyof typeof previewSwatch;
          return (
            <button
              key={name}
              role="radio"
              aria-checked={isActive}
              className={`${themeButton}${isActive ? ` ${themeButtonActive}` : ""}`}
              onClick={() => setTheme(name)}
              title={meta.description}
            >
              {isActive && (
                <span className={activeCheckmark} aria-hidden="true">
                  ✓
                </span>
              )}
              <div className={previewSwatch[swatchKey]} aria-hidden="true" />
              <span className={themeName}>{meta.label}</span>
              <span className={themeDescription}>{meta.description}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
