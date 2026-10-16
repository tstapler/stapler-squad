// +feature: settings-programs
"use client";

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { applyCompletion, filterFlags, tokenAtCaret, type FlagOption } from "@/lib/flags/flagTokens";
import { useListboxNav } from "@/lib/flags/useListboxNav";
import * as styles from "./FlagCombobox.css";

interface FlagComboboxProps {
  id: string;
  value: string;
  onChange: (value: string) => void;
  /** Empty means "plain input": no combobox role, no listbox. */
  flags: readonly FlagOption[];
  testId?: string;
  describedBy?: string;
  className?: string;
  placeholder?: string;
}

/**
 * Text input that completes the flag token at the caret. Always renders the
 * same <input>, so focus and caret survive flags arriving after a late probe.
 */
export function FlagCombobox({
  id,
  value,
  onChange,
  flags,
  testId,
  describedBy,
  className,
  placeholder,
}: FlagComboboxProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const pendingCaret = useRef<number | null>(null);
  const [caret, setCaret] = useState(value.length);
  const nav = useListboxNav();

  const hasFlags = flags.length > 0;
  const token = tokenAtCaret(value, caret);
  const matches = hasFlags && token ? filterFlags(flags, token.text) : [];
  const expanded = nav.open && matches.length > 0;
  const listboxId = `${id}-listbox`;
  const optionId = (i: number) => `${id}-option-${i}`;
  const activeFlag = expanded && nav.active >= 0 ? matches[nav.active] : undefined;
  const activeDescId = activeFlag?.description ? `${id}-active-description` : undefined;
  const describedByIds = [activeDescId, describedBy].filter(Boolean).join(" ") || undefined;

  useLayoutEffect(() => {
    if (pendingCaret.current === null) return;
    inputRef.current?.setSelectionRange(pendingCaret.current, pendingCaret.current);
    pendingCaret.current = null;
  });

  useEffect(() => {
    if (!expanded || nav.active < 0) return;
    document.getElementById(optionId(nav.active))?.scrollIntoView?.({ block: "nearest" });
    // optionId is derived from id
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [expanded, nav.active]);

  const accept = (index: number) => {
    if (!token) return;
    const next = applyCompletion(value, token, matches[index]);
    pendingCaret.current = next.caret;
    setCaret(next.caret);
    onChange(next.value);
  };

  const syncCaret = (el: HTMLInputElement) => setCaret(el.selectionStart ?? el.value.length);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    const result = nav.onKey(e.key, e.altKey, matches.length);
    if (result.handled) e.preventDefault();
    if (result.accept !== null) accept(result.accept);
  };

  const comboboxProps = hasFlags
    ? {
        role: "combobox" as const,
        "aria-autocomplete": "list" as const,
        "aria-expanded": expanded,
        "aria-controls": expanded ? listboxId : undefined,
        "aria-activedescendant": expanded && nav.active >= 0 ? optionId(nav.active) : undefined,
      }
    : {};

  return (
    <>
      <input
        ref={inputRef}
        id={id}
        type="text"
        className={className}
        value={value}
        placeholder={placeholder}
        onChange={(e) => {
          syncCaret(e.target);
          onChange(e.target.value);
          nav.openList();
        }}
        onSelect={(e) => syncCaret(e.currentTarget)}
        onKeyDown={handleKeyDown}
        onBlur={nav.close}
        autoCapitalize="off"
        autoCorrect="off"
        autoComplete="off"
        spellCheck={false}
        aria-describedby={describedByIds}
        data-testid={testId}
        {...comboboxProps}
      />
      {expanded && (
        <ul id={listboxId} role="listbox" aria-label="Flag suggestions" className={styles.listbox}>
          {matches.map((flag, i) => (
            <li
              key={flag.name}
              id={optionId(i)}
              role="option"
              aria-selected={i === nav.active}
              aria-label={flag.takesValue ? `${flag.name}, takes a value` : flag.name}
              className={styles.option}
              // Keeps focus in the input so selecting never blurs (and re-probes).
              onMouseDown={(e) => {
                e.preventDefault();
                accept(i);
                nav.close();
              }}
            >
              <span className={styles.optionName}>
                {flag.name}
                {flag.takesValue && <span className={styles.optionValue}> &lt;value&gt;</span>}
              </span>
              {i === nav.active && flag.description && (
                <span id={activeDescId} className={styles.optionDescription}>
                  {flag.description}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
