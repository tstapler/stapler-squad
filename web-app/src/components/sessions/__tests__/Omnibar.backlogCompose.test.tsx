/**
 * Compose mode (dedicated textarea) opened by the omnibar's "Create Backlog
 * Item" button: text preservation, immediate close + background creation,
 * and no regression to the "backlog: " prefix path (see
 * Omnibar.backlogIntentReview.test.tsx).
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

const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  usePathname: jest.fn(),
  useRouter: () => ({ push: mockPush, replace: jest.fn() }),
}));

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

// Unlike the shared fixture (renders null), expose the button the user clicks.
jest.mock("@/components/sessions/OmnibarResultList", () => ({
  OmnibarResultList: ({ onCreateBacklog }: { onCreateBacklog?: () => void }) => (
    <button type="button" data-testid="mock-create-backlog" onClick={onCreateBacklog}>
      Create Backlog Item (AI)
    </button>
  ),
  getResultListItemCount: jest.fn(() => 0),
  getHighlightedItemId: jest.fn(() => undefined),
}));

jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: jest.fn(),
}));

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: jest.fn(() => false),
}));

const mockAddNotification = jest.fn();
jest.mock("@/lib/contexts/notificationContexts", () => ({
  useNotificationCommands: () => ({ addNotification: mockAddNotification }),
  useNotificationState: () => ({}),
}));

const mockUseBacklogService = useBacklogService as jest.MockedFunction<typeof useBacklogService>;

function mockBacklogService(overrides: Partial<ReturnType<typeof useBacklogService>>) {
  mockUseBacklogService.mockReturnValue({
    listPipelineModes: () => Promise.resolve([]),
    parseBacklogItemIntent: jest.fn().mockResolvedValue(null),
    createBacklogItem: jest.fn().mockResolvedValue(null),
    createBacklogItemFromChat: jest.fn().mockResolvedValue(null),
    ...overrides,
  } as unknown as ReturnType<typeof useBacklogService>);
}

beforeEach(() => {
  jest.useFakeTimers();
  mockUsePathHistory.mockReturnValue(makeHistoryFixture());
  mockUsePathCompletions.mockReturnValue(defaultCompletions);
  mockUseAliases.mockReturnValue({ aliases: [], loading: false, error: null, refetch: jest.fn() });
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

describe("Omnibar compose mode", () => {
  it("pre-fills the compose textarea with text already typed", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();
    await typeAndDetect(input, "fix the flaky login test");

    fireEvent.click(screen.getByTestId("mock-create-backlog"));

    expect(screen.getByTestId("backlog-compose-textarea")).toHaveValue("fix the flaky login test");
    expect(screen.getByLabelText("Describe the work")).toBeInTheDocument();
  });

  it("opens with an empty textarea when nothing was typed", () => {
    mockBacklogService({});
    renderOmnibar();

    fireEvent.click(screen.getByTestId("mock-create-backlog"));

    expect(screen.getByTestId("backlog-compose-textarea")).toHaveValue("");
    expect(screen.getByTestId("backlog-compose-submit")).toBeDisabled();
  });

  it("returns to the omnibar with the typed text intact on cancel", async () => {
    mockBacklogService({});
    const onClose = jest.fn();
    const { input } = renderOmnibar({ onClose });
    await typeAndDetect(input, "keep me");

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.click(screen.getByTestId("backlog-compose-cancel"));

    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("combobox", { name: /session source input/i })).toHaveValue("keep me");
  });

  it("closes immediately on Create, then notifies with a deep link when creation finishes", async () => {
    let resolveCreate: (v: unknown) => void = () => {};
    const createBacklogItemFromChat = jest.fn(
      () => new Promise((resolve) => { resolveCreate = resolve; })
    );
    mockBacklogService({ createBacklogItemFromChat: createBacklogItemFromChat as never });
    const onClose = jest.fn();
    renderOmnibar({ onClose });

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), {
      target: { value: "  add dark mode  " },
    });
    fireEvent.click(screen.getByTestId("backlog-compose-submit"));

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(createBacklogItemFromChat).toHaveBeenCalledWith("add dark mode");
    expect(mockAddNotification).not.toHaveBeenCalled();

    await act(async () => {
      resolveCreate({ item: { id: "item-9", title: "Add dark mode" }, triageTriggered: false });
    });

    await waitFor(() => expect(mockAddNotification).toHaveBeenCalledTimes(1));
    const n = mockAddNotification.mock.calls[0][0];
    expect(n.message).toBe("Add dark mode");
    expect(n.notificationType).toBe("task_complete");
    n.onView();
    expect(mockPush).toHaveBeenCalledWith("/backlog?item=item-9");
  });

  it("raises an error notification when creation fails", async () => {
    mockBacklogService({ createBacklogItemFromChat: jest.fn().mockResolvedValue(null) });
    renderOmnibar();

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), { target: { value: "x" } });
    fireEvent.click(screen.getByTestId("backlog-compose-submit"));

    await waitFor(() => expect(mockAddNotification).toHaveBeenCalledTimes(1));
    expect(mockAddNotification.mock.calls[0][0].notificationType).toBe("error");
  });

  it("rejects an image over 10 MB client-side without uploading", () => {
    mockBacklogService({});
    const fetchSpy = jest.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
    renderOmnibar();

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    const big = new File([new Uint8Array(10 * 1024 * 1024 + 1)], "big.png", { type: "image/png" });
    fireEvent.change(screen.getByTestId("backlog-compose-attach-input"), { target: { files: [big] } });

    expect(screen.getByTestId("backlog-compose-error")).toHaveTextContent("too large");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("still opens the intent review for the backlog: prefix (no regression)", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();

    await typeAndDetect(input, "backlog: fix the login timeout bug");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });

    await waitFor(() =>
      expect(screen.getByTestId("backlog-intent-review-parse-failed-banner")).toBeInTheDocument()
    );
    expect(screen.queryByTestId("backlog-compose-widget")).not.toBeInTheDocument();
  });
});
