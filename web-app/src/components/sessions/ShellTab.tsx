"use client";
// +feature: shell-tabs

import { useCallback } from "react";
import type { ShellTab as ShellTabData } from "@/lib/hooks/useShells";
import * as styles from "./ShellTab.css";

interface ShellTabProps {
  shell: ShellTabData;
  onStop: (shellId: string) => void;
  onRestart: (shellId: string) => void;
  onClose: (shellId: string) => void;
}

export function ShellTabLabel({ shell, onStop, onRestart, onClose }: ShellTabProps) {
  const handleStop = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    onStop(shell.id);
  }, [onStop, shell.id]);

  const handleRestart = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    onRestart(shell.id);
  }, [onRestart, shell.id]);

  const handleClose = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    onClose(shell.id);
  }, [onClose, shell.id]);

  const hasError = shell.status === "error" || (shell.status !== "running" && shell.exitCode != null && shell.exitCode !== 0);

  return (
    <div className={styles.tabLabel}>
      <span
        className={styles.statusDot[shell.status]}
        title={shell.status}
        aria-label={`Shell status: ${shell.status}`}
      />
      <span className={styles.tabName} title={shell.name || shell.command || "shell"}>
        {shell.name || shell.command || "shell"}
      </span>
      {hasError && (
        <span
          className={styles.errorIndicator}
          title={shell.exitCode != null ? `Exited with code ${shell.exitCode}` : "Shell errored"}
          aria-label={shell.exitCode != null ? `Exit code ${shell.exitCode}` : "Shell errored"}
        >
          !
        </span>
      )}
      <div className={styles.actions}>
        {shell.status === "running" && (
          <button
            className={styles.actionButton}
            onClick={handleStop}
            title="Stop shell"
            aria-label={`Stop shell ${shell.name}`}
          >
            ■
          </button>
        )}
        {shell.status !== "running" && (
          <button
            className={styles.actionButton}
            onClick={handleRestart}
            title="Restart shell"
            aria-label={`Restart shell ${shell.name}`}
          >
            ↺
          </button>
        )}
        <button
          className={styles.actionButton}
          onClick={handleClose}
          title="Delete shell"
          aria-label={`Delete shell ${shell.name}`}
        >
          ×
        </button>
      </div>
    </div>
  );
}
