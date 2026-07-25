"use client";

import { useEffect, useRef } from "react";
import type { SlashCommandInfo } from "@/lib/hooks/useSlashCommands";
import * as styles from "./SlashCommandDropdown.css";

interface SlashCommandDropdownProps {
  id?: string;
  suggestions: SlashCommandInfo[];
  selectedIndex: number;
  onSelect: (cmd: SlashCommandInfo) => void;
}

const SOURCE_LABELS: Record<string, string> = {
  project: "proj",
  user: "user",
  builtin: "built-in",
};

/**
 * Dropdown listing matching slash commands when the user types /command.
 * Follows the same controlled-index pattern as AtCommandDropdown.
 *
 * Usage:
 *   const { isActive, suggestions, complete, wordStart, wordEnd } =
 *     useSlashCommandSuggestions(value, cursorPos, commands);
 *   <SlashCommandDropdown suggestions={suggestions} selectedIndex={idx}
 *     onSelect={cmd => { const { newValue, newCursorPos } = complete(value, cmd); ... }} />
 */
export function SlashCommandDropdown({
  id = "slash-command-listbox",
  suggestions,
  selectedIndex,
  onSelect,
}: SlashCommandDropdownProps) {
  const selectedRef = useRef<HTMLLIElement | null>(null);

  useEffect(() => {
    selectedRef.current?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  if (suggestions.length === 0) {
    return (
      <div className={styles.empty} role="status" aria-live="polite">
        No matching commands
      </div>
    );
  }

  return (
    <ul
      id={id}
      className={styles.dropdown}
      role="listbox"
      aria-label="Slash command suggestions"
    >
      {suggestions.map((cmd, i) => (
        <li
          key={cmd.name}
          id={`${id}-option-${i}`}
          ref={i === selectedIndex ? selectedRef : null}
          className={[styles.item, i === selectedIndex ? styles.itemSelected : ""]
            .filter(Boolean)
            .join(" ")}
          role="option"
          aria-selected={i === selectedIndex}
          onMouseDown={(e) => {
            e.preventDefault();
            onSelect(cmd);
          }}
        >
          <span className={styles.icon} aria-hidden="true">
            /
          </span>
          <span className={styles.name}>/{cmd.name}</span>
          {cmd.title && cmd.title !== cmd.name && (
            <span className={styles.title}>{cmd.title}</span>
          )}
          {cmd.description && (
            <span className={styles.description}>{cmd.description}</span>
          )}
          {cmd.source && (
            <span className={styles.sourceBadge}>
              {SOURCE_LABELS[cmd.source] ?? cmd.source}
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}
