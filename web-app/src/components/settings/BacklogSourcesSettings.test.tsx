/**
 * Tests for BacklogSourcesSettings component.
 *
 * Covers:
 *  1. Renders source list after load
 *  2. Shows empty state when no sources
 *  3. Add source form calls createItemSource with correct payload
 *  4. Add button disabled until required fields are filled
 *  5. Toggle enabled calls updateItemSource
 *  6. Delete calls deleteItemSource
 *  7. Sync now calls triggerSync
 *  8. View history calls getSyncHistory and renders events
 */

import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BacklogSourcesSettings } from "./BacklogSourcesSettings";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost", createAuthInterceptor: () => (x: unknown) => x }));

jest.mock("./BacklogSourcesSettings.css", () => {
  return new Proxy({}, { get: (_target, prop) => (typeof prop === "string" ? prop : "") });
});

const mockListItemSources = jest.fn();
const mockCreateItemSource = jest.fn();
const mockUpdateItemSource = jest.fn();
const mockDeleteItemSource = jest.fn();
const mockTriggerSync = jest.fn();
const mockGetSyncHistory = jest.fn();
const mockPreviewBackwardSyncImpact = jest.fn();

const sampleSource = {
  id: "src-1",
  pluginId: "github_issues",
  displayName: "Acme Issues",
  enabled: true,
  tokenConfigured: true,
  lastSyncedAt: undefined,
  forwardSyncEnabled: false,
  backwardSyncEnabled: false,
  forwardSyncCloseLabel: "",
};

beforeEach(() => {
  jest.clearAllMocks();
  mockListItemSources.mockResolvedValue({ sources: [sampleSource] });
  mockCreateItemSource.mockResolvedValue({ source: sampleSource });
  mockUpdateItemSource.mockResolvedValue({ source: { ...sampleSource, enabled: false } });
  mockDeleteItemSource.mockResolvedValue({});
  mockTriggerSync.mockResolvedValue({});
  mockPreviewBackwardSyncImpact.mockResolvedValue({ itemCount: 0, sampleTitles: [] });
  mockGetSyncHistory.mockResolvedValue({
    events: [
      {
        id: "ev-1",
        startedAt: undefined,
        finishedAt: undefined,
        itemsCreated: 2,
        itemsUpdated: 1,
        itemsSkipped: 0,
        itemsErrored: 0,
        errorMessage: "",
      },
    ],
  });

  (createClient as jest.Mock).mockReturnValue({
    listItemSources: mockListItemSources,
    createItemSource: mockCreateItemSource,
    updateItemSource: mockUpdateItemSource,
    deleteItemSource: mockDeleteItemSource,
    triggerSync: mockTriggerSync,
    getSyncHistory: mockGetSyncHistory,
    previewBackwardSyncImpact: mockPreviewBackwardSyncImpact,
  });
  (createConnectTransport as jest.Mock).mockReturnValue({});
});

describe("BacklogSourcesSettings", () => {
  it("renders source list after load", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => {
      expect(screen.getByText("Acme Issues")).toBeInTheDocument();
    });
    expect(screen.getByText("github_issues")).toBeInTheDocument();
  });

  it("shows empty state when no sources", async () => {
    mockListItemSources.mockResolvedValue({ sources: [] });
    render(<BacklogSourcesSettings />);
    await waitFor(() => {
      expect(screen.getByText("No sources configured.")).toBeInTheDocument();
    });
  });

  it("add button is disabled until required fields are filled", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const addButton = screen.getByRole("button", { name: "Add Source" });
    expect(addButton).toBeDisabled();

    fireEvent.change(screen.getByPlaceholderText("Display name (e.g. My Repo Issues)"), {
      target: { value: "Widgets Issues" },
    });
    fireEvent.change(screen.getByPlaceholderText("Owner (e.g. acme)"), { target: { value: "acme" } });
    fireEvent.change(screen.getByPlaceholderText("Repo (e.g. widgets)"), { target: { value: "widgets" } });

    expect(addButton).not.toBeDisabled();
  });

  it("shows a link to connect a GitHub account instead of a token field for GitHub plugins", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    expect(screen.queryByPlaceholderText("GitHub personal access token")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "GitHub accounts" })).toHaveAttribute("href", "/unfinished");
  });

  it("calls createItemSource with the correct payload on submit", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText("Display name (e.g. My Repo Issues)"), {
      target: { value: "Widgets Issues" },
    });
    fireEvent.change(screen.getByPlaceholderText("Owner (e.g. acme)"), { target: { value: "acme" } });
    fireEvent.change(screen.getByPlaceholderText("Repo (e.g. widgets)"), { target: { value: "widgets" } });

    fireEvent.click(screen.getByRole("button", { name: "Add Source" }));

    await waitFor(() => {
      expect(mockCreateItemSource).toHaveBeenCalledWith({
        pluginId: "github_issues",
        displayName: "Widgets Issues",
        configJson: JSON.stringify({ owner: "acme", repo: "widgets" }),
        token: "",
      });
    });
  });

  it("shows an error message when createItemSource fails", async () => {
    mockCreateItemSource.mockRejectedValue(new Error("token invalid"));
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText("Display name (e.g. My Repo Issues)"), {
      target: { value: "Widgets Issues" },
    });
    fireEvent.change(screen.getByPlaceholderText("Owner (e.g. acme)"), { target: { value: "acme" } });
    fireEvent.change(screen.getByPlaceholderText("Repo (e.g. widgets)"), { target: { value: "widgets" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Source" }));

    expect(await screen.findByText("token invalid")).toBeInTheDocument();
  });

  it("clears a stale error banner once a later listItemSources call succeeds", async () => {
    mockListItemSources.mockRejectedValueOnce(new Error("transient network error"));
    mockListItemSources.mockResolvedValue({ sources: [sampleSource] });
    render(<BacklogSourcesSettings />);

    expect(await screen.findByText("transient network error")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Display name (e.g. My Repo Issues)"), {
      target: { value: "Widgets Issues" },
    });
    fireEvent.change(screen.getByPlaceholderText("Owner (e.g. acme)"), { target: { value: "acme" } });
    fireEvent.change(screen.getByPlaceholderText("Repo (e.g. widgets)"), { target: { value: "widgets" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Source" }));

    await waitFor(() => expect(screen.queryByText("transient network error")).not.toBeInTheDocument());
  });

  it("clears schema-driven field values when switching plugin type", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText("Owner (e.g. acme)"), { target: { value: "acme" } });
    fireEvent.change(screen.getByPlaceholderText("Repo (e.g. widgets)"), { target: { value: "widgets" } });

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "github_prs" } });

    expect((screen.getByPlaceholderText("Owner (e.g. acme)") as HTMLInputElement).value).toBe("");
    expect((screen.getByPlaceholderText("Repo (e.g. widgets)") as HTMLInputElement).value).toBe("");
  });

  it("toggles enabled state via updateItemSource", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("switch", { name: "Disable Acme Issues" }));

    await waitFor(() => {
      expect(mockUpdateItemSource).toHaveBeenCalledWith({
        sourceId: "src-1",
        displayName: "Acme Issues",
        enabled: false,
        token: "",
        forwardSyncEnabled: false,
        backwardSyncEnabled: false,
        forwardSyncCloseLabel: "",
      });
    });
  });

  it("ignores a second rapid click on the enabled toggle while its RPC is in flight", async () => {
    // Regression test: handleToggleEnabled had no in-flight guard, unlike
    // handleToggleBackwardSync's backwardSyncPreviewPendingId pattern. A
    // rapid double-click both read the same (still-current) source.enabled
    // prop and sent the same target value twice, silently dropping the
    // user's second click intent.
    let resolveUpdate: (value: { source: typeof sampleSource }) => void = () => {};
    mockUpdateItemSource.mockReturnValue(
      new Promise((resolve) => {
        resolveUpdate = resolve;
      })
    );
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const toggle = screen.getByRole("switch", { name: "Disable Acme Issues" });
    fireEvent.click(toggle);
    await waitFor(() => expect(toggle).toBeDisabled());
    fireEvent.click(toggle);

    resolveUpdate({ source: { ...sampleSource, enabled: false } });
    await waitFor(() => expect(toggle).not.toBeDisabled());

    expect(mockUpdateItemSource).toHaveBeenCalledTimes(1);
  });

  it("calls deleteItemSource when remove is clicked", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Remove Acme Issues" }));

    await waitFor(() => {
      expect(mockDeleteItemSource).toHaveBeenCalledWith({ sourceId: "src-1" });
    });
  });

  it("calls triggerSync when Sync now is clicked", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Sync now" }));

    await waitFor(() => {
      expect(mockTriggerSync).toHaveBeenCalledWith({ sourceId: "src-1" });
    });
  });

  it("fetches and renders sync history on View history click", async () => {
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "View history" }));

    await waitFor(() => {
      expect(mockGetSyncHistory).toHaveBeenCalledWith({ sourceId: "src-1" });
    });
    expect(await screen.findByText(/created 2, updated 1, skipped 0/)).toBeInTheDocument();
  });

  it("shows a truncation notice when sync history is capped", async () => {
    mockGetSyncHistory.mockResolvedValue({
      events: [
        {
          id: "ev-1",
          startedAt: undefined,
          finishedAt: undefined,
          itemsCreated: 2,
          itemsUpdated: 1,
          itemsSkipped: 0,
          itemsErrored: 0,
          errorMessage: "",
        },
      ],
      truncated: true,
    });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "View history" }));

    expect(await screen.findByText(/Older sync history exists but is not shown/)).toBeInTheDocument();
  });
});

describe("BacklogSourcesSettings — Epic 4.3 (backlog-github-two-way-sync): sync-direction toggles", () => {
  it("BacklogSourcesSettings_should_RenderTogglesWithFetchedState_When_SourceLoaded", async () => {
    mockListItemSources.mockResolvedValue({
      sources: [{ ...sampleSource, forwardSyncEnabled: true, backwardSyncEnabled: false }],
    });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    expect(screen.getByRole("switch", { name: /closing GitHub issues/ })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("switch", { name: /reflecting GitHub status back/ })).toHaveAttribute(
      "aria-checked",
      "false"
    );
  });

  it("BacklogSourcesSettings_should_CallSetForwardSyncEnabled_When_ForwardToggleClicked", async () => {
    mockUpdateItemSource.mockResolvedValue({ source: { ...sampleSource, forwardSyncEnabled: true } });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("switch", { name: /closing GitHub issues/ }));

    await waitFor(() => {
      expect(mockUpdateItemSource).toHaveBeenCalledWith({
        sourceId: "src-1",
        displayName: "Acme Issues",
        enabled: true,
        token: "",
        forwardSyncEnabled: true,
        backwardSyncEnabled: false,
        forwardSyncCloseLabel: "",
      });
    });
  });

  it("ignores a second rapid click on the forward-sync toggle while its RPC is in flight", async () => {
    // Same stale-closure double-click guard as the enabled toggle above.
    let resolveUpdate: (value: { source: typeof sampleSource }) => void = () => {};
    mockUpdateItemSource.mockReturnValue(
      new Promise((resolve) => {
        resolveUpdate = resolve;
      })
    );
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const toggle = screen.getByRole("switch", { name: /closing GitHub issues/ });
    fireEvent.click(toggle);
    await waitFor(() => expect(toggle).toBeDisabled());
    fireEvent.click(toggle);

    resolveUpdate({ source: { ...sampleSource, forwardSyncEnabled: true } });
    await waitFor(() => expect(toggle).not.toBeDisabled());

    expect(mockUpdateItemSource).toHaveBeenCalledTimes(1);
  });

  it("calls setBackwardSyncEnabled directly when disabling an already-enabled backward toggle", async () => {
    mockListItemSources.mockResolvedValue({
      sources: [{ ...sampleSource, backwardSyncEnabled: true }],
    });
    mockUpdateItemSource.mockResolvedValue({ source: { ...sampleSource, backwardSyncEnabled: false } });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("switch", { name: /reflecting GitHub status back/ }));

    await waitFor(() => {
      expect(mockUpdateItemSource).toHaveBeenCalledWith({
        sourceId: "src-1",
        displayName: "Acme Issues",
        enabled: true,
        token: "",
        forwardSyncEnabled: false,
        backwardSyncEnabled: false,
        forwardSyncCloseLabel: "",
      });
    });
  });

  it("shows the close-label input only while forward sync is enabled", async () => {
    mockListItemSources.mockResolvedValue({
      sources: [{ ...sampleSource, forwardSyncEnabled: true }],
    });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    expect(screen.getByPlaceholderText("Label to apply on close (optional)")).toBeInTheDocument();
  });

  it("reconciles the close-label input with server truth after a successful commit instead of pinning to the local draft forever", async () => {
    // Regression test: closeLabelDrafts[source.id] used to be set on every
    // keystroke and never cleared after a successful commit, so the input
    // permanently pinned to the locally-typed value and never reflected a
    // later server-side change (e.g. from another client).
    mockListItemSources
      .mockResolvedValueOnce({
        sources: [{ ...sampleSource, forwardSyncEnabled: true, forwardSyncCloseLabel: "old-label" }],
      })
      .mockResolvedValue({
        sources: [{ ...sampleSource, forwardSyncEnabled: true, forwardSyncCloseLabel: "server-label" }],
      });
    mockUpdateItemSource.mockResolvedValue({
      source: { ...sampleSource, forwardSyncEnabled: true, forwardSyncCloseLabel: "typed-label" },
    });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const input = screen.getByPlaceholderText("Label to apply on close (optional)");
    fireEvent.change(input, { target: { value: "typed-label" } });
    fireEvent.blur(input);

    await waitFor(() => {
      expect(mockUpdateItemSource).toHaveBeenCalledWith(
        expect.objectContaining({ forwardSyncCloseLabel: "typed-label" })
      );
    });

    // Once the commit + refresh land, the input should show the server's
    // value ("server-label"), not stay pinned to "typed-label".
    await waitFor(() => expect(input).toHaveValue("server-label"));
  });

  it("BacklogSourcesSettings_should_ShowBothDirectionsWarning_When_BothTogglesEnabled", async () => {
    mockListItemSources.mockResolvedValue({
      sources: [{ ...sampleSource, forwardSyncEnabled: true, backwardSyncEnabled: true }],
    });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    expect(screen.getByText(/Both directions are enabled/)).toBeInTheDocument();
  });

  it("does not show the both-directions warning when only one direction is enabled", async () => {
    mockListItemSources.mockResolvedValue({
      sources: [{ ...sampleSource, forwardSyncEnabled: true, backwardSyncEnabled: false }],
    });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    expect(screen.queryByText(/Both directions are enabled/)).not.toBeInTheDocument();
  });
});

describe("BacklogSourcesSettings — Epic 4.4: first-enable-of-backward-sync confirmation", () => {
  it("TestBacklogSourcesSettings_ShowsConfirmDialogWithPreviewCount_WhenEnablingBackwardSync", async () => {
    mockPreviewBackwardSyncImpact.mockResolvedValue({ itemCount: 3, sampleTitles: ["Bug A", "Bug B", "Bug C"] });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("switch", { name: /reflecting GitHub status back/ }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/archive 3 already-imported items/)).toBeInTheDocument();
    expect(within(dialog).getByText(/Bug A, Bug B, Bug C/)).toBeInTheDocument();
    expect(within(dialog).getByText(/can't be undone by disabling this toggle again/)).toBeInTheDocument();
    expect(mockUpdateItemSource).not.toHaveBeenCalled();
  });

  it("TestBacklogSourcesSettings_SkipsDialogWhenPreviewCountIsZero", async () => {
    mockPreviewBackwardSyncImpact.mockResolvedValue({ itemCount: 0, sampleTitles: [] });
    mockUpdateItemSource.mockResolvedValue({ source: { ...sampleSource, backwardSyncEnabled: true } });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("switch", { name: /reflecting GitHub status back/ }));

    await waitFor(() => {
      expect(mockUpdateItemSource).toHaveBeenCalledWith({
        sourceId: "src-1",
        displayName: "Acme Issues",
        enabled: true,
        token: "",
        forwardSyncEnabled: false,
        backwardSyncEnabled: true,
        forwardSyncCloseLabel: "",
      });
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("TestBacklogSourcesSettings_ShowsDialogWithCaveat_WhenPreviewCountIsZeroButPossiblyIncomplete", async () => {
    // itemCount: 0 alone skips the dialog (see the test above) — but if the
    // underlying fetch hit its pagination cap, a 0 count is not trustworthy
    // as "nothing to warn about" (the CRITICAL under-reporting finding this
    // guards against). The dialog must still show, with an explicit caveat.
    mockPreviewBackwardSyncImpact.mockResolvedValue({ itemCount: 0, sampleTitles: [], possiblyIncomplete: true });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("switch", { name: /reflecting GitHub status back/ }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByTestId("backward-sync-confirm-incomplete-caveat")).toBeInTheDocument();
    expect(mockUpdateItemSource).not.toHaveBeenCalled();
  });

  it("TestBacklogSourcesSettings_CancelLeavesToggleOffAndMakesNoRPCCall", async () => {
    mockPreviewBackwardSyncImpact.mockResolvedValue({ itemCount: 2, sampleTitles: ["A", "B"] });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const toggle = screen.getByRole("switch", { name: /reflecting GitHub status back/ });
    fireEvent.click(toggle);

    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByTestId("backward-sync-confirm-cancel"));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(mockUpdateItemSource).not.toHaveBeenCalled();
    expect(toggle).toHaveAttribute("aria-checked", "false");
  });

  it("TestBacklogSourcesSettings_ConfirmFlipsToggleAndCallsSetBackwardSyncEnabled", async () => {
    mockPreviewBackwardSyncImpact.mockResolvedValue({ itemCount: 1, sampleTitles: ["Only Item"] });
    mockUpdateItemSource.mockResolvedValue({ source: { ...sampleSource, backwardSyncEnabled: true } });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("switch", { name: /reflecting GitHub status back/ }));

    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByTestId("backward-sync-confirm-confirm"));

    await waitFor(() => {
      expect(mockUpdateItemSource).toHaveBeenCalledWith({
        sourceId: "src-1",
        displayName: "Acme Issues",
        enabled: true,
        token: "",
        forwardSyncEnabled: false,
        backwardSyncEnabled: true,
        forwardSyncCloseLabel: "",
      });
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("TestBacklogSourcesSettings_TogglePendingWhilePreviewInFlight", async () => {
    let resolvePreview: (value: { itemCount: number; sampleTitles: string[] }) => void = () => {};
    mockPreviewBackwardSyncImpact.mockReturnValue(
      new Promise((resolve) => {
        resolvePreview = resolve;
      })
    );
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const toggle = screen.getByRole("switch", { name: /reflecting GitHub status back/ });
    fireEvent.click(toggle);

    await waitFor(() => expect(toggle).toBeDisabled());
    expect(toggle).toHaveAttribute("aria-busy", "true");

    resolvePreview({ itemCount: 0, sampleTitles: [] });

    await waitFor(() => expect(toggle).not.toBeDisabled());
  });

  it("TestBacklogSourcesSettings_ShowsInlineErrorWhenPreviewFails_NoDialog", async () => {
    mockPreviewBackwardSyncImpact.mockRejectedValue(new Error("boom"));
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const toggle = screen.getByRole("switch", { name: /reflecting GitHub status back/ });
    fireEvent.click(toggle);

    expect(await screen.findByText("Couldn't check impact — try again")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(toggle).toHaveAttribute("aria-checked", "false");
    expect(mockUpdateItemSource).not.toHaveBeenCalled();
  });

  it("TestBacklogSourcesSettings_ConfirmDialogTrapsFocusAndReturnsFocusOnClose", async () => {
    const user = userEvent.setup();
    mockPreviewBackwardSyncImpact.mockResolvedValue({ itemCount: 2, sampleTitles: ["Item A", "Item B"] });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    const toggle = screen.getByRole("switch", { name: /reflecting GitHub status back/ });
    await user.click(toggle);

    const dialog = await screen.findByRole("dialog");
    const confirmButton = within(dialog).getByTestId("backward-sync-confirm-confirm");
    const cancelButton = within(dialog).getByTestId("backward-sync-confirm-cancel");

    await waitFor(() => expect(confirmButton).toHaveFocus());

    // Tab from the last focusable element wraps to the first (focus trap).
    fireEvent.keyDown(document, { key: "Tab" });
    expect(cancelButton).toHaveFocus();

    // Escape is treated as Cancel: closes the dialog, makes no RPC call, and
    // returns focus to the toggle that triggered it.
    fireEvent.keyDown(document, { key: "Escape" });

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(mockUpdateItemSource).not.toHaveBeenCalled();
    await waitFor(() => expect(toggle).toHaveFocus());
  });
});

describe("BacklogSourcesSettings — Story 4.3.2: row-level non-transient-failure warning", () => {
  it("BacklogSourcesSettings_should_ShowRowLevelWarning_When_RecentSyncHasAuthError", async () => {
    mockGetSyncHistory.mockResolvedValue({
      events: [
        {
          id: "ev-1",
          startedAt: undefined,
          finishedAt: undefined,
          itemsCreated: 0,
          itemsUpdated: 0,
          itemsSkipped: 0,
          itemsErrored: 1,
          errorMessage: "fetch failed: 401 Unauthorized — token revoked",
        },
      ],
      truncated: false,
    });
    render(<BacklogSourcesSettings />);

    expect(await screen.findByTestId("source-row-src-1-auth-warning")).toBeInTheDocument();
  });

  it("BacklogSourcesSettings_should_ShowWarningForForwardSyncFailure_When_MostRecentEventIsACloseFailure", async () => {
    // Regression test for pre-mortem P1 #3: the warning must render for a
    // forward-sync CloseIssue failure (persisted via RecordSourceSyncFailure),
    // not only a Fetch failure — same read path (historyBySource), any origin.
    mockGetSyncHistory.mockResolvedValue({
      events: [
        {
          id: "ev-2",
          startedAt: undefined,
          finishedAt: undefined,
          itemsCreated: 0,
          itemsUpdated: 0,
          itemsSkipped: 0,
          itemsErrored: 1,
          errorMessage: "close issue failed: 403 Forbidden",
        },
      ],
      truncated: false,
    });
    render(<BacklogSourcesSettings />);

    expect(await screen.findByTestId("source-row-src-1-auth-warning")).toBeInTheDocument();
  });

  it("does not show the row-level warning for a transient (non-auth) failure", async () => {
    mockGetSyncHistory.mockResolvedValue({
      events: [
        {
          id: "ev-3",
          startedAt: undefined,
          finishedAt: undefined,
          itemsCreated: 0,
          itemsUpdated: 0,
          itemsSkipped: 0,
          itemsErrored: 1,
          errorMessage: "rate limited, try again later",
        },
      ],
      truncated: false,
    });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    expect(screen.queryByTestId("source-row-src-1-auth-warning")).not.toBeInTheDocument();
  });

  it("BacklogSourcesSettings_should_NotShowRowLevelWarning_When_MostRecentEventIsRateLimited403", async () => {
    // Regression test: GitHub's rate-limit response is also a 403
    // ("github_issues: rate limited (status 403)"), which used to
    // false-positive against the old bare-"403" isAuthFailure match.
    mockGetSyncHistory.mockResolvedValue({
      events: [
        {
          id: "ev-4",
          startedAt: undefined,
          finishedAt: undefined,
          itemsCreated: 0,
          itemsUpdated: 0,
          itemsSkipped: 0,
          itemsErrored: 1,
          errorMessage: "github_issues: rate limited (status 403)",
        },
      ],
      truncated: false,
    });
    render(<BacklogSourcesSettings />);
    await waitFor(() => expect(screen.getByText("Acme Issues")).toBeInTheDocument());

    expect(screen.queryByTestId("source-row-src-1-auth-warning")).not.toBeInTheDocument();
  });

  it("BacklogSourcesSettings_should_ShowRowLevelWarning_When_MostRecentEventIsBadCredentials401", async () => {
    mockGetSyncHistory.mockResolvedValue({
      events: [
        {
          id: "ev-5",
          startedAt: undefined,
          finishedAt: undefined,
          itemsCreated: 0,
          itemsUpdated: 0,
          itemsSkipped: 0,
          itemsErrored: 1,
          errorMessage: "github_issues: unexpected status 401: Bad credentials",
        },
      ],
      truncated: false,
    });
    render(<BacklogSourcesSettings />);

    expect(await screen.findByTestId("source-row-src-1-auth-warning")).toBeInTheDocument();
  });
});
