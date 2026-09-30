/**
 * Omnibar SpawnShell (>shell ...) error surfacing tests.
 *
 * InputType.SpawnShell is excluded from useModeReducer.ts's CREATION_TYPES,
 * so the omnibar never leaves discovery mode for a `>shell ...` submission —
 * meaning OmnibarCreationPanel (the only place `error` previously rendered)
 * never mounts. These tests cover the discovery-mode error render path added
 * to fix that: a failed `>shell` submission must show a visible, accessible
 * error while staying in discovery mode.
 */

import { screen, fireEvent, act } from "@testing-library/react";
import {
  mockUsePathCompletions,
  mockUsePathHistory,
  mockUseAliases,
  defaultCompletions,
  makeHistoryFixture,
  renderOmnibar,
  typeAndDetect,
} from "./omnibarTestFixtures";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

jest.mock("next/navigation", () => require("./omnibarTestFixtures").mockNextNavigationModule());

jest.mock("@/lib/contexts/ThemeContext", () => require("./omnibarTestFixtures").mockThemeContextModule());

jest.mock("@/lib/config", () => require("./omnibarTestFixtures").mockConfigModule());

jest.mock("@/lib/hooks/usePathCompletions", () => require("./omnibarTestFixtures").mockUsePathCompletionsModule());

jest.mock("@/lib/hooks/usePathHistory", () => require("./omnibarTestFixtures").mockUsePathHistoryModule());

jest.mock("@/lib/hooks/useSessionSearch", () => require("./omnibarTestFixtures").mockUseSessionSearchModule());

jest.mock("@/lib/hooks/useWorktreeSuggestions", () => require("./omnibarTestFixtures").mockUseWorktreeSuggestionsModule());

jest.mock("@/lib/hooks/useAliases", () => require("./omnibarTestFixtures").mockUseAliasesModule());

jest.mock("@/lib/hooks/useAliasSuggestions", () => require("./omnibarTestFixtures").mockUseAliasSuggestionsModule());

jest.mock("@/lib/hooks/useAtCommandSuggestions", () => require("./omnibarTestFixtures").mockUseAtCommandSuggestionsModule());

jest.mock("@/lib/store", () => require("./omnibarTestFixtures").mockStoreModule());

jest.mock("@/lib/store/sessionsSlice", () => require("./omnibarTestFixtures").mockSessionsSliceModule());

jest.mock("@/components/sessions/OmnibarResultList", () => require("./omnibarTestFixtures").mockOmnibarResultListModule());

jest.mock("@/lib/api/transport", () => require("./omnibarTestFixtures").mockApiTransportModule());

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const defaultHistory = makeHistoryFixture();

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function submit(input: Element) {
  await act(async () => {
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
  });
}

describe("Omnibar SpawnShell error surfacing", () => {
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
  });

  afterEach(() => {
    act(() => {
      jest.runOnlyPendingTimers();
    });
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("shows a visible, announced error and stays in discovery mode when a >shell submission fails", async () => {
    const onCreateSession = jest.fn().mockRejectedValue(new Error("spawn failed: boom"));
    const { input, onClose } = renderOmnibar({ onCreateSession });

    await typeAndDetect(input, ">shell -- ls");
    await submit(input);

    expect(onCreateSession).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();

    // Error is visible and announced.
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("spawn failed: boom");
    expect(alert).toHaveAttribute("aria-live", "assertive");
    expect(screen.getByTestId("spawn-shell-error")).toBe(alert);

    // Still in discovery mode — OmnibarCreationPanel's form fields never mounted.
    expect(screen.queryByRole("textbox", { name: /session name/i })).not.toBeInTheDocument();
  });

  it("clears the error once the user edits the input to a new, unrelated query", async () => {
    const onCreateSession = jest.fn().mockRejectedValue(new Error("boom"));
    const { input } = renderOmnibar({ onCreateSession });

    await typeAndDetect(input, ">shell -- ls");
    await submit(input);
    expect(screen.getByRole("alert")).toBeInTheDocument();

    // User edits the input to something unrelated.
    await typeAndDetect(input, "some other search");

    expect(screen.queryByTestId("spawn-shell-error")).not.toBeInTheDocument();
    expect(screen.queryByText("boom")).not.toBeInTheDocument();
  });

  it("clears the error on input edit even when the edit keeps SpawnShell detection active", async () => {
    // Regression pin: typing a *different* query normally leaves SpawnShell
    // detection (dispatchMode "detect") and unmounts the whole chip+error
    // block, which would make the error disappear regardless of whether
    // setError(null) actually ran. Stay on a `>shell` input throughout so the
    // block stays mounted and only the error's own clearing is exercised.
    const onCreateSession = jest.fn().mockRejectedValue(new Error("boom"));
    const { input } = renderOmnibar({ onCreateSession });

    await typeAndDetect(input, ">shell -- ls");
    await submit(input);
    expect(screen.getByRole("alert")).toBeInTheDocument();

    await typeAndDetect(input, ">shell -- pwd");

    expect(screen.getByTestId("spawn-shell-chip")).toBeInTheDocument();
    expect(screen.queryByTestId("spawn-shell-error")).not.toBeInTheDocument();
  });

  it("does not render the error for a non-SpawnShell detection type", async () => {
    // Pins the error block's InputType.SpawnShell gate against a future
    // refactor that might hoist it out of that conditional.
    const onCreateSession = jest.fn().mockRejectedValue(new Error("boom"));
    const { input } = renderOmnibar({ onCreateSession });

    await typeAndDetect(input, ">shell -- ls");
    await submit(input);
    expect(screen.getByTestId("spawn-shell-error")).toBeInTheDocument();

    // Switch to a plain local-path input — different detection type, enters
    // creation mode. The stale SpawnShell error must not leak into it.
    await typeAndDetect(input, "/home/user/projects");

    expect(screen.queryByTestId("spawn-shell-error")).not.toBeInTheDocument();
  });

  it("clears the error when clicking a recent-command chip, which sets input directly (bypassing onChange)", async () => {
    // Regression pin: recent-command chips call setInput() directly
    // (Omnibar.tsx's recentCommands button onClick), not the <input>'s
    // onChange handler — an onChange-only clear misses this path entirely.
    window.localStorage.setItem("ssq.recentShellCommands", JSON.stringify(["ls -la"]));

    const onCreateSession = jest.fn().mockRejectedValue(new Error("boom"));
    const { input } = renderOmnibar({ onCreateSession });

    // Bare ">shell" (no dir/command) shows the recent-commands list.
    await typeAndDetect(input, ">shell");
    await submit(input);
    expect(screen.getByTestId("spawn-shell-error")).toBeInTheDocument();

    const recentCommandButton = screen.getByTestId("spawn-shell-recent-command");
    await act(async () => {
      fireEvent.click(recentCommandButton);
    });

    expect(screen.queryByTestId("spawn-shell-error")).not.toBeInTheDocument();

    window.localStorage.removeItem("ssq.recentShellCommands");
  });

  it("does not render an error and closes on a successful >shell submission", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const { input, onClose } = renderOmnibar({ onCreateSession });

    await typeAndDetect(input, ">shell -- ls");
    await submit(input);

    expect(onCreateSession).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
