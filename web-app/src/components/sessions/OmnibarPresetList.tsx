// +feature: launcher-presets-list
"use client";

import type { LauncherPresetEntry } from "@/lib/hooks/useLauncherPresets";
import * as styles from "./OmnibarPresetList.css";

interface OmnibarPresetListProps {
  presets: LauncherPresetEntry[];
  loading: boolean;
  loadError: string | null;
  onSelect: (preset: LauncherPresetEntry) => void;
}

// OmnibarPresetList renders the "Presets" section of the creation panel: hand-edited,
// argv-based launch shortcuts from ~/.stapler-squad/launcher-presets.json. Structurally
// mirrors AliasPalette's a11y shape (listbox/option rows, status/alert states).
export function OmnibarPresetList({ presets, loading, loadError, onSelect }: OmnibarPresetListProps) {
  if (loadError) {
    return (
      <div role="alert" aria-live="assertive" className={styles.errorState} data-testid="preset-config-error">
        {loadError}
      </div>
    );
  }

  // Only the first fetch shows a loading state — once any presets have been shown once,
  // subsequent (e.g. Omnibar-reopen) refetches keep the last-known list visible rather than
  // flashing a loading state over already-rendered rows (avoids layout jank on every open).
  if (loading && presets.length === 0) {
    return (
      <div role="status" aria-live="polite" className={styles.loadingState} data-testid="preset-list-loading">
        Loading presets…
      </div>
    );
  }

  if (presets.length === 0) {
    return (
      <div role="status" className={styles.emptyState} data-testid="preset-list-empty">
        No presets yet. Add one in <code>~/.stapler-squad/launcher-presets.json</code>.
      </div>
    );
  }

  return (
    <ul role="listbox" aria-label="Launcher presets" className={styles.list}>
      {presets.map((preset) => (
        <PresetRow key={preset.id} preset={preset} onSelect={onSelect} />
      ))}
    </ul>
  );
}

function PresetRow({ preset, onSelect }: { preset: LauncherPresetEntry; onSelect: (p: LauncherPresetEntry) => void }) {
  // Always derived from argv, never preset.program — program is presentation-only (per the
  // LauncherPresetProto proto comment and ADR-001 §3) and can disagree with what actually
  // launches if a preset author sets it to something other than argv[0]. Previewing argv
  // keeps "what you see is what launches" true even for a malformed/inconsistent preset.
  const argvPreview = preset.argv.join(" ");
  return (
    <li
      role="option"
      aria-selected={false}
      aria-label={`${preset.label} — ${argvPreview} — selecting will replace the current program and extra args`}
      tabIndex={0}
      className={styles.row}
      onClick={() => onSelect(preset)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(preset);
        }
      }}
      data-testid="preset-row"
    >
      <span className={styles.rowLabel}>{preset.label}</span>
      <span className={styles.rowArgv}>{argvPreview}</span>
    </li>
  );
}
