"use client";

import { createContext, useContext, useState, useCallback, useEffect, useRef, useMemo, ReactNode } from "react";
import { useRouter } from "next/navigation";
import { Omnibar, OmnibarSessionData } from "@/components/sessions/Omnibar";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useWorkflows } from "@/lib/hooks/useWorkflows";
import { useAuth } from "@/lib/contexts/AuthContext";
import { SessionType } from "@/gen/session/v1/types_pb";
import { getDefaultRegistry } from "@/lib/omnibar/detector";
import { WorkflowDetector, type WorkflowEntry } from "@/lib/omnibar/detectors/WorkflowDetector";

const sessionTypeMap: Record<string, SessionType> = {
  directory: SessionType.DIRECTORY,
  new_worktree: SessionType.NEW_WORKTREE,
  existing_worktree: SessionType.EXISTING_WORKTREE,
  one_off: SessionType.DIRECTORY, // one-off is a directory session; type overridden server-side
  new_project: SessionType.NEW_PROJECT, // new-project mode: backend initializes git repo
  autonomous: SessionType.DIRECTORY, // autonomous reuses DIRECTORY; server handles autonomous flag
};

interface OmnibarContextValue {
  isOpen: boolean;
  open: () => void;
  openInCreationMode: () => void;
  openOmnibar: (initialInput?: string) => void;
  close: () => void;
  toggle: () => void;
}

const OmnibarContext = createContext<OmnibarContextValue | null>(null);

export function useOmnibar(): OmnibarContextValue {
  const context = useContext(OmnibarContext);
  if (!context) {
    throw new Error("useOmnibar must be used within an OmnibarProvider");
  }
  return context;
}

interface OmnibarProviderProps {
  children: ReactNode;
}

export function OmnibarProvider({ children }: OmnibarProviderProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [initialMode, setInitialMode] = useState<"discovery" | "creation">("discovery");
  const [initialInput, setInitialInput] = useState<string | undefined>(undefined);
  const router = useRouter();
  const { authEnabled, authenticated, loading: authLoading } = useAuth();
  const { createSession, spawnShell, runWorkflow: runWorkflowRPC } = useSessionService({
    enabled: !authLoading && (!authEnabled || authenticated),
  });
  const { workflows } = useWorkflows();

  // Lean WorkflowEntry[] for the detector and @ autocomplete dropdown.
  const workflowEntries = useMemo<WorkflowEntry[]>(
    () =>
      workflows.map((w) => ({
        slug: w.slug,
        name: w.name,
        description: w.description,
        targetDirectory: w.targetDirectory,
        sessionType: w.sessionType,
        inputTemplate: w.inputTemplate,
      })),
    [workflows]
  );

  // Dynamically register/unregister WorkflowDetector whenever the workflow list changes.
  // The detector is NOT in createDefaultRegistry() — it lives only in the singleton
  // returned by getDefaultRegistry() so it reflects the live DB state.
  const workflowDetectorRef = useRef<WorkflowDetector | null>(null);
  useEffect(() => {
    const registry = getDefaultRegistry();
    if (workflowDetectorRef.current) {
      registry.unregister(workflowDetectorRef.current);
    }
    const detector = new WorkflowDetector(workflowEntries);
    registry.register(detector);
    workflowDetectorRef.current = detector;
    return () => {
      registry.unregister(detector);
      workflowDetectorRef.current = null;
    };
  }, [workflowEntries]);

  const open = useCallback(() => {
    setInitialMode("discovery");
    setInitialInput(undefined);
    setIsOpen(true);
  }, []);
  const openInCreationMode = useCallback(() => {
    setInitialMode("creation");
    setInitialInput(undefined);
    setIsOpen(true);
  }, []);
  const openOmnibar = useCallback((inputValue?: string) => {
    setInitialMode(inputValue ? "creation" : "discovery");
    setInitialInput(inputValue);
    setIsOpen(true);
  }, []);
  const close = useCallback(() => {
    setIsOpen(false);
    setInitialInput(undefined);
  }, []);
  const toggle = useCallback(() => setIsOpen((prev) => !prev), []);

  // Global keyboard shortcut: Cmd+K or Ctrl+K (discovery), Cmd+Shift+K (creation)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Cmd+Shift+K (Mac) or Ctrl+Shift+K — open directly in creation mode
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === "K") {
        e.preventDefault();
        openInCreationMode();
        return;
      }

      // Cmd+K (Mac) or Ctrl+K (Windows/Linux) — discovery mode toggle
      if ((e.metaKey || e.ctrlKey) && !e.shiftKey && e.key === "k") {
        e.preventDefault();
        toggle();
      }

      // Also support 'n' key when not in an input
      if (e.key === "n" && !isInputElement(e.target as Element)) {
        e.preventDefault();
        open();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [toggle, open, openInCreationMode]);

  const handleNavigateToSession = useCallback(
    (sessionId: string) => {
      router.push(`/?session=${sessionId}`);
      close();
    },
    [router, close]
  );

  const handleNavigateToSessionInNewPane = useCallback(
    (sessionId: string) => {
      router.push(`/?session=${sessionId}&newPane=true`);
      close();
    },
    [router, close]
  );

  // Handle session creation
  const handleCreateSession = useCallback(
    async (data: OmnibarSessionData) => {
      // Determine effective session type.
      // For new_project + "open as new_worktree": use NEW_WORKTREE — findGitRepoRoot already
      // handles mkdir + git init + initial commit for non-existent paths, so no special type needed.
      // For new_project + "open as directory": use NEW_PROJECT so the backend initialises the repo
      // and opens the session without a worktree.
      const effectiveSessionType = data.isNewProject
        ? data.sessionType === "new_worktree"
          ? sessionTypeMap["new_worktree"]
          : SessionType.NEW_PROJECT
        : data.sessionType
        ? sessionTypeMap[data.sessionType]
        : undefined;

      // createSession throws on error, so no null check needed
      const session = await createSession({
        title: data.title,
        path: data.path,
        branch: data.branch,
        program: data.program,
        category: data.category,
        prompt: data.prompt,
        autoYes: data.autoYes,
        workingDir: data.workingDir,
        existingWorktree: data.existingWorktree,
        sessionType: effectiveSessionType,
        oneOff: data.oneOff ?? false,
        createIfMissing: data.createIfMissing ?? false,
        initialPrompt: data.initialPrompt,
        autonomousMode: data.autonomousMode ?? false,
        permissionMode: data.permissionMode ?? "",
      });

      if (session) {
        // Navigate to the new session so it auto-opens in the detail pane on mobile.
        router.push(`/?session=${session.id}`);
      }
    },
    [createSession, router]
  );

  // Handle spawn_shell omnibar command — calls the RPC directly with an optional command arg.
  const handleSpawnShell = useCallback(
    async (_sessionId?: string, workingDir?: string, shellCommand?: string) => {
      await spawnShell({
        sessionId: _sessionId ?? "",
        workingDir: workingDir ?? "",
        command: shellCommand ?? "",
      });
    },
    [spawnShell]
  );

  // Handle run_workflow omnibar action — fires the workflow via RunWorkflow RPC,
  // then navigates to the newly created session so the user can see it running.
  const handleRunWorkflow = useCallback(
    async (slug: string, arg: string) => {
      const wf = workflows.find((w) => w.slug === slug);
      if (!wf) {
        console.error("Unknown workflow slug:", slug);
        return;
      }
      try {
        const sessionId = await runWorkflowRPC({ id: wf.id, arg });
        if (sessionId) {
          router.push(`/?session=${sessionId}`);
        }
      } catch (err) {
        console.error("Failed to run workflow:", err);
      }
    },
    [runWorkflowRPC, workflows, router]
  );

  const value: OmnibarContextValue = {
    isOpen,
    open,
    openInCreationMode,
    openOmnibar,
    close,
    toggle,
  };

  return (
    <OmnibarContext.Provider value={value}>
      {children}
      <Omnibar
        isOpen={isOpen}
        onClose={close}
        onCreateSession={handleCreateSession}
        onNavigateToSession={handleNavigateToSession}
        onNavigateToSessionInNewPane={handleNavigateToSessionInNewPane}
        onSpawnShell={handleSpawnShell}
        onRunWorkflow={handleRunWorkflow}
        initialMode={initialMode}
        initialInput={initialInput}
        workflows={workflowEntries}
      />
    </OmnibarContext.Provider>
  );
}

// Helper to check if target is an input element
function isInputElement(element: Element | null): boolean {
  if (!element) return false;
  const tagName = element.tagName.toLowerCase();
  return (
    tagName === "input" ||
    tagName === "textarea" ||
    tagName === "select" ||
    element instanceof HTMLElement && element.isContentEditable
  );
}
