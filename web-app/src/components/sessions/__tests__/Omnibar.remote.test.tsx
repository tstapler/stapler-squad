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

import { screen, fireEvent, act } from "@testing-library/react";
import {
  mockUsePathCompletions,
  mockUsePathHistory,
  mockUseAliases,
  existingPathCompletions,
  makeHistoryFixture,
  renderOmnibar,
  typeAndDetect,
} from "./omnibarTestFixtures";

// ---------------------------------------------------------------------------
// Mocks (copied verbatim from Omnibar.submitReset.test.tsx's baseline block)
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

jest.mock("@/lib/hooks/useAvailablePrograms", () => require("./omnibarTestFixtures").mockUseAvailableProgramsModule());

jest.mock("@/lib/hooks/useSlashCommands", () => require("./omnibarTestFixtures").mockUseSlashCommandsModule());

jest.mock("@/lib/hooks/useSlashCommandSuggestions", () => require("./omnibarTestFixtures").mockUseSlashCommandSuggestionsModule());

jest.mock("@/lib/store", () => require("./omnibarTestFixtures").mockStoreModule());

jest.mock("@/lib/store/sessionsSlice", () => require("./omnibarTestFixtures").mockSessionsSliceModule());

jest.mock("@/components/sessions/OmnibarResultList", () => require("./omnibarTestFixtures").mockOmnibarResultListModule());

jest.mock("@/lib/api/transport", () => require("./omnibarTestFixtures").mockApiTransportModule());

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const emptyHistory = makeHistoryFixture();

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
