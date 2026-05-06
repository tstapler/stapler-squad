"use client";

import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { useRouter } from "next/navigation";
import Fuse from "fuse.js";
import { detect, InputType, INPUT_TYPE_INFO, DetectionResult } from "@/lib/omnibar";
import { useModeReducer, OmnibarModeState } from "@/lib/omnibar/modes/useModeReducer";
import { PROGRAMS } from "@/lib/constants/programs";
import { getApiBaseUrl } from "@/lib/config";
import { useTheme } from "@/lib/contexts/ThemeContext";
import type { ThemeName } from "@/lib/contexts/ThemeContext";
import { usePathCompletions } from "@/lib/hooks/usePathCompletions";
import { usePathHistory } from "@/lib/hooks/usePathHistory";
import { useWorktreeSuggestions } from "@/lib/hooks/useWorktreeSuggestions";
import { useSessionSearch, type SessionSearchResult } from "@/lib/hooks/useSessionSearch";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { PathCompletionDropdown, type CompletionEntry } from "./PathCompletionDropdown";
import { OmnibarResultList, getResultListItemCount, getHighlightedItemId } from "./OmnibarResultList";
import { OmnibarModeBadge } from "./OmnibarModeBadge";
import { OmnibarCreationPanel, SESSION_TYPES } from "./OmnibarCreationPanel";
import { parseSlashCommand } from "@/lib/omnibar/parseSlashCommand";
import { parseInputWithSeparator } from "@/lib/omnibar/parseInput";
import {
  overlay, modal, inputContainer, typeIndicator, input as inputClass,
  detectionInfo, detectionBadge, unknown,
  shortcuts, shortcut, shortcutKey, completionError as completionErrorClass,
  pathIndicator, pathIndicatorValid, pathIndicatorInvalid, pathIndicatorLoading,
} from "./Omnibar.css";

interface OmnibarProps {
  isOpen: boolean;
  onClose: () => void;
  onCreateSession: (data: OmnibarSessionData) => Promise<void>;
  onNavigateToSession: (sessionId: string) => void;
  initialMode?: "discovery" | "creation";
}

// Consolidated form state
export interface OmnibarFormState {
  sessionName: string;
  branch: string;
  program: string;
  category: string;
  autoYes: boolean;
  useTitleAsBranch: boolean;
  sessionType: "directory" | "new_worktree" | "existing_worktree" | "one_off" | "new_project";
  existingWorktree: string;
  workingDir: string;
  // New project mode fields
  parentDir: string;
  projectName: string;
  newProjectSessionType: "directory" | "new_worktree";
  // Opt-in: when the path doesn't exist yet, create the directory and
  // initialize a new git repository. Only applies to directory / new_worktree.
  createIfMissing: boolean;
  firstPrompt: string;
}

const INITIAL_FORM_STATE: OmnibarFormState = {
  sessionName: "",
  branch: "",
  program: "",
  category: "",
  autoYes: false,
  useTitleAsBranch: true,
  sessionType: "new_worktree",
  existingWorktree: "",
  workingDir: "",
  // New project mode defaults
  parentDir: "",
  projectName: "",
  newProjectSessionType: "new_worktree",
  createIfMissing: false,
  firstPrompt: "",
};

// Consolidated UI state
interface OmnibarUIState {
  showAdvanced: boolean;
  dropdownIndex: number;
  dropdownDismissed: boolean;
  resultHighlightIndex: number;
}

export interface OmnibarSessionData {
  title: string;
  path: string;
  branch?: string;
  program: string;
  category?: string;
  prompt?: string;
  autoYes: boolean;
  // GitHub-specific
  gitHubOwner?: string;
  gitHubRepo?: string;
  gitHubPRNumber?: number;
  // Session type and worktree
  sessionType?: "directory" | "new_worktree" | "existing_worktree";
  existingWorktree?: string;
  workingDir?: string;
  oneOff?: boolean;
  initialPrompt?: string;
  // New project mode: tells the context layer to use SESSION_TYPE_NEW_PROJECT
  isNewProject?: boolean;
  // Explicit opt-in to create the directory + git repo if `path` doesn't exist.
  createIfMissing?: boolean;
}

// Validates a project name: no path separators, null bytes, or leading/trailing spaces/dots.
function isValidProjectName(name: string): boolean {
  if (!name.trim()) return false;
  return !/[/\\<>:"|?*\x00]/.test(name) && !/^\.|\.$|^ | $/.test(name);
}

const RESULT_LISTBOX_ID = "omnibar-result-listbox";

export function Omnibar({ isOpen, onClose, onCreateSession, onNavigateToSession, initialMode }: OmnibarProps) {
  const router = useRouter();
  const { setTheme } = useTheme();

  // Input state
  const [input, setInput] = useState("");
  const [detection, setDetection] = useState<DetectionResult | null>(null);

  // Consolidated form state
  const [formState, setFormState] = useState<OmnibarFormState>(INITIAL_FORM_STATE);
  const setFormField = useCallback(
    <K extends keyof OmnibarFormState>(key: K, value: OmnibarFormState[K]) =>
      setFormState((prev) => ({ ...prev, [key]: value })),
    []
  );

  // Consolidated UI state
  const [uiState, setUIState] = useState<OmnibarUIState>({
    showAdvanced: false,
    dropdownIndex: -1,
    dropdownDismissed: false,
    resultHighlightIndex: -1,
  });
  const setUIField = useCallback(
    <K extends keyof OmnibarUIState>(key: K, value: OmnibarUIState[K]) =>
      setUIState((prev) => ({ ...prev, [key]: value })),
    []
  );

  // Submission state
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Mode state machine
  const [modeState, dispatchMode] = useModeReducer();

  // Confirmation dialog state for Directory mode with non-existent path
  const [showPathConfirmation, setShowPathConfirmation] = useState(false);
  const [pendingSessionData, setPendingSessionData] = useState<OmnibarSessionData | null>(null);

  // Convenience aliases for existing code
  // Destructure only fields needed for validation/submission logic in Omnibar.tsx
  const { sessionName, program, category, autoYes, sessionType, branch, useTitleAsBranch, existingWorktree, workingDir, parentDir, projectName, newProjectSessionType, createIfMissing } = formState;
  const { showAdvanced } = uiState;
  const { dropdownIndex, dropdownDismissed, resultHighlightIndex } = uiState;
  // Used in detection auto-fill effects
  const setSessionName = useCallback((v: string) => setFormField("sessionName", v), [setFormField]);
  const setBranch = useCallback((v: string) => setFormField("branch", v), [setFormField]);
  const setDropdownIndex = useCallback((updater: number | ((prev: number) => number)) => {
    setUIState((prev) => ({
      ...prev,
      dropdownIndex: typeof updater === "function" ? updater(prev.dropdownIndex) : updater,
    }));
  }, []);
  const setDropdownDismissed = useCallback((v: boolean) => setUIField("dropdownDismissed", v), [setUIField]);
  const setResultHighlightIndex = useCallback((updater: number | ((prev: number) => number)) => {
    setUIState((prev) => ({
      ...prev,
      resultHighlightIndex: typeof updater === "function" ? updater(prev.resultHighlightIndex) : updater,
    }));
  }, []);

  // Refs
  const inputRef = useRef<HTMLInputElement>(null);
  const debounceRef = useRef<NodeJS.Timeout | null>(null);
  const lastSuggestedNameRef = useRef<string>("");
  const prevDetectionTypeRef = useRef<string | null>(null);
  // Stable ref so handleKeyDown can always call the latest handleSubmit without
  // a circular declaration-order dependency (handleKeyDown is declared before handleSubmit).
  const handleSubmitRef = useRef<() => void>(() => {});
  // Holds the latest attached image paths from OmnibarCreationPanel without causing re-renders.
  const attachedImagePathsRef = useRef<string[]>([]);
  // Refs for form fields read inside the detection effect — avoids re-triggering
  // the effect (and resetting to discovery mode) when only form fields change.
  const sessionNameRef = useRef(sessionName);
  const branchRef = useRef(branch);
  useEffect(() => { sessionNameRef.current = sessionName; }, [sessionName]);
  useEffect(() => { branchRef.current = branch; }, [branch]);

  // API base URL for pre-session image uploads — uses shared helper for SSR/dev consistency.
  const uploadBaseUrl = getApiBaseUrl();

  // Determine whether completions should be active.
  const isPathInput =
    detection?.type === InputType.LocalPath ||
    detection?.type === InputType.PathWithBranch;

  // Use the detected local path (strips branch suffix for PathWithBranch).
  const completionPrefix = isPathInput ? detection?.localPath ?? input : "";

  const {
    entries: completionEntries,
    baseDir: completionBaseDir,
    pathExists,
    isLoading: isCompletionLoading,
    error: completionError,
  } = usePathCompletions(completionPrefix, {
    enabled: isPathInput,
    directoriesOnly: true,
  });

  const { getMatching: getHistoryMatching, getAll: getAllHistory, save: saveHistory } = usePathHistory();

  // Worktree suggestions for the "Use Existing Worktree" mode.
  // Prefer the pre-selected repo path from creation_with_repo mode; fall back to the
  // live-detected local path when the user typed a path directly in the input.
  const repoPathForWorktrees =
    modeState.type === "creation_with_repo" && modeState.path
      ? modeState.path
      : isPathInput
      ? (detection?.localPath ?? "")
      : "";
  const { worktrees, isLoading: isWorktreesLoading } = useWorktreeSuggestions(repoPathForWorktrees, {
    enabled: sessionType === "existing_worktree" && !!repoPathForWorktrees,
  });

  // Convert live OS entries to CompletionEntry for type-safe downstream use.
  const liveEntries = useMemo<CompletionEntry[]>(
    () =>
      completionEntries.map((e) => ({
        name: e.name,
        path: e.path,
        isDirectory: e.isDirectory,
      })),
    [completionEntries]
  );

  // History entries matching the current prefix.
  const historyMatches = useMemo<CompletionEntry[]>(
    () =>
      isPathInput
        ? getHistoryMatching(completionPrefix).map((h) => ({
            name: h.path,
            path: h.path,
            isDirectory: true,
            isHistory: true,
          }))
        : [],
    [isPathInput, completionPrefix, getHistoryMatching]
  );

  // Merged entries: history first, then live (deduped against history).
  const mergedEntries = useMemo<CompletionEntry[]>(() => {
    const liveDeduped = liveEntries.filter(
      (e) => !historyMatches.some((h) => h.path === e.path)
    );
    return [...historyMatches, ...liveDeduped];
  }, [historyMatches, liveEntries]);

  const historyCount = historyMatches.length;

  const isDropdownVisible =
    isPathInput && mergedEntries.length > 0 && !dropdownDismissed;

  // Discovery mode derived from modeState
  const isDiscoveryMode = modeState.type === "discovery";

  // Compute session search query without waiting for the 150ms detection debounce.
  // Bare text (no path/URL/github prefix) passes to Fuse immediately for zero-lag search.
  const sessionSearchQuery = useMemo(() => {
    if (!input.trim()) return "";
    if (detection?.type === InputType.SessionSearch) return input;
    // Eagerly treat as session search if input doesn't look like a path or URL
    if (!input.startsWith("/") && !input.startsWith("~") && !input.startsWith("http")) {
      return input;
    }
    return "";
  }, [input, detection]);
  const sessionResults = useSessionSearch(sessionSearchQuery);

  const allRepoEntries = useMemo(() => getAllHistory(50), [getAllHistory]);
  const repoFuse = useMemo(
    () =>
      new Fuse(allRepoEntries, {
        keys: [{ name: "path", weight: 1.0 }],
        threshold: 0.4,
        ignoreLocation: true,
        minMatchCharLength: 1,
      }),
    [allRepoEntries]
  );

  const allSessions = useAppSelector(selectAllSessions);

  const displayedSessionResults = useMemo((): SessionSearchResult[] => {
    if (!input.trim()) {
      const active = allSessions
        .filter((s) => s.status !== SessionStatus.UNSPECIFIED)
        .sort((a, b) => {
          const aTime = Number(a.updatedAt?.seconds ?? 0);
          const bTime = Number(b.updatedAt?.seconds ?? 0);
          return bTime - aTime;
        })
        .slice(0, 5);
      return active.map((s) => ({ session: s, score: 0, matchedFields: [] }));
    }
    return sessionResults;
  }, [input, sessionResults, allSessions]);

  const displayedRepoEntries = useMemo(() => {
    if (!input.trim()) {
      return allRepoEntries.slice(0, 5);
    }
    return repoFuse.search(input).map((r) => r.item).slice(0, 8);
  }, [input, allRepoEntries, repoFuse]);

  const totalResultCount = getResultListItemCount(
    displayedSessionResults.length,
    displayedRepoEntries.length
  );

  // Accept a completion entry: fill the input and continue for further completion.
  const handleCompletionSelect = useCallback(
    (entry: CompletionEntry) => {
      const newInput = entry.isDirectory ? entry.path + "/" : entry.path;
      setInput(newInput);
      setDropdownIndex(-1);
      setDropdownDismissed(false);
      inputRef.current?.focus();
    },
    []
  );

  // Detect input type with debouncing
  useEffect(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }

    debounceRef.current = setTimeout(() => {
      if (input.trim()) {
        // Pre-process slash commands before detection so /oneoff etc. aren't
        // misidentified as local paths by LocalPathDetector (priority 100).
        const slashCmd = parseSlashCommand(input);
        if (slashCmd) {
          setFormField("sessionType", slashCmd.sessionType);
        }
        const detectInput = slashCmd ? slashCmd.remainder : input;
        const result = detect(detectInput || input);
        setDetection(result);

        // Reset dropdown dismissed state when input type changes modes.
        // Prevents session results from being suppressed after user dismisses
        // path completion dropdown then backspaces to bare text.
        if (result.type !== prevDetectionTypeRef.current) {
          setDropdownDismissed(false);
        }
        prevDetectionTypeRef.current = result.type;

        // Update mode based on detection type
        if (result.type === InputType.NewSession) {
          // "new/" prefix typed → creation_with_repo mode with query from parsedValue
          dispatchMode({ kind: "new_prefix_typed", query: result.parsedValue });
        } else {
          dispatchMode({ kind: "detect", detection: result });
          if (result.type === InputType.SessionSearch) {
            setResultHighlightIndex(-1);
          }
        }

        // Auto-fill session name (and firstPrompt for `>` separator) if:
        // 1. Session name is empty, OR
        // 2. Session name matches the last auto-suggested name (not manually edited)
        // This allows suggestions to update as the user types the path (e.g., "~" → "sqlway")
        if (result.type === InputType.SessionSearch && !slashCmd) {
          // Derive-on-read: split on first `>` to populate name + firstPrompt
          const parsed = parseInputWithSeparator(input);
          const derivedName = parsed.name;
          if (derivedName && (!sessionNameRef.current || sessionNameRef.current === lastSuggestedNameRef.current)) {
            setSessionName(derivedName);
            lastSuggestedNameRef.current = derivedName;
          }
          if (parsed.firstPrompt) {
            setFormField("firstPrompt", parsed.firstPrompt);
          }
        } else if (result.suggestedName) {
          if (!sessionNameRef.current || sessionNameRef.current === lastSuggestedNameRef.current) {
            setSessionName(result.suggestedName);
            lastSuggestedNameRef.current = result.suggestedName;
          }
        }

        // Auto-fill branch if detected
        if (result.branch && !branchRef.current) {
          setBranch(result.branch);
        }
      } else {
        setDetection(null);
        dispatchMode({ kind: "reset_to_discovery" });
        setResultHighlightIndex(-1);
      }
    }, 150); // 150ms debounce

    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
    };
  }, [input]);

  // Focus input when opened
  useEffect(() => {
    if (isOpen && inputRef.current) {
      inputRef.current.focus();
    }
  }, [isOpen]);

  // Reset state when closed
  useEffect(() => {
    if (!isOpen) {
      setInput("");
      setDetection(null);
      setFormState(INITIAL_FORM_STATE);
      setUIState({ showAdvanced: false, dropdownIndex: -1, dropdownDismissed: false, resultHighlightIndex: -1 });
      setError(null);
      lastSuggestedNameRef.current = "";
      prevDetectionTypeRef.current = null;
      dispatchMode({ kind: "reset_to_discovery" });
    }
  }, [isOpen]);

  // On open: apply initialMode if provided
  useEffect(() => {
    if (isOpen && initialMode === "creation") {
      dispatchMode({ kind: "open_creation_direct" });
    }
  }, [isOpen, initialMode]);

  // Session result selection handlers
  const handleSessionSelect = useCallback(
    (session: Session) => {
      onNavigateToSession(session.id);
      onClose();
    },
    [onNavigateToSession, onClose]
  );

  const handleCloneSession = useCallback(
    (session: Session) => {
      // Pre-fill the input with the source session's path and switch to creation mode
      if (session.path) {
        setInput(session.path);
        dispatchMode({ kind: "select_repo", path: session.path });
        setResultHighlightIndex(-1);
        setDropdownDismissed(false);
        inputRef.current?.focus();
      }
    },
    [dispatchMode]
  );

  const handleRepoSelect = useCallback(
    (path: string) => {
      setInput(path + "/");
      dispatchMode({ kind: "select_repo", path });
      setResultHighlightIndex(-1);
      setDropdownDismissed(false);
      inputRef.current?.focus();
    },
    [dispatchMode]
  );

  const dispatchHighlightedResultAction = useCallback(
    (index: number) => {
      if (index < displayedSessionResults.length) {
        handleSessionSelect(displayedSessionResults[index].session);
      } else {
        const repoIndex = index - displayedSessionResults.length;
        if (repoIndex < displayedRepoEntries.length) {
          handleRepoSelect(displayedRepoEntries[repoIndex].path);
        } else {
          dispatchMode({ kind: "open_creation_direct" });
          setResultHighlightIndex(-1);
        }
      }
    },
    [displayedSessionResults, displayedRepoEntries, handleSessionSelect, handleRepoSelect]
  );

  // Handle keyboard shortcuts
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      // Discovery mode navigation (before dropdown check)
      if (isDiscoveryMode && (resultHighlightIndex >= 0 || e.key === "ArrowDown")) {
        if (e.key === "ArrowDown") {
          e.preventDefault();
          setResultHighlightIndex((i) => Math.min(i + 1, totalResultCount - 1));
          return;
        }
        if (e.key === "ArrowUp") {
          e.preventDefault();
          setResultHighlightIndex((i) => Math.max(i - 1, -1));
          return;
        }
        if (e.key === "Enter" && !e.metaKey && resultHighlightIndex >= 0) {
          e.preventDefault();
          dispatchHighlightedResultAction(resultHighlightIndex);
          return;
        }
        if (e.key === "Escape" && resultHighlightIndex >= 0) {
          e.nativeEvent.stopImmediatePropagation();
          setResultHighlightIndex(-1);
          return;
        }
      }

      if (isDropdownVisible) {
        if (e.key === "ArrowDown") {
          e.preventDefault();
          setDropdownIndex((i) => Math.min(i + 1, mergedEntries.length - 1));
          return;
        }
        if (e.key === "ArrowUp") {
          e.preventDefault();
          setDropdownIndex((i) => Math.max(i - 1, -1));
          return;
        }
        if (e.key === "Tab") {
          e.preventDefault();
          if (dropdownIndex >= 0) {
            // Explicit selection (including history entries) → accept it.
            handleCompletionSelect(mergedEntries[dropdownIndex]);
          } else if (liveEntries.length === 1) {
            handleCompletionSelect(liveEntries[0]);
          } else if (liveEntries.length > 1) {
            // Extend input to longest common prefix of live entry names only.
            const lcp = liveEntries.reduce((acc, entry) => {
              let i = 0;
              while (i < acc.length && i < entry.name.length && acc[i] === entry.name[i]) i++;
              return acc.slice(0, i);
            }, liveEntries[0].name);
            if (lcp) {
              const sep = completionBaseDir.endsWith("/") ? "" : "/";
              setInput(completionBaseDir + sep + lcp);
              setDropdownDismissed(false);
            }
          }
          return;
        }
        if (e.key === "Enter" && !e.metaKey && dropdownIndex >= 0) {
          e.preventDefault();
          handleCompletionSelect(mergedEntries[dropdownIndex]);
          return;
        }
        if (e.key === "Escape") {
          // Stop the native event so the global document listener doesn't
          // also call onClose() — first Escape dismisses the dropdown only.
          e.nativeEvent.stopImmediatePropagation();
          setDropdownDismissed(true);
          setDropdownIndex(-1);
          return;
        }
      }

      // Tab cycles session type when in creation mode and no dropdown is open
      if (e.key === "Tab" && !isDiscoveryMode && !isDropdownVisible) {
        e.preventDefault();
        const types = SESSION_TYPES.map((t) => t.value);
        const idx = types.indexOf(sessionType as typeof types[number]);
        const next = types[(idx + 1) % types.length];
        setFormField("sessionType", next as OmnibarFormState["sessionType"]);
        return;
      }

      if (e.key === "Escape") {
        // Stop propagation so the global document listener doesn't call onClose() a second time.
        e.nativeEvent.stopImmediatePropagation();
        if (!isDiscoveryMode) {
          // Escape in creation mode: return to discovery rather than closing.
          // Second Escape (in discovery mode) will close.
          dispatchMode({ kind: "reset_to_discovery" });
          setInput("");
          setResultHighlightIndex(-1);
        } else {
          onClose();
        }
      } else if (e.key === "Enter" && e.metaKey) {
        // Cmd+Enter to submit — use ref to avoid declaration-order dependency.
        handleSubmitRef.current();
      }
    },
    [
      isDiscoveryMode,
      resultHighlightIndex,
      totalResultCount,
      dispatchHighlightedResultAction,
      isDropdownVisible,
      mergedEntries,
      liveEntries,
      completionBaseDir,
      dropdownIndex,
      handleCompletionSelect,
      onClose,
      dispatchMode,
      sessionType,
      setFormField,
    ]
  );

  // Global keyboard handler
  useEffect(() => {
    const handleGlobalKeyDown = (e: KeyboardEvent) => {
      // Cmd+K or Ctrl+K to open (handled by parent)
      if (isOpen && e.key === "Escape") {
        onClose();
      }
    };

    document.addEventListener("keydown", handleGlobalKeyDown);
    return () => document.removeEventListener("keydown", handleGlobalKeyDown);
  }, [isOpen, onClose]);

  // Get type info for display
  const typeInfo = useMemo(() => {
    if (!detection) return INPUT_TYPE_INFO[InputType.Unknown];
    return INPUT_TYPE_INFO[detection.type];
  }, [detection]);

  // True only after path completion has resolved and the path is missing.
  // Also requires that we're working with a local path (GitHub URLs are
  // resolved server-side, so existence isn't meaningful here).
  const pathDoesNotExist =
    isPathInput && !isCompletionLoading && pathExists === false;

  // Check if we can submit
  const canSubmit = useMemo(() => {
    // One-off mode: only session name is required (no path needed).
    if (sessionType === "one_off") {
      return !!sessionName.trim();
    }

    // New project mode: requires parentDir + projectName (valid), no path detection.
    if (sessionType === "new_project") {
      if (!sessionName.trim()) return false;
      if (!parentDir.trim()) return false;
      if (!projectName.trim()) return false;
      if (!isValidProjectName(projectName)) return false;
      if (newProjectSessionType === "new_worktree" && !useTitleAsBranch && !branch.trim()) return false;
      return true;
    }

    // Recognized commands (>theme ..., >go ...) are always submittable
    if (detection?.type === InputType.Command && detection.confidence === 1.0) return true;

    if (!input.trim()) return false;
    if (!sessionName.trim()) return false;
    if (!detection || detection.type === InputType.Unknown || detection.type === InputType.Command || detection.type === InputType.SessionSearch) return false;

    // Validate session type specific requirements
    if (sessionType === "new_worktree") {
      // Branch is required unless using title as branch
      if (!useTitleAsBranch && !branch.trim()) return false;
    } else if (sessionType === "existing_worktree") {
      // Existing worktree path is required
      if (!existingWorktree.trim()) return false;
      // existing_worktree requires the parent repo path to actually exist
      if (pathDoesNotExist) return false;
    }

    // For directory / new_worktree: missing path requires explicit opt-in
    if (
      pathDoesNotExist &&
      (sessionType === "directory" || sessionType === "new_worktree") &&
      !createIfMissing
    ) {
      return false;
    }

    return true;
  }, [input, sessionName, detection, sessionType, branch, useTitleAsBranch, existingWorktree, pathDoesNotExist, createIfMissing, parentDir, projectName, newProjectSessionType]);

  // Handle form submission
  const handleSubmit = useCallback(async () => {
    if (!canSubmit || isSubmitting) return;

    // Execute omnibar commands (>theme ..., >go ...) immediately without entering
    // session-creation flow. These are fire-and-forget; no loading state needed.
    if (detection?.type === InputType.Command && detection.confidence === 1.0 && detection.metadata) {
      const { commandType, commandArg } = detection.metadata as { commandType: string; commandArg: string };
      if (commandType === "theme") {
        setTheme(commandArg as ThemeName);
      } else if (commandType === "navigate") {
        router.push(commandArg);
      }
      onClose();
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      // Determine final branch name
      let finalBranch = branch.trim();
      if ((sessionType === "new_worktree" || (sessionType === "new_project" && newProjectSessionType === "new_worktree")) && useTitleAsBranch) {
        finalBranch = sessionName.trim();
      }

      // Build prompt from attached image paths (pre-session uploads go to temp dir).
      const imagePaths = attachedImagePathsRef.current;
      const finalPrompt = imagePaths.length > 0 ? imagePaths.join(" ") : undefined;
      const firstPromptText = formState.firstPrompt?.trim() || undefined;

      let sessionData: OmnibarSessionData;

      if (sessionType === "new_project") {
        // New project mode: build the resolved path from parentDir + projectName
        const resolvedPath = `${parentDir.trim().replace(/\/$/, "")}/${projectName.trim()}`;
        sessionData = {
          title: sessionName.trim(),
          path: resolvedPath,
          branch: newProjectSessionType === "new_worktree" ? (finalBranch || undefined) : undefined,
          program,
          category: category.trim() || undefined,
          prompt: finalPrompt,
          autoYes,
          sessionType: newProjectSessionType,
          isNewProject: true,
          initialPrompt: firstPromptText,
        };
      } else {
        sessionData = {
          title: sessionName.trim(),
          path: sessionType === "one_off" ? "" : (detection?.localPath || ""),
          branch: sessionType === "one_off" ? undefined : (finalBranch || undefined),
          program,
          category: category.trim() || undefined,
          prompt: finalPrompt,
          autoYes,
          sessionType: sessionType === "one_off" ? "directory" : sessionType,
          existingWorktree: sessionType === "one_off" ? undefined : (existingWorktree.trim() || undefined),
          workingDir: sessionType === "one_off" ? undefined : (workingDir.trim() || undefined),
          oneOff: sessionType === "one_off" ? true : undefined,
          // Only forward when relevant (non-existent path + opt-in checked).
          createIfMissing: pathDoesNotExist && createIfMissing ? true : undefined,
          initialPrompt: firstPromptText,
        };

        // Handle GitHub URLs - path will be resolved server-side
        if (sessionType !== "one_off" && detection?.gitHubRef) {
          sessionData.gitHubOwner = detection.gitHubRef.owner;
          sessionData.gitHubRepo = detection.gitHubRef.repo;
          sessionData.gitHubPRNumber = detection.gitHubRef.prNumber;

          // For GitHub URLs, set path to the parsed value for server-side cloning
          if (!sessionData.path) {
            sessionData.path = detection.parsedValue;
          }
        }
      }

      try {
        await onCreateSession(sessionData);
      } catch (err) {
        // R2: Directory mode with non-existent path → show confirmation dialog
        if (
          sessionType === "directory" &&
          err instanceof Error &&
          (err.message.includes("not found") || err.message.includes("CodeNotFound") || err.message.includes("path does not exist"))
        ) {
          setPendingSessionData(sessionData);
          setShowPathConfirmation(true);
          setIsSubmitting(false);
          return;
        }
        throw err;
      }

      // Persist the chosen path to history for future completions.
      if (isPathInput && detection?.localPath && sessionType !== "one_off") {
        saveHistory(detection.localPath);
      }
      onClose();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to create session";
      setError(message);
    } finally {
      setIsSubmitting(false);
    }
  }, [
    canSubmit,
    isSubmitting,
    branch,
    sessionName,
    sessionType,
    useTitleAsBranch,
    detection,
    program,
    category,
    autoYes,
    existingWorktree,
    workingDir,
    parentDir,
    projectName,
    newProjectSessionType,
    pathDoesNotExist,
    createIfMissing,
    isPathInput,
    saveHistory,
    onCreateSession,
    onClose,
  ]);

  // Keep the ref in sync so handleKeyDown always dispatches the latest version.
  useEffect(() => {
    handleSubmitRef.current = handleSubmit;
  }, [handleSubmit]);

  if (!isOpen) return null;

  const isMac = (() => {
    try {
      return typeof navigator !== 'undefined' && /Mac|iPhone|iPad|iPod/.test(navigator.platform);
    } catch {
      return false;
    }
  })();

  return (
    <div
      className={overlay}
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="omnibar-title"
    >
      <div
        className={modal}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Main Input */}
        <div className={inputContainer}>
          <span className={typeIndicator} aria-hidden="true">
            {sessionType === "one_off" ? "⚡" : typeInfo.icon}
          </span>
          <input
            ref={inputRef}
            type="text"
            className={inputClass}
            placeholder={
              sessionType === "one_off"
                ? "Session title is the only thing needed…"
                : isDiscoveryMode
                ? "Jump to session or search repos..."
                : "Enter path, GitHub URL, or owner/repo..."
            }
            value={input}
            onChange={(e) => {
              setInput(e.target.value);
              setDropdownDismissed(false);
              setDropdownIndex(-1);
            }}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck={false}
            aria-label="Session source input"
            aria-autocomplete="list"
            aria-expanded={
              isDiscoveryMode
                ? displayedSessionResults.length > 0 || displayedRepoEntries.length > 0
                : isDropdownVisible
            }
            aria-controls={
              isDiscoveryMode ? RESULT_LISTBOX_ID : "path-completion-listbox"
            }
            aria-activedescendant={
              isDiscoveryMode
                ? getHighlightedItemId(
                    RESULT_LISTBOX_ID,
                    displayedSessionResults,
                    displayedRepoEntries,
                    resultHighlightIndex
                  )
                : isDropdownVisible && dropdownIndex >= 0
                ? `path-completion-listbox-option-${dropdownIndex}`
                : undefined
            }
          />
          {/* Path existence indicator. When the path is missing and the user
              has opted in to create it, swap ✗ for + so the affordance reads
              as "create" rather than "broken". */}
          {isPathInput && !isDiscoveryMode && input.trim() && (
            <span
              className={pathIndicator}
              aria-live="polite"
              aria-label={
                isCompletionLoading
                  ? "Checking path"
                  : pathExists
                  ? "Path exists"
                  : createIfMissing
                  ? "New repository will be created"
                  : "Path does not exist"
              }
            >
              {isCompletionLoading ? (
                <span className={pathIndicatorLoading} aria-hidden="true">⟳</span>
              ) : pathExists ? (
                <span className={pathIndicatorValid} aria-hidden="true">✓</span>
              ) : createIfMissing ? (
                <span className={pathIndicatorValid} aria-hidden="true">+</span>
              ) : (
                <span className={pathIndicatorInvalid} aria-hidden="true">✗</span>
              )}
            </span>
          )}
        </div>

        {/* Discovery mode: session results + recent repos */}
        {isDiscoveryMode && (
          <OmnibarResultList
            id={RESULT_LISTBOX_ID}
            sessionResults={displayedSessionResults}
            repoEntries={displayedRepoEntries}
            highlightedIndex={resultHighlightIndex}
            onSessionSelect={handleSessionSelect}
            onRepoSelect={handleRepoSelect}
            onCloneSession={handleCloneSession}
            onCreateNew={() => {
              dispatchMode({ kind: "open_creation_direct" });
              setResultHighlightIndex(-1);
            }}
          />
        )}

        {/* Creation mode: path completion dropdown (existing, unchanged) */}
        {!isDiscoveryMode && isDropdownVisible && sessionType !== "one_off" && (
          <PathCompletionDropdown
            id="path-completion-listbox"
            entries={mergedEntries}
            historyCount={historyCount}
            selectedIndex={dropdownIndex}
            onSelect={handleCompletionSelect}
            isLoading={isCompletionLoading}
          />
        )}

        {/* Path completion error */}
        {isPathInput && completionError && (
          <div className={completionErrorClass} aria-live="polite">
            Could not load completions
          </div>
        )}

        {/* Detection Badge */}
        {input.trim() && !isDiscoveryMode && sessionType !== "one_off" && (
          <div className={detectionInfo}>
            <span
              className={`${detectionBadge} ${
                detection?.type === InputType.Unknown ? unknown : ""
              }`}
            >
              {typeInfo.icon} {typeInfo.label}
            </span>
          </div>
        )}

        {/* Creation form + footer — delegated to OmnibarCreationPanel */}
        {!isDiscoveryMode && (
          <OmnibarCreationPanel
            formState={formState}
            setFormField={setFormField}
            onSubmit={handleSubmit}
            onCancel={onClose}
            worktrees={worktrees}
            isWorktreesLoading={isWorktreesLoading}
            isSubmitting={isSubmitting}
            canSubmit={canSubmit}
            error={error}
            showAdvanced={showAdvanced}
            onToggleAdvanced={() => setUIField("showAdvanced", !uiState.showAdvanced)}
            path={modeState.type === "creation_with_repo" ? modeState.path : undefined}
            uploadBaseUrl={uploadBaseUrl}
            onAttachedImagesChange={(paths) => { attachedImagePathsRef.current = paths; }}
            pathDoesNotExist={pathDoesNotExist}
          />
        )}

        {/* R2: Confirmation dialog for Directory mode with non-existent path */}
        {showPathConfirmation && pendingSessionData && (
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="path-confirm-title"
            style={{
              position: "absolute",
              inset: 0,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              background: "rgba(0,0,0,0.5)",
              zIndex: 10,
              borderRadius: "inherit",
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div
              style={{
                background: "var(--card-background)",
                border: "1px solid var(--border-color)",
                borderRadius: "8px",
                padding: "24px",
                maxWidth: "420px",
                width: "100%",
                margin: "16px",
              }}
            >
              <div id="path-confirm-title" style={{ fontWeight: 600, fontSize: "1rem", marginBottom: "8px" }}>
                Create directory?
              </div>
              <div style={{ fontSize: "0.875rem", color: "var(--text-secondary)", marginBottom: "16px" }}>
                The path <code style={{ fontFamily: "monospace", padding: "0 4px" }}>{pendingSessionData.path}</code> does not exist.
                Create it and initialize a git repository?
              </div>
              <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end" }}>
                <button
                  type="button"
                  style={{
                    padding: "6px 14px",
                    fontSize: "0.875rem",
                    borderRadius: "6px",
                    border: "1px solid var(--border-color)",
                    background: "transparent",
                    color: "var(--text-primary)",
                    cursor: "pointer",
                  }}
                  onClick={() => {
                    setShowPathConfirmation(false);
                    setPendingSessionData(null);
                  }}
                >
                  Cancel
                </button>
                <button
                  type="button"
                  style={{
                    padding: "6px 14px",
                    fontSize: "0.875rem",
                    borderRadius: "6px",
                    border: "none",
                    background: "var(--primary)",
                    color: "var(--primary-text)",
                    cursor: "pointer",
                  }}
                  onClick={async () => {
                    setShowPathConfirmation(false);
                    const retryData = { ...pendingSessionData, createIfMissing: true };
                    setPendingSessionData(null);
                    setIsSubmitting(true);
                    setError(null);
                    try {
                      await onCreateSession(retryData);
                      onClose();
                    } catch (err) {
                      const message = err instanceof Error ? err.message : "Failed to create session";
                      setError(message);
                    } finally {
                      setIsSubmitting(false);
                    }
                  }}
                >
                  Create &amp; Open
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Keyboard Shortcuts */}
        <div className={shortcuts}>
          <OmnibarModeBadge
            mode={isDiscoveryMode ? "discovery" : "creation"}
            onToggle={() =>
              isDiscoveryMode
                ? dispatchMode({ kind: "open_creation_direct" })
                : dispatchMode({ kind: "reset_to_discovery" })
            }
          />
          <span className={shortcut}>
            <span className={shortcutKey}>Esc</span> Close
          </span>
          {!isDiscoveryMode && (
            <span className={shortcut}>
              <span className={shortcutKey}>{isMac ? '⌘↵' : 'Ctrl+↵'}</span> Create
            </span>
          )}
          {isDiscoveryMode && (
            <>
              <span className={shortcut}>
                <span className={shortcutKey}>↑↓</span> Navigate
              </span>
              <span className={shortcut}>
                <span className={shortcutKey}>↵</span> Jump
              </span>
            </>
          )}
          {!isDiscoveryMode && isDropdownVisible && (
            <>
              <span className={shortcut}>
                <span className={shortcutKey}>↑↓</span> Navigate
              </span>
              <span className={shortcut}>
                <span className={shortcutKey}>Tab</span> Complete
              </span>
            </>
          )}
          {!isDiscoveryMode && !isDropdownVisible && (
            <span className={shortcut}>
              <span className={shortcutKey}>Tab</span> Cycle type
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
