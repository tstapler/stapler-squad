"use client";

import { useRef, useEffect, useCallback, useState } from "react";
import { useDatabases } from "@/lib/hooks/useDatabase";
import { DatabaseInfo } from "@/gen/session/v1/types_pb";
import styles from "./WorkspaceSwitcher.module.css";

/**
 * WorkspaceSwitcher — header dropdown for switching between workspace databases.
 *
 * Shows the current workspace name as a button. Clicking opens a dropdown
 * listing all discovered workspaces with two actions per non-current row:
 *   • Click row  → Switch (server restarts, page reloads)
 *   • "Merge" button → Copy sessions from that workspace into current one
 */
export function WorkspaceSwitcher() {
  const { databases, currentId, switching, merging, error, switchDatabase, mergeDatabase } = useDatabases();
  const [isOpen, setIsOpen] = useState(false);
  const [pendingDir, setPendingDir] = useState("");
  const [mergeToast, setMergeToast] = useState<string | null>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: MouseEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [isOpen]);

  // Close on Escape
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setIsOpen(false);
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [isOpen]);

  // Auto-dismiss merge toast after 4s
  useEffect(() => {
    if (!mergeToast) return;
    const t = setTimeout(() => setMergeToast(null), 4000);
    return () => clearTimeout(t);
  }, [mergeToast]);

  const current = databases.find((d) => d.workspaceId === currentId);
  const currentName = current?.name ?? (currentId ? currentId.slice(0, 8) : "...");

  const handleSwitch = useCallback(
    async (db: DatabaseInfo) => {
      if (db.isCurrent || switching || merging) return;
      setIsOpen(false);
      setPendingDir(db.configDir);
      await switchDatabase(db.configDir);
    },
    [switching, merging, switchDatabase]
  );

  const handleMerge = useCallback(
    async (e: React.MouseEvent, db: DatabaseInfo) => {
      e.stopPropagation(); // don't trigger the switch click on the row
      if (db.isCurrent || switching || merging) return;
      setPendingDir(db.configDir);
      try {
        const result = await mergeDatabase(db.configDir);
        setMergeToast(
          result.sessionsImported > 0
            ? `Merged ${result.sessionsImported} session${result.sessionsImported !== 1 ? "s" : ""} from "${db.name}"`
            : `Nothing new to merge from "${db.name}" (${result.sessionsSkipped} already present)`
        );
        // Reload so the session list reflects the new sessions
        if (result.sessionsImported > 0) {
          setTimeout(() => window.location.reload(), 800);
        }
      } finally {
        setPendingDir("");
      }
    },
    [switching, merging, mergeDatabase]
  );

  if (switching) {
    return (
      <div className={styles.switching}>
        <div className={styles.spinner} />
        Switching...
      </div>
    );
  }

  return (
    <>
      <div className={styles.wrapper} ref={wrapperRef}>
        <button
          className={styles.trigger}
          onClick={() => setIsOpen((v) => !v)}
          aria-label={`Current workspace: ${currentName}. Click to switch workspace.`}
          title="Switch workspace"
          aria-expanded={isOpen}
          aria-haspopup="listbox"
        >
          <span className={styles.triggerName}>{currentName}</span>
          <svg
            className={`${styles.chevron} ${isOpen ? styles.chevronOpen : ""}`}
            aria-hidden="true"
            width="10"
            height="10"
            viewBox="0 0 10 10"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <polyline points="2,3 5,7 8,3" />
          </svg>
        </button>

        {isOpen && (
          <div className={styles.dropdown} role="listbox" aria-label="Available workspaces">
            <div className={styles.dropdownHeader}>Workspaces</div>
            <ul className={styles.dropdownList}>
              {databases.map((db) => {
                const isRowBusy = pendingDir === db.configDir && (switching || merging);
                return (
                  <li key={db.configDir}>
                    <div
                      className={`${styles.workspaceItem} ${db.isCurrent ? styles.workspaceItemCurrent : ""} ${isRowBusy ? styles.workspaceItemLoading : ""}`}
                      title={db.cwd || db.configDir}
                    >
                      {/* Left: icon + info — clicking switches workspace */}
                      <button
                        role="option"
                        aria-selected={db.isCurrent}
                        className={styles.workspaceItemMain}
                        onClick={() => handleSwitch(db)}
                        disabled={db.isCurrent || switching || merging}
                      >
                        <span className={`${styles.workspaceIcon} ${db.isCurrent ? styles.workspaceIconCurrent : ""}`}>
                          {isRowBusy ? (
                            <span className={styles.spinner} />
                          ) : db.isCurrent ? (
                            <CheckIcon />
                          ) : (
                            <DatabaseIcon />
                          )}
                        </span>
                        <span className={styles.workspaceInfo}>
                          <span className={`${styles.workspaceName} ${db.isCurrent ? styles.workspaceNameCurrent : ""}`}>
                            {db.name}
                          </span>
                          {db.cwd && (
                            <span className={styles.workspacePath} title={db.cwd}>
                              {abbreviatePath(db.cwd)}
                            </span>
                          )}
                        </span>
                        {db.sessionCount > 0 && (
                          <span className={styles.workspaceMeta}>{db.sessionCount}</span>
                        )}
                      </button>

                      {/* Right: Merge button (only on non-current workspaces) */}
                      {!db.isCurrent && (
                        <button
                          className={styles.mergeButton}
                          onClick={(e) => handleMerge(e, db)}
                          disabled={switching || merging}
                          title={`Merge sessions from "${db.name}" into current workspace`}
                          aria-label={`Merge sessions from ${db.name}`}
                        >
                          {isRowBusy && merging ? (
                            <span className={styles.spinner} />
                          ) : (
                            "Merge"
                          )}
                        </button>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
            {error && <div className={styles.errorBanner}>{error}</div>}
          </div>
        )}
      </div>

      {/* Merge result toast */}
      {mergeToast && (
        <div className={styles.mergeToast} role="status">
          {mergeToast}
        </div>
      )}
    </>
  );
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/** Abbreviate a long path for display, e.g. /Users/me/projects/foo → ~/projects/foo */
function abbreviatePath(p: string): string {
  const abbreviated = p.replace(/^\/(?:home|Users)\/[^/]+/, "~");
  if (abbreviated.length <= 36) return abbreviated;
  const tail = abbreviated.split("/").slice(-2).join("/");
  return "…/" + tail;
}

// ── Icon components ───────────────────────────────────────────────────────────

function CheckIcon() {
  return (
    <svg aria-hidden="true" width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="2,6 5,9 10,3" />
    </svg>
  );
}

function DatabaseIcon() {
  return (
    <svg aria-hidden="true" width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <ellipse cx="6" cy="3" rx="4" ry="1.5" />
      <path d="M2 3v2c0 .83 1.79 1.5 4 1.5S10 5.83 10 5V3" />
      <path d="M2 5v2c0 .83 1.79 1.5 4 1.5S10 7.83 10 7V5" />
      <path d="M2 7v2c0 .83 1.79 1.5 4 1.5S10 9.83 10 9V7" />
    </svg>
  );
}
