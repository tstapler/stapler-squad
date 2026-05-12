/**
 * Omnibar discovery mode integration tests.
 *
 * Covers the P1 pitfall: Enter on a highlighted session result must call
 * onNavigateToSession, NOT onCreateSession. This was flagged as untested
 * in the shipping readiness review (validation.md T-INT-001).
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { Omnibar } from "../Omnibar";
import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";
import type { SessionSearchResult } from "@/lib/hooks/useSessionSearch";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockUsePathCompletions = jest.fn();
const mockUsePathHistory = jest.fn();
const mockUseSessionSearch = jest.fn();
const mockUseAppSelector = jest.fn();

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: (...args: unknown[]) => mockUsePathCompletions(...args),
  clearCompletionCache: jest.fn(),
}));

jest.mock("@/lib/hooks/usePathHistory", () => ({
  usePathHistory: (...args: unknown[]) => mockUsePathHistory(...args),
  clearPathHistory: jest.fn(),
}));

jest.mock("@/lib/hooks/useSessionSearch", () => ({
  useSessionSearch: (...args: unknown[]) => mockUseSessionSearch(...args),
}));

jest.mock("@/lib/hooks/useWorktreeSuggestions", () => ({
  useWorktreeSuggestions: jest.fn(() => ({ worktrees: [] })),
}));

jest.mock("@/lib/store", () => ({
  useAppSelector: (...args: unknown[]) => mockUseAppSelector(...args),
}));

jest.mock("@/lib/store/sessionsSlice", () => ({
  selectAllSessions: jest.fn(),
}));

jest.mock("@/components/sessions/OmnibarResultList", () => ({
  OmnibarResultList: () => null,
  // Return 2 so ArrowDown can reach index 0 (1 session + 0 repos + 1 create-new).
  getResultListItemCount: jest.fn((sessionCount: number, repoCount: number) => sessionCount + repoCount + 1),
  getHighlightedItemId: jest.fn(() => undefined),
}));

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const SESSION_1 = {
  id: "session-abc",
  title: "My Feature Branch",
  status: 1, // RUNNING – passes the UNSPECIFIED filter
  branch: "feature/my-feature",
  path: "/home/user/project",
  tags: [] as string[],
  updatedAt: { seconds: BigInt(1_700_000_000), nanos: 0 },
};

const DEFAULT_COMPLETIONS = {
  entries: [],
  baseDir: "/home/user",
  baseDirExists: false,
  pathExists: false,
  isLoading: false,
  error: null,
};

const DEFAULT_HISTORY = {
  getMatching: jest.fn((): PathHistoryEntry[] => []),
  getAll: jest.fn((): PathHistoryEntry[] => []),
  save: jest.fn(),
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderOmnibar(
  props: {
    onClose?: jest.Mock;
    onCreateSession?: jest.Mock;
    onNavigateToSession?: jest.Mock;
  } = {}
) {
  const onClose = props.onClose ?? jest.fn();
  const onCreateSession = props.onCreateSession ?? jest.fn().mockResolvedValue(undefined);
  const onNavigateToSession = props.onNavigateToSession ?? jest.fn();
  render(
    <Omnibar
      isOpen={true}
      onClose={onClose}
      onCreateSession={onCreateSession}
      onNavigateToSession={onNavigateToSession}
    />
  );
  const input = screen.getByRole("combobox", { name: /session source input/i });
  return { input, onClose, onCreateSession, onNavigateToSession };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("Omnibar discovery mode", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockUsePathCompletions.mockReturnValue(DEFAULT_COMPLETIONS);
    mockUsePathHistory.mockReturnValue(DEFAULT_HISTORY);
    mockUseSessionSearch.mockReturnValue([] as SessionSearchResult[]);
    // Default: one session in the store so the empty-state list is non-empty.
    mockUseAppSelector.mockReturnValue([SESSION_1]);
  });

  afterEach(() => {
    act(() => {
      jest.runOnlyPendingTimers();
    });
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  // -------------------------------------------------------------------------
  // P1 Pitfall: Enter routing (T-INT-001)
  // -------------------------------------------------------------------------

  describe("P1 pitfall: Enter key routing", () => {
    it("ArrowDown + Enter calls onNavigateToSession with session id, NOT onCreateSession", async () => {
      const { input, onNavigateToSession, onCreateSession } = renderOmnibar();

      // Omnibar opens in discovery mode with SESSION_1 in empty-state list.
      // ArrowDown selects the first result (resultHighlightIndex 0).
      fireEvent.keyDown(input, { key: "ArrowDown" });

      // Enter should navigate, not create.
      fireEvent.keyDown(input, { key: "Enter" });

      expect(onNavigateToSession).toHaveBeenCalledTimes(1);
      expect(onNavigateToSession).toHaveBeenCalledWith(SESSION_1.id);
      expect(onCreateSession).not.toHaveBeenCalled();
    });

    it("Enter with no selection does NOT call onNavigateToSession", async () => {
      const { input, onNavigateToSession } = renderOmnibar();

      // Enter without ArrowDown first (resultHighlightIndex = -1).
      fireEvent.keyDown(input, { key: "Enter" });

      expect(onNavigateToSession).not.toHaveBeenCalled();
    });

    it("Cmd+Enter does NOT trigger session navigation", async () => {
      const { input, onNavigateToSession } = renderOmnibar();

      fireEvent.keyDown(input, { key: "ArrowDown" });
      // Cmd+Enter is the creation shortcut, not the navigation shortcut.
      fireEvent.keyDown(input, { key: "Enter", metaKey: true });

      expect(onNavigateToSession).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Mode transitions: Escape
  // -------------------------------------------------------------------------

  describe("Escape navigation", () => {
    it("Escape in discovery mode closes the omnibar", async () => {
      const { input, onClose } = renderOmnibar();

      fireEvent.keyDown(input, { key: "Escape" });

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("Escape with selection highlighted clears selection before closing", async () => {
      const { input, onClose } = renderOmnibar();

      fireEvent.keyDown(input, { key: "ArrowDown" });
      // First Escape: clears selection (resultHighlightIndex → -1).
      fireEvent.keyDown(input, { key: "Escape" });
      expect(onClose).not.toHaveBeenCalled();
      // Second Escape: closes.
      fireEvent.keyDown(input, { key: "Escape" });
      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });

  // -------------------------------------------------------------------------
  // Session search updates on every keystroke (immediate, not debounced)
  // -------------------------------------------------------------------------

  describe("session search immediacy", () => {
    it("useSessionSearch receives query without waiting for debounce", () => {
      const { input } = renderOmnibar();

      // Type bare text – should pass to Fuse immediately (no delay needed).
      fireEvent.change(input, { target: { value: "feat" } });

      // useSessionSearch should have been called with "feat" synchronously.
      expect(mockUseSessionSearch).toHaveBeenCalledWith("feat");
    });

    it("path input does NOT pass to session search", () => {
      const { input } = renderOmnibar();

      // Typing a path should NOT trigger session search.
      fireEvent.change(input, { target: { value: "/home/user/proj" } });

      // After the change, useSessionSearch should be called with "" (empty).
      const lastCall = mockUseSessionSearch.mock.calls[mockUseSessionSearch.mock.calls.length - 1];
      expect(lastCall[0]).toBe("");
    });
  });

  describe("canSubmit for SessionSearch input", () => {
    it("Cmd+Enter with bare-text query does not call onCreateSession", async () => {
      const { input, onCreateSession } = renderOmnibar();

      fireEvent.change(input, { target: { value: "squad" } });
      await act(async () => {
        jest.advanceTimersByTime(200);
      });

      fireEvent.keyDown(input, { key: "Enter", metaKey: true });

      expect(onCreateSession).not.toHaveBeenCalled();
    });
  });
});
