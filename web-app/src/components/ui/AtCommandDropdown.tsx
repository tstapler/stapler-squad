"use client";

import { useEffect, useRef } from "react";
import type { WorkflowEntry } from "@/lib/omnibar/detectors/WorkflowDetector";
import * as styles from "./AtCommandDropdown.css";

interface AtCommandDropdownProps {
  id?: string;
  suggestions: WorkflowEntry[];
  selectedIndex: number;
  onSelect: (suggestion: WorkflowEntry) => void;
}

/**
 * Dropdown that lists matching workflows when the user types @slug in an input.
 * Follows the same controlled-index pattern as PathCompletionDropdown.
 *
 * Usage:
 *   const { isAtCommand, suggestions, complete } = useAtCommandSuggestions(value, workflows);
 *   <AtCommandDropdown suggestions={suggestions} selectedIndex={idx} onSelect={wf => setValue(complete(wf))} />
 */
export function AtCommandDropdown({
  id = "at-command-listbox",
  suggestions,
  selectedIndex,
  onSelect,
}: AtCommandDropdownProps) {
  const selectedRef = useRef<HTMLLIElement | null>(null);

  useEffect(() => {
    selectedRef.current?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  if (suggestions.length === 0) {
    return (
      <div className={styles.empty} role="status" aria-live="polite">
        No matching workflows
      </div>
    );
  }

  return (
    <ul
      id={id}
      className={styles.dropdown}
      role="listbox"
      aria-label="Workflow suggestions"
    >
      {suggestions.map((wf, i) => (
        <li
          key={wf.slug}
          id={`${id}-option-${i}`}
          ref={i === selectedIndex ? selectedRef : null}
          className={[styles.item, i === selectedIndex ? styles.itemSelected : ""]
            .filter(Boolean)
            .join(" ")}
          role="option"
          aria-selected={i === selectedIndex}
          onMouseDown={(e) => {
            e.preventDefault();
            onSelect(wf);
          }}
        >
          <span className={styles.icon} aria-hidden="true">
            ⚡
          </span>
          <span className={styles.slug}>@{wf.slug}</span>
          <span className={styles.name}>{wf.name}</span>
          {wf.description && (
            <span className={styles.description}>{wf.description}</span>
          )}
        </li>
      ))}
    </ul>
  );
}
