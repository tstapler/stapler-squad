/**
 * Compose mode (dedicated textarea) opened by the omnibar's "Create Backlog
 * Item" button: text preservation, hand-off to the AI intent review,
 * and no regression to the "backlog: " prefix path (see
 * Omnibar.backlogIntentReview.test.tsx).
 */

import React from "react";
import { render, screen, fireEvent, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Omnibar } from "@/components/sessions/Omnibar";
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
jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.memory"),
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
    // eslint-disable-next-line analytics/require-on-click -- pre-existing, unrelated (test mock, not production UI)
    <button type="button" data-testid="mock-create-backlog" onClick={onCreateBacklog}>
      Create Backlog Item (AI)
    </button>
  ),
  getResultListItemCount: jest.fn(() => 0),
  getHighlightedItemId: jest.fn(() => undefined),
}));

jest.mock("@/lib/hooks/useGitHubEnterpriseHosts", () => ({ useGitHubEnterpriseHosts: () => [] }));

let mockRepoPaths: string[] = [];
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => mockRepoPaths,
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

let fetchSpy: jest.SpyInstance | undefined;

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
  mockRepoPaths = [];
  act(() => {
    jest.runOnlyPendingTimers();
  });
  jest.useRealTimers();
  fetchSpy?.mockRestore();
  fetchSpy = undefined;
  jest.clearAllMocks();
  resetDefaultRegistry();
});

describe("Omnibar compose mode", () => {
  it("pre-fills the compose textarea with text already typed", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();
    await typeAndDetect(input, "fix the flaky login test");

    fireEvent.click(screen.getByTestId("mock-create-backlog"));

    // BacklogComposeWidget is lazy-loaded (see Omnibar.tsx); the very first render in a
    // test file suspends until the chunk resolves, so this first assertion must await it.
    // Later tests in this file hit the same lazy() instance already resolved and can use
    // the synchronous getByTestId as before.
    expect(await screen.findByTestId("backlog-compose-textarea")).toHaveValue("fix the flaky login test");
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

  it("restores edits made in the widget to the omnibar input on cancel", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();
    await typeAndDetect(input, "seed");

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), {
      target: { value: "seed plus a longer draft" },
    });
    fireEvent.click(screen.getByTestId("backlog-compose-cancel"));

    expect(screen.getByRole("combobox", { name: /session source input/i })).toHaveValue(
      "seed plus a longer draft"
    );
  });

  it("strips a leading backlog: prefix when seeding the compose textarea", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();
    await typeAndDetect(input, "backlog: fix login");

    fireEvent.click(screen.getByTestId("mock-create-backlog"));

    expect(screen.getByTestId("backlog-compose-textarea")).toHaveValue("fix login");
  });

  it("does not reopen compose mode after the omnibar is closed and reopened", () => {
    mockBacklogService({});
    const props = {
      onClose: jest.fn(),
      onCreateSession: jest.fn(),
      onNavigateToSession: jest.fn(),
    };
    const { rerender } = render(<Omnibar isOpen={true} {...props} />);
    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    expect(screen.getByTestId("backlog-compose-widget")).toBeInTheDocument();

    rerender(<Omnibar isOpen={false} {...props} />);
    rerender(<Omnibar isOpen={true} {...props} />);

    expect(screen.queryByTestId("backlog-compose-widget")).not.toBeInTheDocument();
  });

  it("Escape returns to the input with the draft and does not close the omnibar", () => {
    mockBacklogService({});
    const onClose = jest.fn();
    renderOmnibar({ onClose });

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    const textarea = screen.getByTestId("backlog-compose-textarea");
    fireEvent.change(textarea, { target: { value: "draft" } });
    fireEvent.keyDown(textarea, { key: "Escape" });

    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("combobox", { name: /session source input/i })).toHaveValue("draft");
  });

  it("ignores a backdrop click so the draft is not discarded", () => {
    mockBacklogService({});
    const onClose = jest.fn();
    renderOmnibar({ onClose });

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), { target: { value: "draft" } });
    fireEvent.click(screen.getByTestId("omnibar"));

    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId("backlog-compose-textarea")).toHaveValue("draft");
  });

  it("creates the item in the background: closes the omnibar, skips intent parsing, notifies", async () => {
    const parseBacklogItemIntent = jest.fn();
    const createBacklogItemFromChat = jest
      .fn()
      .mockResolvedValue({ item: { id: "item-1", title: "Add dark mode" }, triageTriggered: true });
    mockBacklogService({
      parseBacklogItemIntent: parseBacklogItemIntent as never,
      createBacklogItemFromChat: createBacklogItemFromChat as never,
    });
    const onClose = jest.fn();
    renderOmnibar({ onClose });

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), {
      target: { value: "  add dark mode ![shot](/api/local/serve/x.png) " },
    });
    await act(async () => {
      fireEvent.click(screen.getByTestId("backlog-compose-submit"));
    });

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/Parsing your message/)).not.toBeInTheDocument();
    expect(parseBacklogItemIntent).not.toHaveBeenCalled();
    expect(createBacklogItemFromChat).toHaveBeenCalledWith(
      "add dark mode ![shot](/api/local/serve/x.png)",
      undefined,
      ""
    );
    expect(mockAddNotification).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Backlog item created — triage underway", message: "Add dark mode" })
    );
  });

  async function submitCompose(text: string) {
    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), { target: { value: text } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("backlog-compose-submit"));
    });
  }

  it("pre-fills the repo path from active-session repo roots and sends it with the item", async () => {
    mockRepoPaths = ["/home/user/my-repo", "/home/user/other"];
    const createBacklogItemFromChat = jest
      .fn()
      .mockResolvedValue({ item: { id: "i", title: "T" }, triageTriggered: true });
    mockBacklogService({ createBacklogItemFromChat: createBacklogItemFromChat as never });
    renderOmnibar();

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    expect(screen.getByTestId("backlog-compose-repo")).toHaveValue("/home/user/my-repo");
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), { target: { value: "idea" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("backlog-compose-submit"));
    });

    expect(createBacklogItemFromChat).toHaveBeenCalledWith("idea", undefined, "/home/user/my-repo");
  });

  it("sends an empty repo path when the user clears the pre-filled one", async () => {
    mockRepoPaths = ["/home/user/my-repo"];
    const createBacklogItemFromChat = jest
      .fn()
      .mockResolvedValue({ item: { id: "i", title: "T" }, triageTriggered: false });
    mockBacklogService({ createBacklogItemFromChat: createBacklogItemFromChat as never });
    renderOmnibar();

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-repo"), { target: { value: "" } });
    expect(screen.getByTestId("backlog-compose-repo")).toHaveValue("");
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), { target: { value: "idea" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("backlog-compose-submit"));
    });

    expect(createBacklogItemFromChat).toHaveBeenCalledWith("idea", undefined, "");
  });

  it("says triage could not start and links to the item when triageTriggered is false", async () => {
    mockBacklogService({
      createBacklogItemFromChat: jest
        .fn()
        .mockResolvedValue({ item: { id: "item-9", title: "T" }, triageTriggered: false }) as never,
    });
    renderOmnibar();

    await submitCompose("idea");

    const call = mockAddNotification.mock.calls.find(([n]) => /could not start/.test(n.title))?.[0];
    expect(call).toBeDefined();
    expect(call.message).toMatch(/Add a repo path/);
    call.onView();
    expect(mockPush).toHaveBeenCalledWith("/backlog?item=item-9");
    expect(mockAddNotification).not.toHaveBeenCalledWith(
      expect.objectContaining({ title: expect.stringMatching(/triage underway|triage starting/) })
    );
  });

  it("tells the user to check the repo path when one was set but triage still did not start", async () => {
    mockRepoPaths = ["/home/user/my-repo"];
    mockBacklogService({
      createBacklogItemFromChat: jest
        .fn()
        .mockResolvedValue({ item: { id: "i", title: "T" }, triageTriggered: false }) as never,
    });
    renderOmnibar();

    await submitCompose("idea");

    expect(mockAddNotification).toHaveBeenCalledWith(
      expect.objectContaining({ message: expect.stringMatching(/Check the repo path/) })
    );
  });

  it("enriches the item in the background when triage did not start", async () => {
    const item = { id: "item-3", title: "idea", description: "idea", acCriteria: [] };
    const parseBacklogItemIntent = jest.fn().mockResolvedValue({
      title: "Better title",
      description: "Better description",
      acceptanceCriteria: ["AC one", "AC two"],
      confidence: 0.9,
      suggestedRepoPath: "",
    });
    const updateBacklogItem = jest.fn().mockResolvedValue({ ...item, title: "Better title" });
    mockBacklogService({
      createBacklogItemFromChat: jest.fn().mockResolvedValue({ item, triageTriggered: false }) as never,
      parseBacklogItemIntent: parseBacklogItemIntent as never,
      getBacklogItem: jest.fn().mockResolvedValue(item) as never,
      updateBacklogItem: updateBacklogItem as never,
    });
    renderOmnibar();

    await submitCompose("idea");

    expect(parseBacklogItemIntent).toHaveBeenCalledWith("idea", []);
    expect(updateBacklogItem).toHaveBeenCalledWith("item-3", {
      title: "Better title",
      description: "Better description",
      acCriteria: [
        { index: 0, text: "AC one", status: "pending" },
        { index: 1, text: "AC two", status: "pending" },
      ],
    });
    const call = mockAddNotification.mock.calls.find(([n]) => n.title === "Backlog item enriched")?.[0];
    expect(call).toBeDefined();
    call.onView();
    expect(mockPush).toHaveBeenCalledWith("/backlog?item=item-3");
  });

  it("does not enrich when triage started", async () => {
    const parseBacklogItemIntent = jest.fn();
    mockBacklogService({
      createBacklogItemFromChat: jest
        .fn()
        .mockResolvedValue({ item: { id: "i", title: "T" }, triageTriggered: true }) as never,
      parseBacklogItemIntent: parseBacklogItemIntent as never,
    });
    renderOmnibar();

    await submitCompose("idea");

    expect(parseBacklogItemIntent).not.toHaveBeenCalled();
  });

  it("enrichment failure is silent and the update is skipped when the user edited the item", async () => {
    const item = { id: "i", title: "T", description: "d", acCriteria: [] };
    const updateBacklogItem = jest.fn();
    const parse = jest
      .fn()
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ title: "x", description: "y", acceptanceCriteria: [], confidence: 1, suggestedRepoPath: "" });
    mockBacklogService({
      createBacklogItemFromChat: jest.fn().mockResolvedValue({ item, triageTriggered: false }) as never,
      parseBacklogItemIntent: parse as never,
      getBacklogItem: jest.fn().mockResolvedValue({ ...item, description: "edited by user" }) as never,
      updateBacklogItem: updateBacklogItem as never,
    });
    renderOmnibar();

    await submitCompose("idea");
    expect(updateBacklogItem).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), { target: { value: "again" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("backlog-compose-submit"));
    });
    expect(updateBacklogItem).not.toHaveBeenCalled();
    expect(mockAddNotification).not.toHaveBeenCalledWith(
      expect.objectContaining({ title: "Backlog item enriched" })
    );
    expect(mockAddNotification).not.toHaveBeenCalledWith(
      expect.objectContaining({ notificationType: "error" })
    );
  });

  it("Explore with AI reveals a brainstorm panel that sends question + draft without blocking submit", async () => {
    let resolveParse: (v: unknown) => void = () => {};
    const parseBacklogItemIntent = jest.fn().mockReturnValue(new Promise((r) => (resolveParse = r)));
    mockBacklogService({ parseBacklogItemIntent: parseBacklogItemIntent as never });
    renderOmnibar();

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), { target: { value: "add dark mode" } });
    expect(screen.queryByTestId("brainstorm-panel")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("backlog-compose-explore"));
    fireEvent.change(screen.getByTestId("brainstorm-input"), { target: { value: "what are the risks?" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("brainstorm-ask"));
    });

    expect(parseBacklogItemIntent).toHaveBeenCalledWith("what are the risks?\n\nadd dark mode", []);
    expect(screen.getByTestId("backlog-compose-submit")).toBeEnabled();

    await act(async () => {
      resolveParse({ title: "t", description: "d", acceptanceCriteria: ["Toggle persists"], confidence: 1, suggestedRepoPath: "" });
    });
    expect(screen.getByText("Toggle persists")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("backlog-compose-explore"));
    expect(screen.queryByTestId("brainstorm-panel")).not.toBeInTheDocument();
  });

  it("raises an error notification when background creation fails", async () => {
    mockBacklogService({ createBacklogItemFromChat: jest.fn().mockResolvedValue(null) as never });
    renderOmnibar();

    fireEvent.click(screen.getByTestId("mock-create-backlog"));
    fireEvent.change(screen.getByTestId("backlog-compose-textarea"), { target: { value: "raw idea" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("backlog-compose-submit"));
    });

    expect(mockAddNotification).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Failed to create backlog item", notificationType: "error" })
    );
  });

  it("rejects an image over 10 MB client-side without uploading", () => {
    mockBacklogService({});
    // jsdom has no fetch; spyOn needs an existing property.
    if (!global.fetch) global.fetch = jest.fn() as unknown as typeof fetch;
    fetchSpy = jest.spyOn(global, "fetch").mockImplementation(jest.fn());
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

describe("Omnibar footer / fallback Create Backlog Item button", () => {
  afterEach(() => {
    const atMock = jest.requireMock("@/lib/hooks/useAtCommandSuggestions").useAtCommandSuggestions as jest.Mock;
    atMock.mockReturnValue({ isAtCommand: false, suggestions: [], complete: jest.fn() });
  });

  function enterCreationMode() {
    fireEvent.click(screen.getByRole("button", { name: /^Create$/ }));
  }

  it("footer click seeds and focuses the compose textarea", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();
    await typeAndDetect(input, "fix the flaky login test");
    enterCreationMode();

    fireEvent.click(screen.getByTestId("omnibar-create-backlog-button"));

    const textarea = screen.getByTestId("backlog-compose-textarea");
    expect(textarea).toHaveValue("fix the flaky login test");
    expect(textarea).toHaveFocus();
  });

  it("plain Enter on the focused footer button opens compose and does not also run the submit path", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();
    // "backlog: ..." makes canSubmit true, so a leaked Enter would open the intent review.
    await typeAndDetect(input, "backlog: fix login");
    enterCreationMode();
    expect(screen.getByTestId("omnibar-create-session-button")).toBeEnabled();

    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    screen.getByTestId("omnibar-create-backlog-button").focus();
    await user.keyboard("{Enter}");

    // Had Enter also run handleSubmit, the intent review would have replaced the compose widget.
    expect(screen.getByTestId("backlog-compose-textarea")).toBeInTheDocument();
  });

  it("Cancel from the footer-opened compose returns to the creation form with the input intact", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();
    await typeAndDetect(input, "/tmp");
    enterCreationMode();

    fireEvent.click(screen.getByTestId("omnibar-create-backlog-button"));
    fireEvent.click(screen.getByTestId("backlog-compose-cancel"));

    expect(screen.getByTestId("omnibar-create-session-button")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: /session source input/i })).toHaveValue("/tmp");
  });

  it("does not stop Escape on the button, so the modal handler still sees it", async () => {
    mockBacklogService({});
    const { input } = renderOmnibar();
    await typeAndDetect(input, "/tmp");
    enterCreationMode();
    const onKeyDown = jest.fn();
    const modal = screen.getByTestId("omnibar");
    modal.addEventListener("keydown", onKeyDown);

    fireEvent.keyDown(screen.getByTestId("omnibar-create-backlog-button"), { key: "Escape" });

    expect(onKeyDown).toHaveBeenCalled();
  });

  it("shows the footer button in every discovery state, including while an @ command hides the list row", async () => {
    mockBacklogService({});
    const atMock = jest.requireMock("@/lib/hooks/useAtCommandSuggestions").useAtCommandSuggestions as jest.Mock;
    const { input } = renderOmnibar();
    await typeAndDetect(input, "fix login");
    expect(screen.getByTestId("omnibar-create-backlog-button")).toBeInTheDocument();

    atMock.mockReturnValue({ isAtCommand: true, suggestions: [], complete: jest.fn() });
    await typeAndDetect(input, "@deploy");
    fireEvent.click(screen.getByTestId("omnibar-create-backlog-button"));

    expect(screen.getByTestId("backlog-compose-textarea")).toHaveValue("@deploy");
  });
});
