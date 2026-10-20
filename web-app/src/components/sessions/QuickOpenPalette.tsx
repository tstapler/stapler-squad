"use client";

import React, {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  useCallback,
} from "react";
import { createPortal } from "react-dom";
import type { FileNode } from "@/gen/session/v1/types_pb";
import { searchFiles } from "@/lib/hooks/useFileService";
import { getFileIcon as getFileIconByName } from "@/lib/utils/fileIcons";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import type { LabelsOf } from "@/lib/files/analytics/events";
import * as styles from "./QuickOpenPalette.css";
import { visuallyHidden } from "@/styles/a11y.css";

// ---------------------------------------------------------------------------
// Fuse.js — optional progressive enhancement
// ---------------------------------------------------------------------------
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let FuseClass: any = null;
try {
  // Dynamic require so the bundle never fails when fuse.js is absent.
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  FuseClass = require("fuse.js").default ?? require("fuse.js");
} catch {
  // fuse.js not installed — fall back to raw server results
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getFileIcon(node: { isDir: boolean; name: string }): string {
  if (node.isDir) return "📁";
  return getFileIconByName(node.name);
}

function dirPart(path: string): string {
  const idx = path.lastIndexOf("/");
  return idx > 0 ? path.slice(0, idx) : "";
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type PaletteLabels = LabelsOf<"files.nav.palette">;

/** A command-mode (`>`) entry; `id` is the analytics enum value, so typed text never reaches an event. */
export interface PaletteCommand {
  id: Exclude<PaletteLabels["command"], "none">;
  label: string;
  shortcut?: string;
  /** Listed but not runnable; shown as "<label> (<reason>)". */
  disabledReason?: string;
  run: () => void;
}

interface QuickOpenPaletteProps {
  sessionId: string;
  baseUrl: string;
  recentPaths: string[];
  onSelect: (path: string) => void;
  onClose: () => void;
  /** Enables `>` command mode. */
  commands?: PaletteCommand[];
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function QuickOpenPalette({
  sessionId,
  baseUrl,
  recentPaths,
  onSelect,
  onClose,
  commands,
}: QuickOpenPaletteProps) {
  const [query, setQuery] = useState("");
  const [notice, setNotice] = useState("");
  const analytics = useFilesAnalytics({ component: "QuickOpenPalette", sessionId });
  const commandMode = !!commands && query.startsWith(">");
  const listId = useId();
  const optionId = (i: number) => `${listId}-opt-${i}`;
  const matchedCommands = useMemo(() => {
    if (!commandMode || !commands) return [];
    const needle = query.slice(1).trim().toLowerCase();
    return commands.filter((c) => c.label.toLowerCase().includes(needle));
  }, [commandMode, commands, query]);
  const [results, setResults] = useState<FileNode[]>(() =>
    recentPaths.map((p) => ({
      path: p,
      name: p.split("/").pop() ?? p,
      isDir: false,
      size: BigInt(0),
      gitStatus: "",
      isSymlink: false,
      symlinkTarget: "",
      isIgnored: false,
    } as FileNode))
  );
  const [loading, setLoading] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);

  const inputRef = useRef<HTMLInputElement>(null);
  const prevFocusRef = useRef<Element | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const requestIdRef = useRef(0);
  const resultRefs = useRef<(HTMLDivElement | null)[]>([]);
  const cardRef = useRef<HTMLDivElement>(null);

  // Save and restore focus
  useEffect(() => {
    prevFocusRef.current = document.activeElement;
    inputRef.current?.focus();
    return () => {
      if (
        prevFocusRef.current &&
        typeof (prevFocusRef.current as HTMLElement).focus === "function"
      ) {
        (prevFocusRef.current as HTMLElement).focus();
      }
    };
  }, []);

  // Debounced search
  useEffect(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
    }

    if (commandMode) {
      setActiveIndex(0);
      return;
    }

    if (query === "") {
      const recent: FileNode[] = recentPaths.map((p) => ({
        path: p,
        name: p.split("/").pop() ?? p,
        isDir: false,
        size: BigInt(0),
        gitStatus: "",
        isSymlink: false,
        symlinkTarget: "",
        isIgnored: false,
      } as FileNode));
      setResults(recent);
      setActiveIndex(0);
      return;
    }

    timerRef.current = setTimeout(() => {
      const requestId = ++requestIdRef.current;
      setLoading(true);

      searchFiles(sessionId, query, false, baseUrl)
        .then((resp) => {
          if (requestId !== requestIdRef.current) return;

          let files: FileNode[] = resp.files;

          if (FuseClass && files.length > 0) {
            const fuse = new FuseClass(files, {
              keys: [
                { name: "name", weight: 2 },
                { name: "path", weight: 1 },
              ],
              threshold: 0.4,
              ignoreLocation: true,
            });
            files = fuse.search(query).map(
              (r: { item: FileNode }) => r.item
            );
          }

          setResults(files);
          setActiveIndex(0);
        })
        .catch(() => {
          if (requestId === requestIdRef.current) {
            setResults([]);
          }
        })
        .finally(() => {
          if (requestId === requestIdRef.current) {
            setLoading(false);
          }
        });
    }, 300);

    return () => {
      if (timerRef.current !== null) clearTimeout(timerRef.current);
    };
  }, [query, commandMode, sessionId, baseUrl, recentPaths]);

  // Clear stale DOM refs when the results list changes length
  useEffect(() => {
    resultRefs.current = [];
  }, [results]);

  // Scroll active item into view
  useEffect(() => {
    const el = resultRefs.current[activeIndex];
    if (el && typeof el.scrollIntoView === "function") {
      el.scrollIntoView({ block: "nearest" });
    }
  }, [activeIndex]);

  const mode: PaletteLabels["mode"] = commandMode ? "command" : "file";
  const rowCount = commandMode ? matchedCommands.length : results.length;
  const emit = useCallback(
    (outcome: PaletteLabels["outcome"], command: PaletteLabels["command"] = "none") =>
      analytics.emit("files.nav.palette", { mode, outcome, command }),
    [analytics, mode],
  );
  const cancel = useCallback(() => {
    emit("cancel");
    onClose();
  }, [emit, onClose]);

  const runCommand = useCallback(
    (cmd: PaletteCommand) => {
      if (cmd.disabledReason) {
        setNotice(`${cmd.label} is unavailable: ${cmd.disabledReason}`);
        return;
      }
      emit("run", cmd.id);
      onClose();
      cmd.run();
    },
    [emit, onClose],
  );
  const pickFile = useCallback(
    (path: string) => {
      emit("run");
      onSelect(path);
    },
    [emit, onSelect],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setActiveIndex((i) => (rowCount === 0 ? 0 : (i + 1) % rowCount));
          break;
        case "ArrowUp":
          e.preventDefault();
          setActiveIndex((i) => (rowCount === 0 ? 0 : (i - 1 + rowCount) % rowCount));
          break;
        case "Enter":
          e.preventDefault();
          if (commandMode) {
            const cmd = matchedCommands[activeIndex];
            if (cmd) runCommand(cmd);
            else emit("empty");
          } else if (results[activeIndex]) {
            pickFile(results[activeIndex].path);
          } else {
            emit("empty");
          }
          break;
        case "Escape":
          e.preventDefault();
          cancel();
          break;
      }
    },
    [rowCount, commandMode, matchedCommands, results, activeIndex, runCommand, pickFile, cancel, emit]
  );

  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) {
        cancel();
      }
    },
    [cancel]
  );

  return createPortal(
    <div className={styles.backdrop} onClick={handleBackdropClick}>
      <div
        ref={cardRef}
        className={styles.card}
        onClick={(e) => e.stopPropagation()}
      >
        <input
          ref={inputRef}
          className={styles.searchInput}
          type="text"
          role="combobox"
          aria-expanded="true"
          aria-controls={listId}
          aria-autocomplete="list"
          aria-activedescendant={rowCount > 0 ? optionId(activeIndex) : undefined}
          placeholder={loading ? "Searching…" : commands ? "Go to file… (> for commands)" : "Go to file…"}
          value={query}
          onChange={(e) => { setQuery(e.target.value); setNotice(""); }}
          onKeyDown={handleKeyDown}
          aria-label="Quick open file"
          autoComplete="off"
          spellCheck={false}
        />
        <div data-testid="palette-count" role="status" aria-live="polite" className={visuallyHidden}>
          {notice || (commandMode ? `${matchedCommands.length} commands` : `${results.length} results`)}
        </div>
        <div id={listId} className={styles.resultsList} role="listbox" aria-label={commandMode ? "Commands" : "File results"}>
          {commandMode && matchedCommands.length === 0 && <div className={styles.emptyState}>No commands match</div>}
          {!commandMode && results.length === 0 && !loading && (
            <div className={styles.emptyState}>
              {query ? "No files found" : "No recent files"}
            </div>
          )}
          {commandMode && matchedCommands.map((cmd, i) => (
            <div
              key={cmd.id}
              id={optionId(i)}
              ref={(el) => { resultRefs.current[i] = el; }}
              className={i === activeIndex ? styles.resultItemActive : styles.resultItem}
              role="option"
              aria-selected={i === activeIndex}
              aria-disabled={cmd.disabledReason ? true : undefined}
              onClick={() => runCommand(cmd)}
              onMouseEnter={() => setActiveIndex(i)}
            >
              <span className={styles.resultName}>{cmd.disabledReason ? `${cmd.label} (${cmd.disabledReason})` : cmd.label}</span>
              {cmd.shortcut && <span className={styles.resultPath}>{cmd.shortcut}</span>}
            </div>
          ))}
          {!commandMode && results.map((node, i) => (
            <div
              key={node.path}
              id={optionId(i)}
              ref={(el) => {
                resultRefs.current[i] = el;
              }}
              className={i === activeIndex ? styles.resultItemActive : styles.resultItem}
              role="option"
              aria-selected={i === activeIndex}
              onClick={() => pickFile(node.path)}
              onMouseEnter={() => setActiveIndex(i)}
            >
              <span className={styles.resultIcon}>
                {getFileIcon(node)}
              </span>
              <span className={styles.resultName}>{node.name}</span>
              {dirPart(node.path) && (
                <span className={styles.resultPath} title={node.path}>
                  {dirPart(node.path)}
                </span>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>,
    document.body
  );
}

export default QuickOpenPalette;
