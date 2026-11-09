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
  /** Rendered `aria-disabled` and kept in the tab order; it cannot be selected, and arrow keys skip it. */
  disabled?: boolean;
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

  function step(direction: 1 | -1) {
    const fromIndex = hasSelection ? currentIndex : 0;
    for (let offset = 1; offset <= options.length; offset++) {
      const candidate = options[(((fromIndex + direction * offset) % options.length) + options.length) % options.length];
      if (!candidate.disabled) {
        onChange(candidate.value);
        return;
      }
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      step(1);
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      step(-1);
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
            aria-disabled={option.disabled || undefined}
            tabIndex={value === option.value ? 0 : (!hasSelection && idx === 0 ? 0 : -1)}
            type="button"
            onClick={() => !option.disabled && onChange(option.value)}
            data-testid={option.dataTestId}
            className={[radioBtn, value === option.value ? radioBtnActive : ""]
              .filter(Boolean)
              .join(" ")}
          >{/* analytics-exempt */}
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
