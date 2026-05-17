"use client";

import React from "react";
import { container, icon, headline, body, hint, kbd } from "./SessionListEmptyState.css";

export function SessionListEmptyState() {
  return (
    <div className={container} data-testid="session-list-empty-state">
      <span className={icon} aria-hidden="true">⌘</span>
      <p className={headline}>No sessions yet</p>
      <p className={body}>
        Press <kbd className={kbd}>⌘K</kbd> or <kbd className={kbd}>Ctrl+K</kbd> to create your first session
      </p>
      <p className={hint}>or paste a GitHub URL</p>
    </div>
  );
}
