/**
 * handlePickerSelect's "already imported" duplicate-count message (PR #663):
 * when importGitHubIssue resolves with `alreadyExisted: true`, the picker
 * modal must stay open (not navigate away via setShowForm(false)) and show
 * an error-styled "already imported" message instead of silently succeeding.
 *
 * Also covers the third import outcome (cross_host_claim_dedup): an issue
 * another host already claimed renders its own notice with Copy link and an
 * audited "Import anyway" reason form.
 *
 * Reuses the shared BacklogPage test harness (backlogPageTestFixtures.ts,
 * same pattern as BacklogPage.exitTransition.test.tsx etc.) rather than
 * building a new one -- only the GitHubIssuePicker and useBacklogService
 * mocks are overridden here to drive handlePickerSelect directly.
 */

import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import BacklogPage from "../page";
import { itemFixture, mockUseWatchBacklogItems } from "./backlogPageTestFixtures";
import type { GitHubIssue } from "@/lib/hooks/useBacklogService";

const importGitHubIssue = jest.fn();

jest.mock("next/navigation", () => require("./backlogPageTestFixtures").nextNavigationMock());
jest.mock("@/lib/analytics", () => require("./backlogPageTestFixtures").analyticsMock());
jest.mock("@/lib/analytics/usePageView", () => require("./backlogPageTestFixtures").usePageViewMock());
jest.mock("@/components/backlog/BacklogItemDetail", () => require("./backlogPageTestFixtures").backlogItemDetailMock());
jest.mock("@/components/backlog/BacklogItemForm", () => require("./backlogPageTestFixtures").backlogItemFormMock());
jest.mock("@/components/backlog/BacklogEmptyState", () => require("./backlogPageTestFixtures").backlogEmptyStateMock());
jest.mock("@/components/backlog/VaguenessPromptModal", () => require("./backlogPageTestFixtures").vaguenessPromptModalMock());
jest.mock("@/components/backlog/BacklogTourModal", () => require("./backlogPageTestFixtures").backlogTourModalMock());
jest.mock("@connectrpc/connect", () => require("./backlogPageTestFixtures").connectMock());
jest.mock("@connectrpc/connect-web", () => require("./backlogPageTestFixtures").connectWebMock());
jest.mock("@/lib/config", () => require("./backlogPageTestFixtures").configMock());
jest.mock("@/lib/store", () => require("./backlogPageTestFixtures").storeMock());
jest.mock("@/lib/hooks/useWatchBacklogItems", () => require("./backlogPageTestFixtures").useWatchBacklogItemsMock());

// Overrides useBacklogServiceMockFactory's importGitHubIssue with a
// controllable jest.fn() so each test can resolve it with a chosen
// `alreadyExisted` value.
jest.mock("@/lib/hooks/useBacklogService", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogService");
  return {
    ...actual,
    useBacklogService: () => ({
      createBacklogItem: jest.fn(),
      importGitHubIssue,
      triggerTriage: jest.fn(),
    }),
  };
});

// Real GitHubIssuePicker requires its own repo/issue search flow -- stand in
// with a button that invokes onSelect directly with one fixed issue, so the
// test can drive handlePickerSelect without re-testing GitHubIssuePicker's
// own search UI (already covered by GitHubIssuePicker.test.tsx).
const SELECTED_ISSUE: GitHubIssue = {
  number: 42,
  title: "Some issue",
  state: "open",
  url: "https://github.com/octocat/hello-world/issues/42",
  labels: [],
  isPR: false,
  host: "",
};

jest.mock("@/components/backlog/GitHubIssuePicker", () => ({
  GitHubIssuePicker: ({
    onSelect,
  }: {
    onSelect: (owner: string, repo: string, issues: GitHubIssue[]) => void;
  }) => (
    <button
      type="button"
      onClick={() => onSelect("octocat", "hello-world", [SELECTED_ISSUE])}
    >
      Mock select issue
    </button>
  ),
}));

function openGitHubImportModal() {
  mockUseWatchBacklogItems.mockReturnValue({ items: [itemFixture({})], connectionState: "live" });
  render(<BacklogPage />);
  fireEvent.click(screen.getByTestId("backlog-new-item-button"));
  fireEvent.click(screen.getByRole("button", { name: "Import from GitHub Issue" }));
}

describe("BacklogPage handlePickerSelect — duplicate feedback (PR #663)", () => {
  beforeEach(() => {
    localStorage.clear();
    jest.clearAllMocks();
  });

  it("shows the all-duplicates message and keeps the modal open when every issue was already imported", async () => {
    importGitHubIssue.mockResolvedValue({
      item: itemFixture({ id: "item-dup-1" }),
      triageTriggered: false,
      alreadyExisted: true,
    });

    openGitHubImportModal();
    fireEvent.click(screen.getByRole("button", { name: "Mock select issue" }));

    expect(await screen.findByText("Already imported — no new items created.")).toBeInTheDocument();
    // Modal stays open -- handlePickerSelect must not call setShowForm(false)
    // on the duplicates branch, or this message would never be visible.
    expect(screen.getByRole("dialog", { name: "Create new backlog item" })).toBeInTheDocument();
  });

  it("does not show a duplicate message when the issue was newly created", async () => {
    importGitHubIssue.mockResolvedValue({
      item: itemFixture({ id: "item-new-1" }),
      triageTriggered: false,
      alreadyExisted: false,
    });

    openGitHubImportModal();
    fireEvent.click(screen.getByRole("button", { name: "Mock select issue" }));

    await waitFor(() => expect(importGitHubIssue).toHaveBeenCalled());
    expect(screen.queryByText("Already imported — no new items created.")).not.toBeInTheDocument();
    // A single successful import closes the modal (not asserting navigation
    // here -- next/navigation's router.push is mocked to a no-op).
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Create new backlog item" })).not.toBeInTheDocument()
    );
  });
});

const CLAIM = {
  externalUrl: SELECTED_ISSUE.url,
  claimingHostId: "host_abc123",
  itemDeepLink: "ssq://hostA.example/backlog/v1/bl_01J",
  disputed: false,
};

describe("BacklogPage handlePickerSelect — claimed by another host (cross_host_claim_dedup)", () => {
  beforeEach(() => {
    localStorage.clear();
    jest.clearAllMocks();
  });

  it("handlePickerSelect_should_RenderAlreadyClaimedElsewhereBucketWithHostAndCopyLink_When_ImportResultIncludesClaimMetadata", async () => {
    importGitHubIssue.mockResolvedValue({ alreadyClaimedElsewhere: CLAIM });
    const writeText = jest.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });

    openGitHubImportModal();
    fireEvent.click(screen.getByRole("button", { name: "Mock select issue" }));

    const notice = await screen.findByTestId("claimed-elsewhere-notice");
    expect(within(notice).getByText(/claimed by hostA\.example/)).toBeInTheDocument();
    // Distinct from the same-host "already imported" and generic-failure lines.
    expect(screen.queryByText("Already imported — no new items created.")).not.toBeInTheDocument();
    expect(screen.queryByText(/Import failed/)).not.toBeInTheDocument();
    // The modal stays open so Copy link / Import anyway remain reachable.
    expect(screen.getByRole("dialog", { name: "Create new backlog item" })).toBeInTheDocument();

    fireEvent.click(within(notice).getByTestId("claimed-elsewhere-copy"));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(CLAIM.itemDeepLink));
  });

  it("handlePickerSelect_should_ResubmitWithOverrideTrueAndReason_When_ImportAnywayConfirmedWithValidReason", async () => {
    importGitHubIssue
      .mockResolvedValueOnce({ alreadyClaimedElsewhere: CLAIM })
      .mockResolvedValueOnce({ item: itemFixture({ id: "item-override-1" }), triageTriggered: false, alreadyExisted: false });

    openGitHubImportModal();
    fireEvent.click(screen.getByRole("button", { name: "Mock select issue" }));
    const notice = await screen.findByTestId("claimed-elsewhere-notice");

    fireEvent.click(within(notice).getByTestId("claimed-elsewhere-import-anyway"));
    const confirm = screen.getByTestId("claim-override-confirm");
    expect(confirm).toBeDisabled();

    fireEvent.change(screen.getByTestId("claim-override-reason"), { target: { value: "abcd" } });
    expect(confirm).toBeDisabled();
    fireEvent.change(screen.getByTestId("claim-override-reason"), { target: { value: "  stale claim, host is gone  " } });
    expect(confirm).toBeEnabled();
    fireEvent.click(confirm);

    await waitFor(() => expect(importGitHubIssue).toHaveBeenCalledTimes(2));
    expect(importGitHubIssue).toHaveBeenLastCalledWith(SELECTED_ISSUE.url, { overrideReason: "stale claim, host is gone" });
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Create new backlog item" })).not.toBeInTheDocument()
    );
  });

  it("keeps the notice and shows the failure on the form when the override import fails", async () => {
    importGitHubIssue.mockResolvedValueOnce({ alreadyClaimedElsewhere: CLAIM }).mockResolvedValueOnce(null);

    openGitHubImportModal();
    fireEvent.click(screen.getByRole("button", { name: "Mock select issue" }));
    const notice = await screen.findByTestId("claimed-elsewhere-notice");
    fireEvent.click(within(notice).getByTestId("claimed-elsewhere-import-anyway"));
    fireEvent.change(screen.getByTestId("claim-override-reason"), { target: { value: "valid reason" } });
    fireEvent.click(screen.getByTestId("claim-override-confirm"));

    expect(await screen.findByTestId("claim-override-error")).toBeInTheDocument();
    expect(screen.getByTestId("claimed-elsewhere-notice")).toBeInTheDocument();
  });
});
