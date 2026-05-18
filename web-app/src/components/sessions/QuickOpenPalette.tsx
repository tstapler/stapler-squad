"use client";

import React, {
  useEffect,
  useRef,
  useState,
  useCallback,
} from "react";
import { createPortal } from "react-dom";
import type { FileNode } from "@/gen/session/v1/types_pb";
import { searchFiles } from "@/lib/hooks/useFileService";
import { getFileIcon as getFileIconByName } from "@/lib/utils/fileIcons";
import * as styles from "./QuickOpenPalette.css";

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

interface QuickOpenPaletteProps {
  sessionId: string;
  baseUrl: string;
  recentPaths: string[];
  onSelect: (path: string) => void;
  onClose: () => void;
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
}: QuickOpenPaletteProps) {
  const [query, setQuery] = useState("");
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
  }, [query, sessionId, baseUrl, recentPaths]);

  // Scroll active item into view
  useEffect(() => {
    resultRefs.current[activeIndex]?.scrollIntoView({
      block: "nearest",
    });
  }, [activeIndex]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setActiveIndex((i) => (results.length === 0 ? 0 : (i + 1) % results.length));
          break;
        case "ArrowUp":
          e.preventDefault();
          setActiveIndex((i) =>
            results.length === 0 ? 0 : (i - 1 + results.length) % results.length
          );
          break;
        case "Enter":
          e.preventDefault();
          if (results[activeIndex]) {
            onSelect(results[activeIndex].path);
          }
          break;
        case "Escape":
          e.preventDefault();
          onClose();
          break;
      }
    },
    [results, activeIndex, onSelect, onClose]
  );

  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) {
        onClose();
      }
    },
    [onClose]
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
          placeholder={loading ? "Searching…" : "Go to file…"}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          aria-label="Quick open file"
          autoComplete="off"
          spellCheck={false}
        />
        <div className={styles.resultsList} role="listbox" aria-label="File results">
          {results.length === 0 && !loading && (
            <div className={styles.emptyState}>
              {query ? "No files found" : "No recent files"}
            </div>
          )}
          {results.map((node, i) => (
            <div
              key={node.path}
              ref={(el) => {
                resultRefs.current[i] = el;
              }}
              className={i === activeIndex ? styles.resultItemActive : styles.resultItem}
              role="option"
              aria-selected={i === activeIndex}
              onClick={() => onSelect(node.path)}
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
