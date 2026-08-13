"use client";

import { registry, ShortcutContext, Shortcut } from "@/lib/shortcuts/shortcutRegistry";
import { Kbd } from "@/components/ui/Kbd";
import * as styles from "./settings.css";

const CONTEXT_LABELS: Record<ShortcutContext, string> = {
  global: "Global",
  "session-list": "Session List",
  approval: "Approval Review",
  terminal: "Terminal",
  cockpit: "Cockpit / Panes",
  omnibar: "Omnibar",
};

const CONTEXTS: ShortcutContext[] = ["global", "session-list", "approval", "terminal", "cockpit", "omnibar"];

function formatKey(s: Shortcut): string {
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

export function KeyboardShortcutsTab() {
  const allShortcuts = registry.getAll();

  return (
    <div className={styles.shortcutsTable}>
      {CONTEXTS.map((ctx) => {
        const shortcuts = allShortcuts[ctx];
        if (!shortcuts || shortcuts.length === 0) return null;
        return (
          <section key={ctx} className={styles.shortcutsContextSection}>
            <h3 className={styles.shortcutsContextHeading}>{CONTEXT_LABELS[ctx]}</h3>
            {shortcuts.map((s) => (
              <div key={s.label} className={styles.shortcutRow}>
                <span className={styles.shortcutLabel}>{s.label}</span>
                <Kbd size="sm">{formatKey(s)}</Kbd>
              </div>
            ))}
          </section>
        );
      })}
    </div>
  );
}
