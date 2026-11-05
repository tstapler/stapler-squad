"use client";

import { useCallback, useEffect, useState } from "react";
import { registry, ShortcutContext, Shortcut } from "@/lib/shortcuts/shortcutRegistry";
import {
  chordFromEvent,
  formatLeader,
  getLeaderConfig,
  isSafeChord,
  LEADER_CONFIG_EVENT,
  resetLeaderConfig,
  setLeaderConfig,
} from "@/lib/shortcuts/leaderConfig";
import { commandBarController } from "@/lib/shortcuts/commandBarController";
import { Kbd } from "@/components/ui/Kbd";
import * as styles from "./settings.css";

const CONTEXT_LABELS: Record<ShortcutContext, string> = {
  global: "Global",
  "session-list": "Session List",
  approval: "Approval Review",
  terminal: "Terminal",
  cockpit: "Cockpit / Panes",
  omnibar: "Omnibar",
  diff: "Diff View",
};

const CONTEXTS: ShortcutContext[] = ["global", "session-list", "approval", "terminal", "cockpit", "omnibar", "diff"];

function formatKey(s: Shortcut): string {
  if (s.sequence && s.sequence.length > 0 && s.key === undefined) return `Leader ${s.sequence.join(" ")}`;
  return [
    s.modifiers?.meta && "⌘",
    s.modifiers?.ctrl && "Ctrl",
    s.modifiers?.shift && "⇧",
    s.modifiers?.alt && "⌥",
    s.key,
  ]
    .filter(Boolean)
    .join("+");
}

/** Leader chord picker: record a chord, reset to default, or opt out ("no safe chord"). */
function LeaderKeySetting() {
  const [config, setConfig] = useState(getLeaderConfig);
  const [recording, setRecording] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const sync = () => setConfig(getLeaderConfig());
    window.addEventListener(LEADER_CONFIG_EVENT, sync);
    return () => window.removeEventListener(LEADER_CONFIG_EVENT, sync);
  }, []);

  useEffect(() => {
    if (!recording) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (["Alt", "Control", "Shift", "Meta"].includes(e.key)) return; // wait for the real key
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
      if (e.key === "Escape") {
        setRecording(false);
        return;
      }
      const chord = chordFromEvent(e);
      if (!isSafeChord(chord)) {
        setError("Hold Alt or Meta with a key. Ctrl-only and bare keys would collide with terminal input.");
        return;
      }
      setLeaderConfig({ enabled: true, chord });
      setError(null);
      setRecording(false);
    };
    // window capture runs before the command bar's document capture listener, so
    // recording the current leader chord does not just open the bar.
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [recording]);

  const openBar = useCallback(() => commandBarController.open([]), []);

  return (
    <section className={styles.shortcutsContextSection} data-testid="leader-key-setting">
      <h3 className={styles.shortcutsContextHeading}>Command bar leader key</h3>
      <div className={styles.shortcutRow}>
        <span className={styles.shortcutLabel}>Current leader</span>
        <Kbd size="sm">{formatLeader(config)}</Kbd>
      </div>
      <p role={error ? "alert" : undefined}>
        {error ?? (recording ? "Press the new chord (Esc cancels)…" : "The leader opens the command bar; the terminal never sees it.")}
      </p>
      <div>
        <button type="button" onClick={() => { setError(null); setRecording((r) => !r); }}>
          {recording ? "Cancel" : "Change leader key"}
        </button>{" "}
        <button type="button" onClick={() => { resetLeaderConfig(); setError(null); }}>Reset to default</button>{" "}
        <button type="button" onClick={() => setLeaderConfig({ enabled: false })}>No safe chord: disable leader</button>{" "}
        <button type="button" onClick={openBar} data-testid="open-command-bar">Open command bar</button>
      </div>
    </section>
  );
}

export function KeyboardShortcutsTab() {
  const allShortcuts = registry.getAll();

  return (
    <div className={styles.shortcutsTable}>
      <LeaderKeySetting />
      {CONTEXTS.map((ctx) => {
        const shortcuts = allShortcuts[ctx];
        if (!shortcuts || shortcuts.length === 0) return null;
        return (
          <section key={ctx} className={styles.shortcutsContextSection}>
            <h3 className={styles.shortcutsContextHeading}>{CONTEXT_LABELS[ctx]}</h3>
            {shortcuts.map((s) => (
              <div key={`${s.label}-${s.sequence?.join("") ?? s.key}`} className={styles.shortcutRow}>
                <span className={styles.shortcutLabel}>{s.label}</span>
                <span>
                  {s.sequence && s.key !== undefined && <Kbd size="sm">{`Leader ${s.sequence.join(" ")}`}</Kbd>}{" "}
                  <Kbd size="sm">{formatKey(s)}</Kbd>
                </span>
              </div>
            ))}
          </section>
        );
      })}
    </div>
  );
}
