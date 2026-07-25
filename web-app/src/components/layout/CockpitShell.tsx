"use client";

import { useState, useCallback, useEffect, ReactNode } from "react";
import { usePathname } from "next/navigation";
import { DrawerNav } from "./DrawerNav";
import { BottomNav } from "./BottomNav";
import { KeyboardShortcutOverlay } from "@/components/ui/KeyboardShortcutOverlay";
import { useShortcut } from "@/lib/shortcuts/useShortcut";
import { useNavigation } from "@/lib/contexts/NavigationContext";
import { useViewport } from "@/components/providers/ViewportProvider";
import { cockpitRoot, drawerColumn, mainContent } from "@/styles/layout.css";

interface CockpitShellProps {
  children: ReactNode;
}

/**
 * CockpitShell — client component that renders the two-column cockpit layout
 * (DrawerNav + main content area) and hosts global shortcuts (?  and [).
 *
 * Must be a client component because it reads NavigationContext and registers
 * keyboard shortcuts.
 */
export function CockpitShell({ children }: CockpitShellProps) {
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const { toggleDrawer } = useNavigation();
  const pathname = usePathname();
  const { isMobile, isFoldable } = useViewport();
  const isNarrow = isMobile || isFoldable;
  // Hide the floating ? button on pages that already have their own shortcut button
  const showFloatingShortcutsButton = !isNarrow && pathname !== "/review-queue";

  const openShortcuts = useCallback(() => setShortcutsOpen(true), []);
  const closeShortcuts = useCallback(() => setShortcutsOpen(false), []);

  // Global: [ → toggle nav drawer
  useShortcut("nav:toggle-drawer", {
    key: "[",
    context: "global",
    label: "Toggle navigation drawer",
    action: toggleDrawer,
  });

  // Global: ? → open keyboard shortcut overlay
  useShortcut("shortcuts:open", {
    key: "?",
    context: "global",
    label: "Show keyboard shortcuts",
    action: openShortcuts,
  });

  // Listen for the custom event dispatched by OnboardingModal "View all shortcuts" link
  useEffect(() => {
    const handler = () => setShortcutsOpen(true);
    window.addEventListener("stapler-squad:open-shortcuts", handler);
    return () => window.removeEventListener("stapler-squad:open-shortcuts", handler);
  }, []);

  // Global: ⌘? / Ctrl+? → also open keyboard shortcut overlay
  useShortcut("shortcuts:open-meta", {
    key: "?",
    modifiers: { meta: true },
    context: "global",
    label: "Show keyboard shortcuts",
    action: openShortcuts,
  });

  return (
    <>
      <div className={cockpitRoot}>
        <div className={drawerColumn}>
          <DrawerNav />
        </div>
        <div className={mainContent}>
          {children}
        </div>
      </div>
      <BottomNav />
      <KeyboardShortcutOverlay isOpen={shortcutsOpen} onClose={closeShortcuts} />
      {showFloatingShortcutsButton && (
        <button
          onClick={openShortcuts}
          aria-label="Keyboard shortcuts (?)"
          title="Keyboard shortcuts (?)"
          style={{
            position: "fixed",
            bottom: "1.5rem",
            right: "1.5rem",
            width: "2rem",
            height: "2rem",
            borderRadius: "50%",
            background: "var(--card-background)",
            border: "1px solid var(--border-color)",
            color: "var(--text-secondary)",
            cursor: "pointer",
            fontSize: "0.875rem",
            zIndex: 100,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          ?
        </button>
      )}
    </>
  );
}
