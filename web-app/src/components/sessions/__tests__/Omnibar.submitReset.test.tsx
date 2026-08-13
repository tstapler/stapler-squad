/**
 * Regression tests for handleSubmit's SpawnShell and Alias branches leaving
 * isSubmitting stuck true after a successful onCreateSession call when
 * onClose() doesn't (or can't be proven to) unmount the modal.
 *
 * Root cause: those two branches previously reset isSubmitting only inside
 * `catch`, unlike the Default branch's `finally`. If onClose() didn't
 * synchronously dismiss the modal, the Create button stayed stuck on
 * "Creating..." forever even though the session was created successfully.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { Omnibar } from "../Omnibar";
import type { AliasEntry } from "@/lib/hooks/useAliases";
import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";
import { SessionType } from "@/gen/session/v1/types_pb";
import { getDefaultRegistry, resetDefaultRegistry } from "@/lib/omnibar";
import { AliasDetector } from "@/lib/omnibar/detectors/AliasDetector";

// ---------------------------------------------------------------------------
// Mocks (copied verbatim from Omnibar.alias.test.tsx's baseline block)
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
    complete: jest.fn((a: AliasEntry) => `@${a.name} `),
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

const SSQ_ALIAS: AliasEntry = {
  name: "ssq",
  group: "",
  path: "/home/user/projects/stapler-squad",
  description: "Stapler Squad project alias",
  profile: "",
  program: "claude",
  autoYes: false,
  tags: [],
  sessionType: SessionType.NEW_WORKTREE,
  namePrefix: "ssq-",
};

const defaultCompletions = {
  entries: [],
  baseDir: "/home/user",
  baseDirExists: false,
  pathExists: false,
  isLoading: false,
  error: null,
};

const defaultHistory = {
  getMatching: jest.fn((): PathHistoryEntry[] => []),
  getAll: jest.fn((): PathHistoryEntry[] => []),
  save: jest.fn(),
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderOmnibar(
  props: { onClose?: jest.Mock; onCreateSession?: jest.Mock; onNavigateToSession?: jest.Mock } = {}
) {
  const onClose = props.onClose ?? jest.fn();
  const onCreateSession = props.onCreateSession ?? jest.fn().mockResolvedValue(undefined);
  const onNavigateToSession = props.onNavigateToSession ?? jest.fn();
  const utils = render(
    <Omnibar
      isOpen={true}
      onClose={onClose}
      onCreateSession={onCreateSession}
      onNavigateToSession={onNavigateToSession}
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

// ---------------------------------------------------------------------------
// Tests
//
// Fake-timer setup and mock/registry cleanup are identical across all three
// describe blocks below, so they're hoisted to a file-level beforeEach/
// afterEach. Each describe's own beforeEach only sets what's distinct for
// that suite (path-completion fixture, alias data, detector registration).
// ---------------------------------------------------------------------------

beforeEach(() => {
  jest.useFakeTimers();
  mockUsePathHistory.mockReturnValue(defaultHistory);
  resetDefaultRegistry();
});

afterEach(() => {
  act(() => {
    jest.runOnlyPendingTimers();
  });
  jest.useRealTimers();
  jest.clearAllMocks();
  resetDefaultRegistry();
});

describe("Omnibar SpawnShell submit resets isSubmitting", () => {
  beforeEach(() => {
    mockUsePathCompletions.mockReturnValue(defaultCompletions);
    mockUseAliases.mockReturnValue({ aliases: [], loading: false, error: null, refetch: jest.fn() });
    // CommandDetector (which detects ">shell") is already in createDefaultRegistry(),
    // restored by the file-level resetDefaultRegistry() above — no manual registration needed.
  });

  it("allows a second submission after a successful SpawnShell create, even when onClose is a no-op", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const onClose = jest.fn(); // no-op: does not flip isOpen, reproducing the bug scenario
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, ">shell");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledTimes(1);
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Terminal", sessionType: "one_off" })
    );

    // Submit again with the same (still-detected) input. If isSubmitting were
    // stuck true (the bug), the `!canSubmit || isSubmitting` guard in
    // handleSubmit (Omnibar.tsx:1019) would silently no-op this second call.
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledTimes(2);
  });

  it("still allows resubmission after onCreateSession rejects (pre-existing catch-path reset, must not regress)", async () => {
    const onCreateSession = jest
      .fn()
      .mockRejectedValueOnce(new Error("boom"))
      .mockResolvedValueOnce(undefined);
    const onClose = jest.fn();
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, ">shell");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledTimes(1);

    // Retry after failure — the branch's catch already called setIsSubmitting(false)
    // before this fix; this asserts that behavior is preserved, not newly added.
    // Note: SpawnShell's `error` state has no visible DOM surface (OmnibarCreationPanel,
    // the only place `error` renders, never mounts while SpawnShell stays in discovery
    // mode) — this is pre-existing, out-of-scope behavior, not asserted here.
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledTimes(2);
  });
});

describe("Omnibar Alias submit resets isSubmitting", () => {
  beforeEach(() => {
    mockUsePathCompletions.mockReturnValue(defaultCompletions);
    mockUseAliases.mockReturnValue({ aliases: [SSQ_ALIAS], loading: false, error: null, refetch: jest.fn() });
    getDefaultRegistry().register(new AliasDetector([SSQ_ALIAS]));
  });

  it("re-enables both Create Session buttons after a successful alias create, even when onClose is a no-op", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const onClose = jest.fn(); // no-op: does not flip isOpen
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, "@ssq my-feature");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "ssq-my-feature", aliasName: "ssq" })
    );

    // Alias IS a CREATION_TYPE (useModeReducer.ts:23), so both Create Session
    // buttons (OmnibarCreationPanel footer + Omnibar.tsx shortcuts-bar) render.
    const buttons = screen.getAllByRole("button", { name: /create session/i });
    expect(buttons.length).toBeGreaterThan(0);
    for (const btn of buttons) {
      expect(btn).toBeEnabled();
    }
  });

  it("does not show an error message when the Alias submission succeeds normally", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const onClose = jest.fn();
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, "@ssq my-feature");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    // onClose fired exactly once — no retry/re-entry into the error branch.
    expect(onClose).toHaveBeenCalledTimes(1);
    // The only place error text renders is OmnibarCreationPanel.tsx's
    // {error && <div className={errorClass}>{error}</div>}; assert it's absent.
    expect(screen.queryByText(/failed to create session/i)).not.toBeInTheDocument();
  });

  it("surfaces the error and re-enables both buttons when onCreateSession rejects (must not regress)", async () => {
    const onCreateSession = jest.fn().mockRejectedValueOnce(new Error("boom"));
    const onClose = jest.fn();
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, "@ssq my-feature");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    // error surfaces via OmnibarCreationPanel.tsx ({error && <div>{error}</div>}),
    // which IS rendered here since Alias is a CREATION_TYPE.
    expect(screen.getByText("boom")).toBeInTheDocument();

    const buttons = screen.getAllByRole("button", { name: /create session/i });
    for (const btn of buttons) {
      expect(btn).toBeEnabled();
    }
  });
});

describe("Omnibar defense-in-depth reset on close", () => {
  // pathExists: true so the Default/LocalPath branch's canSubmit isn't blocked by the
  // unrelated "path does not exist" gate (Omnibar.tsx's canSubmit, new_worktree/directory
  // sessionType) — this test isolates isSubmitting's reset behavior, not path validation.
  const existingPathCompletions = { ...defaultCompletions, pathExists: true, baseDirExists: true };

  beforeEach(() => {
    mockUsePathCompletions.mockReturnValue(existingPathCompletions);
    mockUseAliases.mockReturnValue({ aliases: [], loading: false, error: null, refetch: jest.fn() });
  });

  it("does not leave isSubmitting stuck if the modal is closed while a submission is in flight", async () => {
    let resolveCreate!: () => void;
    const heldPromise = new Promise<void>((resolve) => {
      resolveCreate = resolve;
    });
    const onCreateSession = jest.fn().mockReturnValue(heldPromise);
    const { input, rerender } = renderOmnibar({ onCreateSession });

    await typeAndDetect(input, "/home/user/projects");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    // isSubmitting is now true; onCreateSession's promise is still unresolved.

    // Parent force-closes the modal (isOpen: true -> false) without the
    // in-flight submission ever resolving.
    rerender(
      <Omnibar isOpen={false} onClose={jest.fn()} onCreateSession={onCreateSession} onNavigateToSession={jest.fn()} />
    );
    // Reopen.
    rerender(
      <Omnibar isOpen={true} onClose={jest.fn()} onCreateSession={onCreateSession} onNavigateToSession={jest.fn()} />
    );

    const reopenedInput = screen.getByRole("combobox", { name: /session source input/i });
    await typeAndDetect(reopenedInput, "/home/user/projects");

    const buttons = screen.getAllByRole("button", { name: /create session/i });
    for (const btn of buttons) {
      expect(btn).toBeEnabled();
    }

    // Clean up the dangling promise so it doesn't leak into a later test.
    await act(async () => {
      resolveCreate();
    });
  });
});
