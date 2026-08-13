"use client";

import {
  dropdown, item, itemSelected, icon as iconClass, name as nameClass,
  suffix, itemHistory, divider, loading as loadingClass,
} from "./PathCompletionDropdown.css";

/** A single item shown in the path completion dropdown. */
export interface CompletionEntry {
  name: string;
  path: string;
  isDirectory: boolean;
  /** True for entries sourced from local history (not the live filesystem). */
  isHistory?: boolean;
}

interface PathCompletionDropdownProps {
  entries: CompletionEntry[];
  selectedIndex: number;
  onSelect: (entry: CompletionEntry) => void;
  isLoading: boolean;
  /** How many leading entries are history entries (renders a divider after them). */
  historyCount?: number;
  id?: string;
}

function EntryItem({
  entry,
  index,
  selectedIndex,
  onSelect,
  id,
}: {
  entry: CompletionEntry;
  index: number;
  selectedIndex: number;
  onSelect: (entry: CompletionEntry) => void;
  id: string;
}) {
  return (
    <li
      id={`${id}-option-${index}`}
      className={[
        item,
        index === selectedIndex ? itemSelected : "",
        entry.isHistory ? itemHistory : "",
      ]
        .filter(Boolean)
        .join(" ")}
      role="option"
      aria-selected={index === selectedIndex}
      title={entry.path}
      onMouseDown={(e) => {
        e.preventDefault();
        onSelect(entry);
      }}
    >
      <span className={iconClass} aria-hidden="true">
        {entry.isHistory ? "🕒" : entry.isDirectory ? "📁" : "📄"}
      </span>
      <span className={nameClass}>{entry.name}</span>
      {entry.isDirectory && !entry.isHistory && (
        <span className={suffix} aria-hidden="true">
          /
        </span>
      )}
    </li>
  );
}

export function PathCompletionDropdown({
  entries,
  selectedIndex,
  onSelect,
  isLoading,
  historyCount = 0,
  id = "path-completion-listbox",
}: PathCompletionDropdownProps) {
  if (isLoading && entries.length === 0) {
    return <div className={loadingClass}>Loading completions…</div>;
  }
  if (entries.length === 0) return null;

  const showDivider = historyCount > 0 && historyCount < entries.length;

  return (
    <ul
      id={id}
      className={dropdown}
      role="listbox"
      aria-label="Path completions"
      data-testid="path-completion-dropdown"
    >
      {entries.slice(0, historyCount).map((entry, i) => (
        <EntryItem
          key={entry.path}
          entry={entry}
          index={i}
          selectedIndex={selectedIndex}
          onSelect={onSelect}
          id={id}
        />
      ))}
      {showDivider && (
        <li className={divider} role="presentation" aria-hidden="true" />
      )}
      {entries.slice(historyCount).map((entry, i) => (
        <EntryItem
          key={entry.path}
          entry={entry}
          index={historyCount + i}
          selectedIndex={selectedIndex}
          onSelect={onSelect}
          id={id}
        />
      ))}
    </ul>
  );
}
