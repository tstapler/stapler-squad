/**
 * Covers the real production wiring for the "backlog: <message>" trigger:
 * Omnibar.tsx's own `detection?.type === InputType.ChatBacklogItem` branch
 * (handleSubmit) opening BacklogItemIntentReview in place of the old
 * fire-and-close raw-text creation, and its onDone/onCancel callbacks.
 *
 * dispatch.test.ts's "parse_backlog_item" describe block covers a separate,
 * unreferenced dispatcher (dispatchOmnibarAction) — it does not exercise this
 * path at all, so this file is the only regression guard for the actual
 * shipped behavior.
 */

import React from "react";
import { screen, fireEvent, act, waitFor } from "@testing-library/react";
import {
  mockUsePathCompletions,
  mockUsePathHistory,
  mockUseAliases,
  defaultCompletions,
  makeHistoryFixture,
  renderOmnibar,
  typeAndDetect,
} from "./omnibarTestFixtures";
import { resetDefaultRegistry } from "@/lib/omnibar";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";

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

// BacklogItemIntentReview -> BacklogItemForm both call useBacklogService()
// directly; RepoPathInput (inside BacklogItemForm) needs these too.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: jest.fn(),
}));

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: jest.fn(() => false),
}));

const mockUseBacklogService = useBacklogService as jest.MockedFunction<typeof useBacklogService>;
const mockUseFeatureFlag = useFeatureFlag as jest.MockedFunction<typeof useFeatureFlag>;

function mockBacklogService(overrides: Partial<ReturnType<typeof useBacklogService>>) {
  mockUseBacklogService.mockReturnValue({
    listPipelineModes: () => Promise.resolve([]),
    parseBacklogItemIntent: jest.fn().mockResolvedValue(null),
    createBacklogItem: jest.fn().mockResolvedValue(null),
    ...overrides,
  } as unknown as ReturnType<typeof useBacklogService>);
}

const defaultHistory = makeHistoryFixture();

beforeEach(() => {
  jest.useFakeTimers();
  mockUsePathHistory.mockReturnValue(defaultHistory);
  mockUsePathCompletions.mockReturnValue(defaultCompletions);
  mockUseAliases.mockReturnValue({ aliases: [], loading: false, error: null, refetch: jest.fn() });
  mockUseFeatureFlag.mockReturnValue(false);
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

describe("Omnibar backlog: trigger opens BacklogItemIntentReview", () => {
  it("opens the review UI instead of creating immediately, and does not close the omnibar", async () => {
    mockBacklogService({ parseBacklogItemIntent: jest.fn().mockResolvedValue(null) });
    const onClose = jest.fn();
    const { input } = renderOmnibar({ onClose });

    await typeAndDetect(input, "backlog: fix the login timeout bug");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });

    expect(onClose).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(screen.getByTestId("backlog-intent-review-parse-failed-banner")).toBeInTheDocument()
    );
  });

  it("calls onDone's navigate-and-close path once the reviewed item is created", async () => {
    const createBacklogItem = jest.fn().mockResolvedValue({ item: { id: "item-1" }, triageTriggered: false });
    mockBacklogService({
      parseBacklogItemIntent: jest.fn().mockResolvedValue({
        title: "Fix login timeout",
        description: "Users are logged out too soon.",
        acceptanceCriteria: [],
        confidence: 0.9,
      }),
      createBacklogItem,
    });
    const onClose = jest.fn();
    const { input } = renderOmnibar({ onClose });

    await typeAndDetect(input, "backlog: fix the login timeout bug");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });

    const titleInput = await screen.findByDisplayValue("Fix login timeout");
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), { target: { value: "/repo" } });
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => expect(createBacklogItem).toHaveBeenCalled());
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(titleInput).toBeInTheDocument();
  });

  it("closes the review UI without creating anything on cancel", async () => {
    mockBacklogService({
      parseBacklogItemIntent: jest.fn().mockResolvedValue({
        title: "Fix login timeout",
        description: "Users are logged out too soon.",
        acceptanceCriteria: [],
        confidence: 0.9,
      }),
    });
    const { input } = renderOmnibar();

    await typeAndDetect(input, "backlog: fix the login timeout bug");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });

    await screen.findByDisplayValue("Fix login timeout");
    fireEvent.click(screen.getByTestId("backlog-form-cancel"));

    await waitFor(() =>
      expect(screen.queryByDisplayValue("Fix login timeout")).not.toBeInTheDocument()
    );
    // Back to the plain input, not creating a session.
    expect(screen.getByRole("combobox", { name: /session source input/i })).toBeInTheDocument();
  });
});
