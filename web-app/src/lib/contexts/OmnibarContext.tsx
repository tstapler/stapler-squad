"use client";

import { createContext, useContext, useState, useCallback, useEffect, ReactNode } from "react";
import { useRouter } from "next/navigation";
import { Omnibar, OmnibarSessionData } from "@/components/sessions/Omnibar";
import { useSessionService } from "@/lib/hooks/useSessionService";

interface OmnibarContextValue {
  isOpen: boolean;
  open: () => void;
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
  const router = useRouter();
  const { createSession } = useSessionService();

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => setIsOpen(false), []);
  const toggle = useCallback(() => setIsOpen((prev) => !prev), []);

  // Global keyboard shortcut: Cmd+K or Ctrl+K
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Cmd+K (Mac) or Ctrl+K (Windows/Linux)
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
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
  }, [toggle, open]);

  // Handle session creation
  const handleCreateSession = useCallback(
    async (data: OmnibarSessionData) => {
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
    (element as HTMLElement).isContentEditable
  );
}
