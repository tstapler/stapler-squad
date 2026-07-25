import React from "react";
import { render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { VCSStatusSchema, FileChangeSchema, SessionSchema, FileStatus } from "@/gen/session/v1/types_pb";
import { VcsPanel } from "./VcsPanel";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";

jest.mock("@/lib/contexts/SessionVcsContext", () => ({
  useSessionVcsContext: jest.fn(),
}));

jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const mockUseSessionVcsContext = useSessionVcsContext as jest.Mock;

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
});
