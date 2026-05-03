"use client";

import { createContext, useContext, useState, useCallback, useEffect, ReactNode } from "react";
import { useRouter } from "next/navigation";
import { Omnibar, OmnibarSessionData } from "@/components/sessions/Omnibar";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useAuth } from "@/lib/contexts/AuthContext";
import { SessionType } from "@/gen/session/v1/types_pb";

const sessionTypeMap: Record<string, SessionType> = {
  directory: SessionType.DIRECTORY,
  new_worktree: SessionType.NEW_WORKTREE,
  existing_worktree: SessionType.EXISTING_WORKTREE,
  one_off: SessionType.DIRECTORY, // one-off is a directory session; type overridden server-side
  new_project: SessionType.NEW_PROJECT, // new-project mode: backend initializes git repo
};

interface OmnibarContextValue {
  isOpen: boolean;
  open: () => void;
  openInCreationMode: () => void;
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
  const router = useRouter();
  const { authEnabled, authenticated, loading: authLoading } = useAuth();
  const { createSession } = useSessionService({
    enabled: !authLoading && (!authEnabled || authenticated),
  });

  const open = useCallback(() => {
    setInitialMode("discovery");
    setIsOpen(true);
  }, []);
  const openInCreationMode = useCallback(() => {
    setInitialMode("creation");
    setIsOpen(true);
  }, []);
  const close = useCallback(() => setIsOpen(false), []);
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
      });

      if (session) {
        // Navigate to the sessions list (home)
        router.push("/");
        router.refresh();
      }
    },
    [createSession, router]
  );

  const value: OmnibarContextValue = {
    isOpen,
    open,
    openInCreationMode,
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
        initialMode={initialMode}
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
