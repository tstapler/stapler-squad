"use client";

import React from "react";
import type { AliasEntry } from "@/lib/hooks/useAliases";
import * as styles from "./AliasPalette.css";

interface AliasPaletteProps {
  aliases: AliasEntry[];
  input: string;
  selectedIndex: number;
  onSelect: (alias: AliasEntry) => void;
  error?: Error | null;
}

export function AliasPalette({ aliases, input, selectedIndex, onSelect, error }: AliasPaletteProps) {
  const partial = input.startsWith("@") ? input.slice(1).toLowerCase() : "";

  const filtered = partial
    ? aliases.filter(
        (a) =>
          a.name.toLowerCase().includes(partial) ||
          a.description.toLowerCase().includes(partial) ||
          a.group.toLowerCase().includes(partial)
      )
    : aliases;

  const isFiltering = partial.length > 0;

  if (error) {
    return (
      <div className={styles.palette} data-testid="alias-palette">
        <div role="alert" aria-live="assertive" className={styles.errorState} data-testid="alias-config-error">
          <span className={styles.errorIcon}>⚠</span>
          <div>
            <div className={styles.errorTitle}>Alias config failed to load</div>
            <div className={styles.errorDetail}>{error.message}</div>
          </div>
        </div>
      </div>
    );
  }

  if (filtered.length === 0) {
    return (
      <div className={styles.palette} data-testid="alias-palette">
        <div className={styles.emptyState} data-testid="alias-palette-empty" role="status">
          <div className={styles.emptyTitle}>No aliases yet</div>
          <div className={styles.emptyBody}>
            Add aliases in Settings → Aliases to launch sessions faster.
          </div>
        </div>
      </div>
    );
  }

  if (isFiltering) {
    const activeFiltered = filtered[selectedIndex];
    return (
      <div className={styles.palette} data-testid="alias-palette">
        <ul
          role="listbox"
          aria-label="Alias matches"
          aria-activedescendant={activeFiltered ? `alias-option-${activeFiltered.name}` : undefined}
          className={styles.list}
        >
          {filtered.map((alias, i) => (
            <AliasRow
              key={alias.name}
              alias={alias}
              isSelected={i === selectedIndex}
              onSelect={onSelect}
            />
          ))}
        </ul>
      </div>
    );
  }

  const grouped: { group: string; items: AliasEntry[] }[] = [];
  const ungrouped: AliasEntry[] = [];
  for (const alias of filtered) {
    if (!alias.group) {
      ungrouped.push(alias);
    } else {
      const existing = grouped.find((g) => g.group === alias.group);
      if (existing) {
        existing.items.push(alias);
      } else {
        grouped.push({ group: alias.group, items: [alias] });
      }
    }
  }

  // Build a flat ordered list so selection index is derived explicitly, not via a mutable counter.
  const flatList = [...ungrouped, ...grouped.flatMap((g) => g.items)];
  const activeAlias = flatList[selectedIndex];

  return (
    <div className={styles.palette} data-testid="alias-palette">
      <ul
        role="listbox"
        aria-label="Alias palette"
        aria-activedescendant={activeAlias ? `alias-option-${activeAlias.name}` : undefined}
        className={styles.list}
      >
        {ungrouped.map((alias) => (
          <AliasRow
            key={alias.name}
            alias={alias}
            isSelected={flatList.indexOf(alias) === selectedIndex}
            onSelect={onSelect}
          />
        ))}
        {grouped.map(({ group, items }) => (
          <React.Fragment key={group}>
            <li role="presentation" className={styles.groupHeader} data-testid="alias-group-header">
              {group.toUpperCase()}
            </li>
            {items.map((alias) => (
              <AliasRow
                key={alias.name}
                alias={alias}
                isSelected={flatList.indexOf(alias) === selectedIndex}
                onSelect={onSelect}
              />
            ))}
          </React.Fragment>
        ))}
      </ul>
    </div>
  );
}

function AliasRow({ alias, isSelected, onSelect }: { alias: AliasEntry; isSelected: boolean; onSelect: (a: AliasEntry) => void }) {
  return (
    <li
      id={`alias-option-${alias.name}`}
      role="option"
      aria-selected={isSelected}
      aria-label={`@${alias.name}${alias.description ? ` — ${alias.description}` : ""}${alias.path ? `, path ${alias.path}` : ""}${alias.program ? `, program ${alias.program}` : ""}`}
      tabIndex={isSelected ? 0 : -1}
      className={isSelected ? styles.rowSelected : styles.row}
      onClick={() => onSelect(alias)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(alias);
        }
      }}
      data-testid="alias-row"
    >
      <span className={styles.rowName}>@{alias.name}</span>
      {alias.description && <span className={styles.rowDesc}>{alias.description}</span>}
      <span className={styles.rowMeta}>
        {alias.path && <span>{alias.path}</span>}
        {alias.program && <span className={styles.rowProgram}>[{alias.program}]</span>}
      </span>
    </li>
  );
}
