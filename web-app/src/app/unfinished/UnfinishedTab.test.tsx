import React from "react";
import { render, screen } from "@testing-library/react";

const mockUseSearchParams = jest.fn();

jest.mock("next/navigation", () => ({
  useSearchParams: () => mockUseSearchParams(),
}));

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    dismissWorktree: jest.fn(),
    snoozeWorktree: jest.fn(),
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("@/lib/hooks/useUnfinishedWork", () => ({
  useUnfinishedWork: () => ({
    worktrees: [],
    lastScanTime: null,
    isScanning: false,
    triggerScan: jest.fn(),
  }),
}));

jest.mock("@/components/unfinished/UnfinishedRepoGroup", () => ({
  UnfinishedRepoGroup: () => <div data-testid="unfinished-repo-group" />,
}));

jest.mock("@/components/unfinished/GitHubPRsSection", () => ({
  GitHubPRsSection: () => <div data-testid="github-prs-section" />,
}));

jest.mock("@/components/unfinished/BacklogQueueSection", () => ({
  BacklogQueueSection: () => <div data-testid="backlog-queue-section" />,
}));

const mockStuckItemsSection = jest.fn();
jest.mock("@/components/backlog-stuck/StuckItemsSection", () => ({
  StuckItemsSection: (props: { focusItemId?: string }) => {
    mockStuckItemsSection(props);
    return <div data-testid="stuck-items-section" />;
  },
}));

import { UnfinishedTab } from "./UnfinishedTab";

describe("UnfinishedTab", () => {
  beforeEach(() => {
    mockStuckItemsSection.mockClear();
  });

  it("UnfinishedTab_should_expandAndFocusItem_When_ItemQueryParamMatches", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "f9fcef32-c27e-434d-b23f-c873c18afa92" }));
    render(<UnfinishedTab />);
    expect(screen.getByTestId("stuck-items-section")).toBeInTheDocument();
    expect(mockStuckItemsSection).toHaveBeenCalledWith(
      expect.objectContaining({ focusItemId: "f9fcef32-c27e-434d-b23f-c873c18afa92" })
    );
  });

  it("UnfinishedTab_should_behaveAsBefore_When_ItemQueryParamAbsentOrUnmatched", () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams());
    render(<UnfinishedTab />);
    expect(screen.getByTestId("stuck-items-section")).toBeInTheDocument();
    expect(mockStuckItemsSection).toHaveBeenCalledWith(
      expect.objectContaining({ focusItemId: undefined })
    );

    mockStuckItemsSection.mockClear();
    mockUseSearchParams.mockReturnValue(new URLSearchParams({ item: "does-not-exist" }));
    render(<UnfinishedTab />);
    expect(mockStuckItemsSection).toHaveBeenCalledWith(
      expect.objectContaining({ focusItemId: "does-not-exist" })
    );
  });
});
