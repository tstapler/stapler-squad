"use client";
// +feature: settings-input-mode-override

import { useInputModeOverride, InputModeOverride } from "@/lib/hooks/useInputModeOverride";
import {
  container,
  sectionTitle,
  sectionDescription,
  optionRow,
  optionButton,
  optionButtonActive,
  optionLabel,
  optionDescription,
} from "./InputModeSetting.css";

const OPTIONS: { value: InputModeOverride; label: string; description: string }[] = [
  {
    value: "auto",
    label: "Auto-detect (recommended)",
    description: "Collapse the mobile keyboard row and toolbar when a mouse and physical keyboard are detected.",
  },
  {
    value: "desktop",
    label: "Always compact",
    description: "Always hide the on-screen keyboard row and keep the toolbar collapsed by default.",
  },
  {
    value: "touch",
    label: "Always touch-optimized",
    description: "Always show the on-screen keyboard row, even if a mouse is detected.",
  },
];

/** Settings > Appearance — override for mouse+keyboard-on-mobile detection. */
export function InputModeSetting() {
  const { inputModeOverride, setInputModeOverride } = useInputModeOverride();

  return (
    <div className={container}>
      <div>
        <h3 className={sectionTitle}>Terminal Input Mode</h3>
        <p className={sectionDescription}>
          Controls whether the compact terminal toolbar (no on-screen keyboard row) is used
          when a real mouse and keyboard are attached to a phone or tablet.
        </p>
      </div>
      <div className={optionRow} role="radiogroup" aria-label="Terminal input mode">
        {OPTIONS.map((opt) => {
          const isActive = inputModeOverride === opt.value;
          return (
            <button
              key={opt.value}
              role="radio"
              aria-checked={isActive}
              className={`${optionButton}${isActive ? ` ${optionButtonActive}` : ""}`}
              onClick={() => setInputModeOverride(opt.value)}
              data-testid={`input-mode-option-${opt.value}`}
            >
              <span className={optionLabel}>{opt.label}</span>
              <span className={optionDescription}>{opt.description}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
