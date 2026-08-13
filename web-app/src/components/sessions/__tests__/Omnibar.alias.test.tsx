/**
 * Omnibar alias namePrefix population tests.
 *
 * Tests that when an alias with a namePrefix is detected (e.g. "@ssq my-feature"),
 * the session name field is populated with:
 * - namePrefix + typedLabel when a label is typed (e.g. "ssq-my-feature")
 * - just namePrefix when no label is typed yet (e.g. "ssq-")
 * - the user's manual entry is not clobbered
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { Omnibar } from "../Omnibar";
import type { AliasEntry } from "@/lib/hooks/useAliases";
import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";
import { SessionType } from "@/gen/session/v1/types_pb";
import { getDefaultRegistry, resetDefaultRegistry, detect as realDetect } from "@/lib/omnibar";
import { AliasDetector } from "@/lib/omnibar/detectors/AliasDetector";

// Omnibar.tsx imports `detect` from the "@/lib/omnibar" barrel. Mock just that
// export (passing through to the real implementation by default) so a single
// test can make detection throw for a specific input and exercise the
// detection debounce effect's try/catch in Omnibar.tsx — DetectorRegistry.detect()
// already swallows individual detector exceptions internally, so a throwing
// Detector class registered into the registry never reaches Omnibar.tsx's own
// catch; mocking `detect` itself is the only way to simulate that failure mode.
jest.mock("@/lib/omnibar", () => {
  const actual = jest.requireActual("@/lib/omnibar");
  return {
    ...actual,
    detect: jest.fn((input: string) => actual.detect(input)),
  };
});

// ---------------------------------------------------------------------------
// Mocks
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
// ---------------------------------------------------------------------------

describe("Omnibar alias namePrefix population", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockUsePathCompletions.mockReturnValue(defaultCompletions);
    mockUsePathHistory.mockReturnValue(defaultHistory);
    mockUseAliases.mockReturnValue({
      aliases: [SSQ_ALIAS],
      loading: false,
      error: null,
      refetch: jest.fn(),
    });

    // Register AliasDetector into the global registry so detect("@ssq ...") resolves
    // to InputType.Alias. In production this is done by OmnibarContext, but in unit
    // tests we render Omnibar directly without that context wrapper.
    resetDefaultRegistry();
    getDefaultRegistry().register(new AliasDetector([SSQ_ALIAS]));
  });

  afterEach(() => {
    act(() => {
      jest.runOnlyPendingTimers();
    });
    jest.useRealTimers();
    jest.clearAllMocks();
    // Restore the registry to its default state for other tests.
    resetDefaultRegistry();
  });

  // InputType.Alias IS a CREATION_TYPE (useModeReducer.ts), so detecting an alias
  // switches the omnibar into creation mode and renders OmnibarCreationPanel. The
  // namePrefix population logic runs and updates formState.sessionName, which is then
  // used as the session title when the user submits. We verify the population logic by
  // submitting via Cmd+Enter and asserting the title passed to onCreateSession.

  it("populates session name with prefix+label when alias has namePrefix and label is typed", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const { input } = renderOmnibar({ onCreateSession });

    // Type "@ssq my-feature" — alias invocation with label
    await typeAndDetect(input, "@ssq my-feature");

    // Submit via Ctrl+Enter (works in both discovery and creation mode)
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    // onCreateSession should have been called with title reflecting the alias namePrefix + label
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "ssq-my-feature" })
    );
  });

  it("populates session name with bare prefix when alias has namePrefix and no label typed yet", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const { input } = renderOmnibar({ onCreateSession });

    // Type "@ssq " (trailing space, no label) — alias invoked with empty label
    await typeAndDetect(input, "@ssq ");

    // Submit via Ctrl+Enter
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    // onCreateSession title should be the bare prefix "ssq-"
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "ssq-" })
    );
  });

  it("populates session name with prefix+label when alias has namePrefix, branch, and label all set", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const { input } = renderOmnibar({ onCreateSession });

    // Type "@ssq:main my-feature" — alias with branch and label (three-field combination)
    await typeAndDetect(input, "@ssq:main my-feature");

    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    // Session name should reflect namePrefix + label; branch is passed separately
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "ssq-my-feature",
        branch: "main",
      })
    );
  });

  it("does not override a manually typed session name with prefix+label", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const { input } = renderOmnibar({ onCreateSession });

    // First: switch to creation mode so the session name field is visible,
    // by typing a local path to enter creation mode.
    await typeAndDetect(input, "/home/user/projects");

    // Manually set session name field to a custom value.
    const sessionNameField = screen.getByRole("textbox", { name: /session name/i });
    fireEvent.change(sessionNameField, { target: { value: "my-custom-name" } });

    // Now switch to alias invocation — the namePrefix logic should NOT clobber the manual name.
    await typeAndDetect(input, "@ssq my-feature");

    // Submit via Ctrl+Enter and verify the manual name was preserved.
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "my-custom-name" })
    );
  });
});

describe("Omnibar detection re-run on async alias load (AC0)", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockUsePathCompletions.mockReturnValue(defaultCompletions);
    mockUsePathHistory.mockReturnValue(defaultHistory);
    // Simulate the alias list not having resolved yet — same as OmnibarContext.tsx
    // before its useAliases() fetch completes and registers AliasDetector.
    mockUseAliases.mockReturnValue({
      aliases: [],
      loading: true,
      error: null,
      refetch: jest.fn(),
    });
    // No AliasDetector registered yet — mirrors production before OmnibarContext's
    // effect has registered it. Reset to the plain default set (no dynamic detectors).
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

  it("re-evaluates and unblocks submission once aliases finish loading, without an extra keystroke", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const { input, rerender, getByRole } = renderOmnibar({ onCreateSession });

    // Type "@ssq my-feature" before the alias list has resolved — AliasDetector
    // isn't registered yet, so this currently mis-detects as a bare SessionSearch
    // (not submittable).
    await typeAndDetect(input, "@ssq my-feature");

    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).not.toHaveBeenCalled();

    // Alias list resolves: OmnibarContext would register AliasDetector at this point.
    getDefaultRegistry().register(new AliasDetector([SSQ_ALIAS]));
    mockUseAliases.mockReturnValue({
      aliases: [SSQ_ALIAS],
      loading: false,
      error: null,
      refetch: jest.fn(),
    });

    // Re-render with unchanged props so Omnibar re-invokes useAliases() and picks
    // up the resolved list — no additional typing/keystroke from the user.
    rerender(
      <Omnibar
        isOpen={true}
        onClose={jest.fn()}
        onCreateSession={onCreateSession}
        onNavigateToSession={jest.fn()}
      />
    );

    await act(async () => {
      jest.advanceTimersByTime(200);
    });

    await act(async () => {
      fireEvent.keyDown(getByRole("combobox", { name: /session source input/i }), {
        key: "Enter",
        ctrlKey: true,
      });
    });

    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "ssq-my-feature" })
    );
  });
});

// The detection debounce effect (Omnibar.tsx) wraps its whole body in try/catch,
// with the catch calling setError(...). This exercises that path — previously
// untested — by making the mocked `detect` (imported from the "@/lib/omnibar"
// barrel, see jest.mock above) throw for a specific input.
describe("Omnibar detection effect error handling", () => {
  const THROW_TRIGGER = "/trigger-detector-throw";
  const THROW_MESSAGE = "Detector exploded";

  const mockedDetect = realDetect as jest.MockedFunction<typeof realDetect>;

  let consoleErrorSpy: jest.SpyInstance;

  beforeEach(() => {
    jest.useFakeTimers();
    mockUsePathCompletions.mockReturnValue(defaultCompletions);
    mockUsePathHistory.mockReturnValue(defaultHistory);
    mockUseAliases.mockReturnValue({
      aliases: [],
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    consoleErrorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    resetDefaultRegistry();
    // Only throws for THROW_TRIGGER — normal inputs (e.g. a plain local path)
    // still detect correctly (delegates to the real implementation) so we can
    // first reach creation mode before forcing the throw.
    mockedDetect.mockImplementation((input: string) => {
      if (input === THROW_TRIGGER) {
        throw new Error(THROW_MESSAGE);
      }
      return jest.requireActual("@/lib/omnibar").detect(input);
    });
  });

  afterEach(() => {
    act(() => {
      jest.runOnlyPendingTimers();
    });
    jest.useRealTimers();
    consoleErrorSpy.mockRestore();
    mockedDetect.mockReset();
    jest.clearAllMocks();
    resetDefaultRegistry();
  });

  it("shows an error and keeps the Create button in a sane state instead of crashing when detection throws", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const { input } = renderOmnibar({ onCreateSession });

    // First reach creation mode with a normal, non-throwing input.
    await typeAndDetect(input, "/home/user/projects");

    // Omnibar renders two "Create Session" buttons that both key off canSubmit —
    // the footer button (OmnibarCreationPanel) and the shortcuts-bar button
    // (Omnibar.tsx). Both must stay in sync before and after the throw.
    const disabledBeforeThrow = screen
      .getAllByRole("button", { name: /create session/i })
      .map((btn) => btn.hasAttribute("disabled"));

    // Now type an input that makes the registered detector throw during the
    // debounced detection effect.
    await typeAndDetect(input, THROW_TRIGGER);

    // The component must not crash/unmount — the input and Create button(s) are
    // still present and interactive.
    expect(screen.getByRole("combobox", { name: /session source input/i })).toBeInTheDocument();
    const createButtonsAfterThrow = screen.getAllByRole("button", { name: /create session/i });
    expect(createButtonsAfterThrow.length).toBeGreaterThan(0);

    // An error message surfaced to the user.
    expect(screen.getByText(THROW_MESSAGE)).toBeInTheDocument();
    expect(consoleErrorSpy).toHaveBeenCalled();

    // canSubmit/Create button state is left as it was prior to the throw (the
    // stale-but-valid detection from before), not stuck in some undefined/broken
    // state — clicking it should not throw and should not silently no-op into
    // calling onCreateSession with garbage data.
    const disabledAfterThrow = createButtonsAfterThrow.map((btn) => btn.hasAttribute("disabled"));
    expect(disabledAfterThrow).toEqual(disabledBeforeThrow);
  });
});
