"use client";

import { useState } from "react";
import {
  PINNED_COLLAPSE_CHOICES_MS,
  UNDO_WINDOW_CHOICES_MS,
  readPinnedCollapseMs,
  readUndoWindowMs,
  writePinnedCollapseMs,
  writeUndoWindowMs,
} from "@/lib/utils/deckSettings";
import { trayButton, trayConfirm, traySelect, traySettings, traySettingsRow } from "./NotificationPanel.css";

export const WHAT_CHANGED_TEXT =
  "What changed: routine events from hidden sessions no longer notify, toasts are capped and the rest wait here, and Move all to tray never deletes anything. Quiet mode in this header sends every non-urgent toast straight here.";

const seconds = (ms: number) => `${ms / 1000}s`;

/** Per-device "Undo window" and "Collapse pinned cards after" choices, persisted with try/catch. */
export function TraySettings() {
  const [undoMs, setUndoMs] = useState(readUndoWindowMs);
  const [collapseMs, setCollapseMs] = useState<number | null>(readPinnedCollapseMs);

  return (
    <div className={traySettings} data-testid="tray-settings">
      <label className={traySettingsRow}>
        Undo window
        <select
          className={traySelect}
          value={undoMs}
          onChange={(e) => {
            const ms = Number(e.target.value);
            setUndoMs(ms);
            writeUndoWindowMs(ms);
          }}
        >
          {UNDO_WINDOW_CHOICES_MS.map((ms) => (
            <option key={ms} value={ms}>
              {seconds(ms)}
            </option>
          ))}
        </select>
      </label>
      <label className={traySettingsRow}>
        Collapse pinned cards after
        <select
          className={traySelect}
          value={collapseMs === null ? "never" : collapseMs}
          onChange={(e) => {
            const ms = e.target.value === "never" ? null : Number(e.target.value);
            setCollapseMs(ms);
            writePinnedCollapseMs(ms);
          }}
        >
          {PINNED_COLLAPSE_CHOICES_MS.map((ms) => (
            <option key={ms} value={ms}>
              {seconds(ms)}
            </option>
          ))}
          <option value="never">Never</option>
        </select>
      </label>
    </div>
  );
}

/** The one-time "What changed" card. It has no live role: the Announcer speaks it once, politely. */
export function WhatChangedCard({ onDismiss }: { onDismiss: () => void }) {
  return (
    <div className={trayConfirm} data-testid="tray-what-changed">
      <span>{WHAT_CHANGED_TEXT}</span>
      <div>
        <button type="button" className={trayButton} data-testid="tray-what-changed-dismiss" onClick={onDismiss}>
          Got it
        </button>
      </div>
    </div>
  );
}
