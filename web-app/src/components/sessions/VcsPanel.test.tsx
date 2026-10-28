import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { VCSStatusSchema, FileChangeSchema, SessionSchema, FileStatus } from "@/gen/session/v1/types_pb";
import { VcsPanel } from "./VcsPanel";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { rateLimitError } from "@/lib/vcs/__testUtils__/rateLimitFixtures";

jest.mock("@/lib/contexts/SessionVcsContext", () => ({
  useSessionVcsContext: jest.fn(),
}));

jest.mock("./DiffPane", () => ({
  DiffPane: ({ content }: { content: string }) => <pre data-testid="mock-diff-pane">{content}</pre>,
}));

jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const mockUseSessionVcsContext = useSessionVcsContext as jest.Mock;

function mockVcsError(error: Error) {
  mockUseSessionVcsContext.mockReturnValue({
    status: null,
    statusLoading: false,
    error,
    refresh: jest.fn(),
  });
}

describe("VcsPanel", () => {
  it("VcsPanel_should_RenderThroughVcsWidgetLoadedTestid_When_SessionVcsContextResolves", () => {
    const status = create(VCSStatusSchema, {
      branch: "feat/vcs-widget",
      isClean: false,
      unstagedFiles: [
        create(FileChangeSchema, { path: "src/foo.ts", status: FileStatus.MODIFIED, additions: 3, deletions: 1 }),
      ],
    });
    const session = create(SessionSchema, {
      githubOwner: "tstapler",
      githubRepo: "stapler-squad",
      githubPrNumber: 42,
      githubCheckConclusion: "success",
    });
    mockUseSessionVcsContext.mockReturnValue({
      status,
      statusLoading: false,
      error: null,
      refresh: jest.fn(),
    });

    render(<VcsPanel session={session} />);

    const widget = screen.getByTestId("vcs-widget-loaded");
    expect(widget).toBeInTheDocument();
    expect(screen.getByText("feat/vcs-widget")).toBeInTheDocument();
    expect(screen.getByText(/#42/)).toBeInTheDocument();
    expect(screen.getByText("src/foo.ts")).toBeInTheDocument();
    expect(screen.getByText(/success/i)).toBeInTheDocument();
  });

  it("VcsPanel_should_RenderLoadingSkeletonNotStaleMarkup_When_VcsStatusNotYetResolved", () => {
    mockUseSessionVcsContext.mockReturnValue({
      status: null,
      statusLoading: true,
      error: null,
      refresh: jest.fn(),
    });

    render(<VcsPanel />);

    expect(screen.getByRole("status", { name: "Loading VCS status" })).toBeInTheDocument();
    expect(screen.queryByTestId("vcs-widget-loaded")).not.toBeInTheDocument();
  });

  it("VcsPanel_should_RenderFriendlyRateLimitCopy_When_ErrorCarriesTransientReasonMarker", () => {
    const error = rateLimitError("transient", 20_000);
    mockVcsError(error);

    render(<VcsPanel />);

    expect(
      screen.getByText(
        /^GitHub is temporarily rate-limited — this should clear up in about .+, retrying automatically\.$/
      )
    ).toBeInTheDocument();
    expect(screen.queryByText(error.message)).not.toBeInTheDocument();
  });

  it("VcsPanel_should_RenderFriendlyRateLimitCopy_When_ErrorCarriesExhaustedReasonMarker", () => {
    const error = rateLimitError("exhausted", 4 * 60_000);
    mockVcsError(error);

    render(<VcsPanel />);

    expect(screen.getByText(/^GitHub rate limit reached — try again in ~.+\.$/)).toBeInTheDocument();
    expect(screen.queryByText(error.message)).not.toBeInTheDocument();
  });

  it("VcsPanel_should_RenderErrorBoxAsPoliteLiveRegion_When_ErrorPresent", () => {
    mockVcsError(new Error("network error: connection refused"));

    render(<VcsPanel />);

    const errorBox = screen.getByRole("status");
    expect(errorBox).toHaveAttribute("aria-live", "polite");
  });

  it("VcsPanel_should_RenderRawMessageFallback_When_ErrorHasNoRateLimitReasonMarker", () => {
    const error = new Error("network error: connection refused");
    mockVcsError(error);

    render(<VcsPanel />);

    expect(screen.getByText(error.message)).toBeInTheDocument();
  });

  describe("inline file diff", () => {
    const FOO_DIFF = ["diff --git a/src/foo.ts b/src/foo.ts", "--- a/src/foo.ts", "+++ b/src/foo.ts", "@@ -1,1 +1,1 @@", "-a", "+b", ""].join("\n");
    const status = create(VCSStatusSchema, {
      branch: "feat/x",
      isClean: false,
      unstagedFiles: [
        create(FileChangeSchema, { path: "src/foo.ts", status: FileStatus.MODIFIED, additions: 1, deletions: 1 }),
      ],
    });
    const mockContext = (diffContent: string | null) =>
      mockUseSessionVcsContext.mockReturnValue({
        status,
        statusLoading: false,
        error: null,
        refresh: jest.fn(),
        diff: diffContent === null ? null : { content: diffContent, added: 1, removed: 1 },
        diffContextLines: 0,
        setDiffContextLines: jest.fn(),
      });

    it("VcsPanel_should_ShowFileDiffInPlaceAndRestoreList_When_FileClickedThenBack", () => {
      const onNavigateToFile = jest.fn();
      mockContext(FOO_DIFF);
      render(<VcsPanel onNavigateToFile={onNavigateToFile} />);

      fireEvent.click(screen.getByRole("button", { name: /src\/foo\.ts/ }));
      expect(screen.getByTestId("file-diff-view")).toBeInTheDocument();
      expect(screen.getByTestId("mock-diff-pane")).toHaveTextContent("+b");
      expect(screen.queryByTestId("vcs-widget-loaded")).not.toBeInTheDocument();
      expect(onNavigateToFile).not.toHaveBeenCalled();

      fireEvent.click(screen.getByTestId("file-diff-back"));
      expect(screen.queryByTestId("file-diff-view")).not.toBeInTheDocument();
      expect(screen.getByTestId("vcs-widget-loaded")).toBeInTheDocument();
    });

    it("VcsPanel_should_FallBackToFilesTab_When_DiffHasNoSectionForFile", () => {
      const onNavigateToFile = jest.fn();
      mockContext(null);
      render(<VcsPanel onNavigateToFile={onNavigateToFile} />);

      fireEvent.click(screen.getByRole("button", { name: /src\/foo\.ts/ }));
      expect(onNavigateToFile).toHaveBeenCalledWith("src/foo.ts");
      expect(screen.queryByTestId("file-diff-view")).not.toBeInTheDocument();
    });
  });
});
