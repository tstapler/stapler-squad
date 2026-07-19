"use client";

import { useId } from "react";
import type { KeyboardEvent, ReactNode } from "react";
import { field, groupLabel as groupLabelClass, radioGroup, radioBtn, radioBtnActive, hint as hintClass } from "./RadioGroup.css";

export interface RadioGroupOption<T extends string> {
  value: T;
  label: string;
  description?: string;
  /** Optional data-testid override for the option's button element. */
  dataTestId?: string;
}

export interface RadioGroupProps<T extends string> {
  options: readonly RadioGroupOption<T>[];
  value: T;
  onChange: (value: T) => void;
  /** Visible group label, rendered above the radio row and linked via aria-labelledby. */
  groupLabel: string;
  /** Optional id override for the group label element. Auto-generated via useId() if omitted. */
  groupLabelId?: string;
  /**
   * Optional per-value hint text, rendered below the radio row and linked to the
   * radiogroup via aria-describedby so screen readers announce it alongside the group.
   */
  hintForValue?: (value: T) => string | undefined;
  /**
   * Optional extra content rendered inline after the option buttons, inside the same
   * flex-wrap radiogroup row (e.g. a "More/Less" progressive-disclosure toggle). Not
   * part of the roving-tabindex/arrow-key cycle — callers are responsible for its
   * own tabIndex/keyboard handling.
   */
  trailingContent?: ReactNode;
}

/**
 * Generic ARIA radiogroup: role="radiogroup" + role="radio" + aria-checked per option,
 * roving tabindex, and arrow-key cycling where arrow keys both move focus AND select
 * (no separate Space/Enter confirmation step).
 */
export function RadioGroup<T extends string>({
  options,
  value,
  onChange,
  groupLabel,
  groupLabelId,
  hintForValue,
  trailingContent,
}: RadioGroupProps<T>) {
  const autoLabelId = useId();
  const autoHintId = useId();
  const labelId = groupLabelId ?? autoLabelId;

  const currentIndex = options.findIndex((o) => o.value === value);
  const hasSelection = currentIndex !== -1;
  const hintText = hintForValue?.(value);

  function handleKeyDown(e: KeyboardEvent) {
    const fromIndex = hasSelection ? currentIndex : 0;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      const next = (fromIndex + 1) % options.length;
      onChange(options[next].value);
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      const prev = (fromIndex - 1 + options.length) % options.length;
      onChange(options[prev].value);
    }
  }

  return (
    <div className={field}>
      <span id={labelId} className={groupLabelClass}>
        {groupLabel}
      </span>
      <div
        role="radiogroup"
        aria-labelledby={labelId}
        aria-describedby={hintText ? autoHintId : undefined}
        className={radioGroup}
        onKeyDown={handleKeyDown}
      >
        {options.map((option, idx) => (
          <button
            key={option.value}
            role="radio"
            aria-checked={value === option.value}
            tabIndex={value === option.value ? 0 : (!hasSelection && idx === 0 ? 0 : -1)}
            type="button"
            onClick={() => onChange(option.value)}
            data-testid={option.dataTestId}
            className={[radioBtn, value === option.value ? radioBtnActive : ""]
              .filter(Boolean)
              .join(" ")}
          >
            {option.label}
          </button>
        ))}
        {trailingContent}
      </div>
      {hintText && (
        <span id={autoHintId} className={hintClass}>
          {hintText}
        </span>
      )}
    </div>
  );
}
