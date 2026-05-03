"use client";

import { useState, useCallback, ReactNode } from "react";
import { DrawerNav } from "./DrawerNav";
import { KeyboardShortcutOverlay } from "@/components/ui/KeyboardShortcutOverlay";
import { useShortcut } from "@/lib/shortcuts/useShortcut";
import { useNavigation } from "@/lib/contexts/NavigationContext";
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
      <KeyboardShortcutOverlay isOpen={shortcutsOpen} onClose={closeShortcuts} />
    </>
  );
}
