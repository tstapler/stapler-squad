"use client";

import { useState, useCallback, useRef, useEffect, useMemo, useId } from "react";
import { usePathCompletions } from "@/lib/hooks/usePathCompletions";
import { useSessionRepoPaths } from "@/lib/hooks/useSessionRepoPaths";
import { PathCompletionDropdown, type CompletionEntry } from "@/components/ui/PathCompletionDropdown";
import { isGitHubRef, parseGitHubRef, getRepoFullName } from "@/lib/github/urlParser";
import * as styles from "./RepoPathInput.css";

interface RepoPathInputProps {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  error?: string;
  placeholder?: string;
  required?: boolean;
  /** Optional helper text rendered under the input, e.g. explaining the expected format. */
  hint?: string;
  /**
   * When true, live-detect a GitHub URL/shorthand in the value and render an
   * inline confirmation of what will happen on save (clone to a local path),
   * instead of leaving the user to guess whether a pasted URL is supported.
   */
  detectGitHubUrl?: boolean;
  "data-testid"?: string;
}

const MAX_HISTORY = 5;

function tildeAbbreviate(p: string): string {
  const m = p.match(/^(\/(?:Users|home)\/[^/]+)(\/.*)?$/);
  return m ? `~${m[2] ?? ""}` : p;
}

export function RepoPathInput({
  id: idProp,
  value,
  onChange,
  disabled = false,
  error,
  placeholder = "/path/to/repo",
  required = false,
  hint,
  detectGitHubUrl = false,
  "data-testid": testId,
}: RepoPathInputProps) {
  const generatedId = useId();
  const id = idProp ?? generatedId;
  const listboxId = `${id}-listbox`;

  const [open, setOpen] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement>(null);

  const historyPaths = useSessionRepoPaths();
  const { entries: fsEntries, isLoading } = usePathCompletions(value, {
    enabled: value.length > 0,
    directoriesOnly: true,
  });

  const detectedRepo = useMemo(() => {
    if (!detectGitHubUrl || !value.trim() || !isGitHubRef(value)) return null;
    return parseGitHubRef(value);
  }, [detectGitHubUrl, value]);

  const { allEntries, historyCount } = useMemo(() => {
    const filtered = historyPaths.filter(
      (p) => value === "" || p.toLowerCase().includes(value.toLowerCase())
    );
    const history = filtered.slice(0, MAX_HISTORY);
    const historySet = new Set(history);

    const historyEntries: CompletionEntry[] = history.map((p) => ({
      name: tildeAbbreviate(p),
      path: p,
      isDirectory: true,
      isHistory: true,
    }));

    const fsCompletionEntries: CompletionEntry[] = fsEntries
      .filter((e) => !historySet.has(e.path))
      .map((e) => ({
        name: e.name,
        path: e.path,
        isDirectory: e.isDirectory,
        isHistory: false,
      }));

    return {
      allEntries: [...historyEntries, ...fsCompletionEntries],
      historyCount: historyEntries.length,
    };
  }, [historyPaths, fsEntries, value]);

  const showDropdown = open && (allEntries.length > 0 || isLoading);

  const handleSelect = useCallback(
    (entry: CompletionEntry) => {
      onChange(entry.path);
      setOpen(false);
      setSelectedIndex(-1);
    },
    [onChange]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (!open) {
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          setOpen(true);
        }
        return;
      }
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setSelectedIndex((i) => Math.min(i + 1, allEntries.length - 1));
          break;
        case "ArrowUp":
          e.preventDefault();
          setSelectedIndex((i) => Math.max(i - 1, -1));
          break;
        case "Enter":
          if (selectedIndex >= 0 && selectedIndex < allEntries.length) {
            e.preventDefault();
            handleSelect(allEntries[selectedIndex]);
          }
          break;
        case "Escape":
          setOpen(false);
          setSelectedIndex(-1);
          break;
      }
    },
    [open, allEntries, selectedIndex, handleSelect]
  );

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
        setSelectedIndex(-1);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  return (
    <div ref={containerRef} className={styles.container}>
      <input
        id={id}
        type="text"
        className={[styles.input, error ? styles.inputError : ""].filter(Boolean).join(" ")}
        value={value}
        onChange={(e) => {
          onChange(e.target.value);
          setOpen(true);
          setSelectedIndex(-1);
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        title={value || undefined}
        required={required}
        aria-required={required || undefined}
        aria-invalid={error ? true : undefined}
        aria-autocomplete="list"
        aria-controls={showDropdown ? listboxId : undefined}
        aria-activedescendant={
          showDropdown && selectedIndex >= 0
            ? `${listboxId}-option-${selectedIndex}`
            : undefined
        }
        aria-describedby={error ? `${id}-error` : undefined}
        disabled={disabled}
        data-testid={testId}
        autoComplete="off"
        spellCheck={false}
      />
      {showDropdown && (
        <div className={styles.dropdownWrapper}>
          <PathCompletionDropdown
            id={listboxId}
            entries={allEntries}
            selectedIndex={selectedIndex}
            onSelect={handleSelect}
            isLoading={isLoading}
            historyCount={historyCount}
          />
        </div>
      )}
      {detectedRepo ? (
        <span className={styles.githubHint} data-testid="repo-path-github-hint">
          Will clone {getRepoFullName(detectedRepo)} to{" "}
          {`~/.stapler-squad/repos/github.com/${detectedRepo.owner}/${detectedRepo.repo}`} when
          you save.
        </span>
      ) : (
        hint && <span className={styles.hint}>{hint}</span>
      )}
    </div>
  );
}
