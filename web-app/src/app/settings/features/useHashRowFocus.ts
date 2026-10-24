"use client";

import { useEffect, useRef, useState } from "react";

export type ListState = "loading" | "loaded" | "failed";

/** A still-loading list ends the attempt after this long (UX-37). */
export const HASH_FOCUS_CEILING_MS = 10_000;

/** Elements whose focus counts as "the user has not put focus anywhere yet". */
const IDLE_FOCUS_IDS = ["main-content"];

interface Attempt {
  id: string;
  /** hashchange/popstate is the user's own navigation, so the no-steal checks do not apply. */
  explicit: boolean;
}

function readHash(): string {
  const raw = window.location.hash.replace(/^#/, "");
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

function focusIsIdle(): boolean {
  const active = document.activeElement;
  return !active || active === document.body || IDLE_FOCUS_IDS.includes(active.id);
}

/**
 * Focuses and scrolls to the element named by the URL hash once the flag list
 * reaches a terminal state. Runs once per distinct hash; never steals focus
 * the user already moved (a key or pointer press since mount, or focus on
 * anything but body/the page container) for the mount-time hash.
 */
export function useHashRowFocus(listState: ListState): void {
  const [attempt, setAttempt] = useState<Attempt | null>(null);
  const handled = useRef<string | null>(null);
  const interacted = useRef(false);
  // Which list state the latest press happened in, and what `interacted` was when the list failed.
  const lastPressState = useRef<ListState | null>(null);
  const interactedBeforeFailure = useRef(false);
  const stateRef = useRef(listState);
  stateRef.current = listState;

  useEffect(() => {
    const mark = () => {
      interacted.current = true;
      lastPressState.current = stateRef.current;
    };
    const opts = { capture: true } as const;
    document.addEventListener("keydown", mark, opts);
    document.addEventListener("pointerdown", mark, opts);
    document.addEventListener("mousedown", mark, opts);

    const begin = (explicit: boolean) => {
      const id = readHash();
      handled.current = null;
      setAttempt(id ? { id, explicit } : null);
    };
    const onNav = () => begin(true);
    begin(false);
    window.addEventListener("hashchange", onNav);
    window.addEventListener("popstate", onNav);
    return () => {
      document.removeEventListener("keydown", mark, opts);
      document.removeEventListener("pointerdown", mark, opts);
      document.removeEventListener("mousedown", mark, opts);
      window.removeEventListener("hashchange", onNav);
      window.removeEventListener("popstate", onNav);
    };
  }, []);

  // Ceiling: a list still loading when it fires ends the attempt with no focus change.
  useEffect(() => {
    if (!attempt) return;
    const timer = setTimeout(() => {
      if (stateRef.current === "loading") handled.current = attempt.id;
    }, HASH_FOCUS_CEILING_MS);
    return () => clearTimeout(timer);
  }, [attempt]);

  // A press made while "failed" is the Retry that triggered this load, not the user choosing another target;
  // a press made in any other state is real interaction and must survive the retry.
  const previousState = useRef(listState);
  useEffect(() => {
    const prev = previousState.current;
    if (prev !== "failed" && listState === "failed") interactedBeforeFailure.current = interacted.current;
    if (prev === "failed" && listState !== "failed" && lastPressState.current === "failed") {
      interacted.current = interactedBeforeFailure.current;
      lastPressState.current = null;
    }
    previousState.current = listState;
  }, [listState]);

  useEffect(() => {
    if (!attempt || handled.current === attempt.id) return;
    // "failed" waits: a successful Retry moves the list to "loaded" and the attempt runs then.
    if (listState !== "loaded") return;
    handled.current = attempt.id;
    const row = document.getElementById(attempt.id);
    if (!row) return;
    if (!attempt.explicit && (interacted.current || !focusIsIdle())) return;
    row.focus({ preventScroll: true });
    row.scrollIntoView({ block: "center" });
  }, [attempt, listState]);
}
