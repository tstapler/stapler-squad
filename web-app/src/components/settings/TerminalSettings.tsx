"use client";
// +feature: settings-terminal-config

import { useEffect, useState } from "react";
import {
  DEFAULT_TERMINAL_CONFIG,
  loadTerminalConfig,
  saveTerminalConfig,
  type TerminalConfig,
} from "@/lib/config/terminalConfig";
import {
  container,
  sectionTitle,
  sectionDescription,
  optionRow,
  optionButton,
  optionButtonActive,
  optionLabel,
} from "./InputModeSetting.css";

const MIN_FONT_SIZE = 8;
const MAX_FONT_SIZE = 32;
const CURSOR_STYLES: TerminalConfig["cursorStyle"][] = ["block", "underline", "bar"];

type Editable = Pick<TerminalConfig, "fontSize" | "cursorStyle" | "cursorBlink">;

/** Settings > Appearance — font size and cursor for every terminal, applied live. */
export function TerminalSettings() {
  const [values, setValues] = useState<Editable>({
    fontSize: DEFAULT_TERMINAL_CONFIG.fontSize,
    cursorStyle: DEFAULT_TERMINAL_CONFIG.cursorStyle,
    cursorBlink: DEFAULT_TERMINAL_CONFIG.cursorBlink,
  });

  // Loaded after mount: localStorage is unavailable during SSR.
  useEffect(() => {
    const { fontSize, cursorStyle, cursorBlink } = loadTerminalConfig();
    setValues({ fontSize, cursorStyle, cursorBlink });
  }, []);

  const update = (patch: Partial<Editable>) => {
    setValues((prev) => ({ ...prev, ...patch }));
    saveTerminalConfig(patch);
  };

  const onFontSize = (raw: string) => {
    const n = Number(raw);
    if (!Number.isFinite(n)) return;
    update({ fontSize: Math.max(MIN_FONT_SIZE, Math.min(MAX_FONT_SIZE, Math.round(n))) });
  };

  const reset = () =>
    update({
      fontSize: DEFAULT_TERMINAL_CONFIG.fontSize,
      cursorStyle: DEFAULT_TERMINAL_CONFIG.cursorStyle,
      cursorBlink: DEFAULT_TERMINAL_CONFIG.cursorBlink,
    });

  return (
    <div className={container}>
      <div>
        <h3 className={sectionTitle}>Terminal</h3>
        <p className={sectionDescription}>
          Font size and cursor for all terminal sessions. Changes apply immediately and are
          stored in this browser.
        </p>
      </div>

      <label className={optionLabel}>
        Font size ({MIN_FONT_SIZE}–{MAX_FONT_SIZE}px)
        <input
          type="number"
          min={MIN_FONT_SIZE}
          max={MAX_FONT_SIZE}
          value={values.fontSize}
          onChange={(e) => onFontSize(e.target.value)}
          data-testid="terminal-font-size"
        />
      </label>

      <div className={optionRow} role="radiogroup" aria-label="Cursor style">
        {CURSOR_STYLES.map((style) => {
          const isActive = values.cursorStyle === style;
          return (
            <button
              key={style}
              role="radio"
              aria-checked={isActive}
              className={`${optionButton}${isActive ? ` ${optionButtonActive}` : ""}`}
              onClick={() => update({ cursorStyle: style })}
              data-testid={`terminal-cursor-${style}`}
            >
              <span className={optionLabel}>{style[0].toUpperCase() + style.slice(1)} cursor</span>
            </button>
          );
        })}
      </div>

      <label className={optionLabel}>
        <input
          type="checkbox"
          checked={values.cursorBlink}
          onChange={(e) => update({ cursorBlink: e.target.checked })}
          data-testid="terminal-cursor-blink"
        />{" "}
        Blinking cursor
      </label>

      <button type="button" className={optionButton} onClick={reset} data-testid="terminal-settings-reset">
        <span className={optionLabel}>Reset to defaults</span>
      </button>
    </div>
  );
}
