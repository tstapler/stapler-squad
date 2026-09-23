"use client";

import { useCallback, useEffect, useRef } from "react";
import type { RefObject } from "react";
import { useShortcut } from "@/lib/shortcuts/useShortcut";
import { registry } from "@/lib/shortcuts/shortcutRegistry";
import type { NamedWindow, WindowId } from "./windowTypes";

/**
 * useWindowShortcuts — tmux-style leader-key window switcher.
 *
 * Alt+W arms a leader mode for LEADER_TIMEOUT_MS, during which 1-9/n/p/,/Escape
 * are dynamically registered as follow-ups (Task 3.3.1a/b). Alt+W was chosen
 * over any Ctrl-based leader (Ctrl+B, tmux's actual default prefix, or Ctrl+A,
 * its common screen/remap alternative) specifically because this app displays
 * live tmux-backed terminal sessions in panes: a Ctrl-based leader risks arming
 * this app's outer leader mode whenever the user sends a real tmux prefix
 * keystroke into a pane, silently hijacking their very next keystroke before it
 * reaches the terminal (see pre-mortem.md P1 #1 / Failure Mode #1). Alt sidesteps
 * that whole collision class rather than trading one Ctrl+<letter> for another
 * that might also be someone's tmux remap.
 *
 * Task 3.3.2a — manually testing Alt+W across Chrome/Firefox/Edge/Safari x
 * Windows/macOS/Linux for Alt-as-menu-accelerator or other browser-chrome/OS
 * reservations — is a REQUIRED human follow-up step that could NOT be
 * performed here: this implementation ran in a sandboxed coding environment
 * with no real browser/OS matrix available. Alt+W is left registered as the
 * plan's chosen default below, NOT yet verified collision-free. If that manual
 * spike finds a collision on any target combination, the fallback is
 * Alt+Shift+W — the same escape-hatch pattern usePaneShortcuts.ts already uses
 * for Ctrl+- (documented fallback: Ctrl+Shift+H) — and the only change needed
 * is the `key`/`modifiers` passed to the `useShortcut("window.leader", ...)`
 * call below.
 */

const LEADER_TIMEOUT_MS = 3000;

const DIGIT_KEYS = ["1", "2", "3", "4", "5", "6", "7", "8", "9"] as const;

/** Every key the leader recognizes as a follow-up once armed. */
const FOLLOWUP_KEYS = new Set<string>([...DIGIT_KEYS, "n", "p", ",", "Escape"]);

/** Mutable state threaded through the module-level leader functions below. */
interface LeaderRefs {
  windows: RefObject<NamedWindow[]>;
  currentWindowId: RefObject<WindowId>;
  switchToWindow: RefObject<(id: WindowId) => void>;
  onRenameRequest: RefObject<(id: WindowId) => void>;
  followupCleanups: RefObject<Array<() => void>>;
  timeout: RefObject<ReturnType<typeof setTimeout> | null>;
  catchAllListener: RefObject<((e: KeyboardEvent) => void) | null>;
}

function disarmLeader(refs: LeaderRefs): void {
  for (const cleanup of refs.followupCleanups.current) cleanup();
  refs.followupCleanups.current = [];
  if (refs.timeout.current !== null) {
    clearTimeout(refs.timeout.current);
    refs.timeout.current = null;
  }
  if (refs.catchAllListener.current !== null) {
    document.removeEventListener("keydown", refs.catchAllListener.current, true);
    refs.catchAllListener.current = null;
  }
}

function switchToOneIndexed(refs: LeaderRefs, oneIndexedPosition: number): void {
  const target = refs.windows.current[oneIndexedPosition - 1];
  if (target) refs.switchToWindow.current(target.id);
}

function cycle(refs: LeaderRefs, delta: 1 | -1): void {
  const list = refs.windows.current;
  if (list.length === 0) return;
  const currentIndex = list.findIndex((w) => w.id === refs.currentWindowId.current);
  const base = currentIndex === -1 ? 0 : currentIndex;
  const nextIndex = (base + delta + list.length) % list.length;
  refs.switchToWindow.current(list[nextIndex].id);
}

function registerFollowup(refs: LeaderRefs, id: string, key: string, action: () => void): void {
  refs.followupCleanups.current.push(
    registry.register(id, {
      key,
      label: `Window leader: ${key}`,
      context: "cockpit",
      action: () => {
        action();
        disarmLeader(refs);
      },
    })
  );
}

function registerDigitFollowups(refs: LeaderRefs): void {
  DIGIT_KEYS.forEach((key, index) => {
    const oneIndexedPosition = index + 1;
    registerFollowup(refs, `window.leader.${key}`, key, () => switchToOneIndexed(refs, oneIndexedPosition));
  });
}

function registerNavigationFollowups(refs: LeaderRefs): void {
  registerFollowup(refs, "window.leader.next", "n", () => cycle(refs, 1));
  registerFollowup(refs, "window.leader.prev", "p", () => cycle(refs, -1));
  registerFollowup(refs, "window.leader.rename", ",", () =>
    refs.onRenameRequest.current(refs.currentWindowId.current)
  );
  // Escape disarms with no other action.
  registerFollowup(refs, "window.leader.cancel", "Escape", () => {});
}

function armCatchAllListener(refs: LeaderRefs): void {
  // Watches every keydown while armed, purely to catch an unrecognized
  // follow-up key and disarm before the registry's own bubble-phase dispatch
  // runs (capture phase always fires first). Deregistering the follow-ups
  // here means the registry's dispatch loop then finds nothing to match, so
  // it never calls preventDefault -- the keystroke is left unconsumed for
  // whatever else (e.g. a terminal pane) would normally receive it.
  const onAnyKeyWhileArmed = (e: KeyboardEvent) => {
    if (!FOLLOWUP_KEYS.has(e.key)) {
      disarmLeader(refs);
    }
  };
  refs.catchAllListener.current = onAnyKeyWhileArmed;
  document.addEventListener("keydown", onAnyKeyWhileArmed, true);
}

function armLeader(refs: LeaderRefs): void {
  // A second Alt+W while already armed starts a fresh sequence rather than
  // stacking duplicate registrations.
  disarmLeader(refs);
  registerDigitFollowups(refs);
  registerNavigationFollowups(refs);
  armCatchAllListener(refs);
  refs.timeout.current = setTimeout(() => disarmLeader(refs), LEADER_TIMEOUT_MS);
}

/**
 * Builds the LeaderRefs bundle. Rebuilt each render, but every property
 * points at the same persistent ref object across renders, so closures
 * capturing the returned bundle never go stale.
 */
function useLeaderRefs(
  windows: NamedWindow[],
  currentWindowId: WindowId,
  switchToWindow: (id: WindowId) => void,
  onRenameRequest: (id: WindowId) => void
): LeaderRefs {
  const windowsRef = useRef(windows);
  windowsRef.current = windows;
  const currentWindowIdRef = useRef(currentWindowId);
  currentWindowIdRef.current = currentWindowId;
  const switchToWindowRef = useRef(switchToWindow);
  switchToWindowRef.current = switchToWindow;
  const onRenameRequestRef = useRef(onRenameRequest);
  onRenameRequestRef.current = onRenameRequest;
  const followupCleanupsRef = useRef<Array<() => void>>([]);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const catchAllListenerRef = useRef<((e: KeyboardEvent) => void) | null>(null);

  return {
    windows: windowsRef,
    currentWindowId: currentWindowIdRef,
    switchToWindow: switchToWindowRef,
    onRenameRequest: onRenameRequestRef,
    followupCleanups: followupCleanupsRef,
    timeout: timeoutRef,
    catchAllListener: catchAllListenerRef,
  };
}

export function useWindowShortcuts(
  windows: NamedWindow[],
  currentWindowId: WindowId,
  switchToWindow: (id: WindowId) => void,
  onRenameRequest: (id: WindowId) => void
): void {
  const refs = useLeaderRefs(windows, currentWindowId, switchToWindow, onRenameRequest);

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const handleArmLeader = useCallback(() => armLeader(refs), []);

  useEffect(() => {
    return () => disarmLeader(refs);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useShortcut("window.leader", {
    key: "w",
    modifiers: { alt: true },
    label: "Window leader key",
    context: "cockpit",
    action: handleArmLeader,
  });
}
