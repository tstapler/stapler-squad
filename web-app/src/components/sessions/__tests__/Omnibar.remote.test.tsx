/**
 * Story 4.3.1c (ssh-remote-workspaces Epic 4.3): OmnibarFormState.remoteName's
 * compose-with-sessionType submit path.
 *
 * Mirrors Omnibar.submitReset.test.tsx's boilerplate and render-real-Omnibar approach
 * (rather than dispatch.test.ts's dispatchOmnibarAction, which is an unrelated
 * search-result-action dispatcher that never sees OmnibarFormState/handleSubmit) — the
 * behavior under test lives entirely in Omnibar.tsx's own handleSubmit.
 *
 * Covers plan.md Story 4.3.1's two ACs:
 *   (a) remoteName set -> OmnibarSessionData carries both sessionType and remoteName.
 *   (b) remoteName unset (no remotes configured, selector absent) -> OmnibarSessionData
 *       omits remoteName entirely, byte-identical to pre-change local behavior.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { Omnibar } from "../Omnibar";
import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";

// ---------------------------------------------------------------------------
// Mocks (copied verbatim from Omnibar.submitReset.test.tsx's baseline block)
// ---------------------------------------------------------------------------

const mockUsePathCompletions = jest.fn();
const mockUsePathHistory = jest.fn();
const mockUseAliases = jest.fn();

jest.mock("next/navigation", () => ({
  usePathname: jest.fn(),
  useRouter: jest.fn(() => ({ push: jest.fn(), replace: jest.fn() })),
}));

jest.mock("@/lib/contexts/ThemeContext", () => ({
  useTheme: jest.fn(() => ({ setTheme: jest.fn(), theme: "dark" })),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: jest.fn(() => "http://localhost:8543"),
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: (...args: unknown[]) => mockUsePathCompletions(...args),
  clearCompletionCache: jest.fn(),
}));

jest.mock("@/lib/hooks/usePathHistory", () => ({
  usePathHistory: (...args: unknown[]) => mockUsePathHistory(...args),
  clearPathHistory: jest.fn(),
}));

jest.mock("@/lib/hooks/useSessionSearch", () => ({
  useSessionSearch: jest.fn(() => []),
}));

jest.mock("@/lib/hooks/useWorktreeSuggestions", () => ({
  useWorktreeSuggestions: jest.fn(() => ({ worktrees: [], isLoading: false })),
}));

jest.mock("@/lib/hooks/useAliases", () => ({
  useAliases: (...args: unknown[]) => mockUseAliases(...args),
}));

jest.mock("@/lib/hooks/useAliasSuggestions", () => ({
  useAliasSuggestions: jest.fn(() => ({
    isAliasBrowse: false,
    isAliasCompletion: false,
    filteredAliases: [],
    complete: jest.fn(),
  })),
}));

jest.mock("@/lib/hooks/useAtCommandSuggestions", () => ({
  useAtCommandSuggestions: jest.fn(() => ({
    isAtCommand: false,
    suggestions: [],
    complete: jest.fn(),
  })),
}));

jest.mock("@/lib/hooks/useAvailablePrograms", () => ({
  useAvailablePrograms: jest.fn(() => []),
}));

jest.mock("@/lib/hooks/useSlashCommands", () => ({
  useSlashCommands: jest.fn(() => ({ commands: [] })),
}));

jest.mock("@/lib/hooks/useSlashCommandSuggestions", () => ({
  useSlashCommandSuggestions: jest.fn(() => ({
    isActive: false,
    suggestions: [],
    complete: jest.fn(),
  })),
}));

jest.mock("@/lib/store", () => ({
  useAppSelector: jest.fn(() => []),
}));

jest.mock("@/lib/store/sessionsSlice", () => ({
  selectAllSessions: jest.fn(),
  selectActiveSessionsSortedByUpdatedAt: jest.fn(),
}));

jest.mock("@/components/sessions/OmnibarResultList", () => ({
  OmnibarResultList: () => null,
  getResultListItemCount: jest.fn(() => 0),
  getHighlightedItemId: jest.fn(() => undefined),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn(() => ({})),
}));

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// pathExists: true so canSubmit's "new_worktree" branch isn't blocked by the unrelated
// "path does not exist" gate — this test isolates remoteName pass-through, not path validation.
const existingPathCompletions = {
  entries: [],
  baseDir: "/home/user",
  baseDirExists: true,
  pathExists: true,
  isLoading: false,
  error: null,
};

const emptyHistory = {
  getMatching: jest.fn((): PathHistoryEntry[] => []),
  getAll: jest.fn((): PathHistoryEntry[] => []),
  save: jest.fn(),
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderOmnibar(
  props: { onCreateSession?: jest.Mock; remotes?: { name: string }[] } = {}
) {
  const onClose = jest.fn();
  const onCreateSession = props.onCreateSession ?? jest.fn().mockResolvedValue(undefined);
  const onNavigateToSession = jest.fn();
  const utils = render(
    <Omnibar
      isOpen={true}
      onClose={onClose}
      onCreateSession={onCreateSession}
      onNavigateToSession={onNavigateToSession}
      remotes={props.remotes}
    />
  );
  const input = screen.getByRole("combobox", { name: /session source input/i });
  return { ...utils, input, onClose, onCreateSession, onNavigateToSession };
}

/** Type a value into the omnibar input and wait for the 150ms detect debounce plus React state flush. */
async function typeAndDetect(input: Element, value: string) {
  fireEvent.change(input, { target: { value } });
  await act(async () => {
    jest.advanceTimersByTime(200);
  });
}

beforeEach(() => {
  jest.useFakeTimers();
  mockUsePathHistory.mockReturnValue(emptyHistory);
  mockUsePathCompletions.mockReturnValue(existingPathCompletions);
  mockUseAliases.mockReturnValue({ aliases: [], loading: false, error: null, refetch: jest.fn() });
});

afterEach(() => {
  act(() => {
    jest.runOnlyPendingTimers();
  });
  jest.useRealTimers();
  jest.clearAllMocks();
});

describe("Omnibar remoteName submit path (Epic 4.3 Story 4.3.1)", () => {
  it("includes both sessionType and remoteName when a remote is selected", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const { input } = renderOmnibar({ onCreateSession, remotes: [{ name: "prod-box" }] });

    await typeAndDetect(input, "/home/user/projects");

    const remoteSelect = screen.getByTestId("remote-selector");
    fireEvent.change(remoteSelect, { target: { value: "prod-box" } });

    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ sessionType: "new_worktree", remoteName: "prod-box" })
    );
  });

  it("omits remoteName entirely when unset — byte-identical to pre-change local behavior", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    // No remotes configured -> selector never renders, remoteName stays at its
    // INITIAL_FORM_STATE default (undefined).
    const { input } = renderOmnibar({ onCreateSession, remotes: [] });

    expect(screen.queryByTestId("remote-selector")).not.toBeInTheDocument();

    await typeAndDetect(input, "/home/user/projects");

    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    expect(onCreateSession).toHaveBeenCalledTimes(1);
    // remoteName is `undefined` (never a populated string) -- the object-literal key may
    // still be present with that value (matching the existing autonomousMode/permissionMode
    // pattern), but JSON/protobuf serialization omits an undefined field entirely, so the
    // resulting CreateSessionRequest never carries a `remote` field for a local session.
    const sessionData = onCreateSession.mock.calls[0][0];
    expect(sessionData.remoteName).toBeUndefined();
  });
});
