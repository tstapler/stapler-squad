/**
 * Tests that BacklogItemDetail renders the description as markdown (bold,
 * links, images) and never executes injected HTML/script — the description
 * text is user-authored and untrusted.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

jest.mock("./SessionMonitor", () => ({ SessionMonitor: () => null }));
jest.mock("./GateVerdictBox", () => ({ GateVerdictBox: () => null }));
jest.mock("./TriageReviewPanel", () => ({ TriageReviewPanel: () => null }));
jest.mock("./TriageLoadingIndicator", () => ({ TriageLoadingIndicator: () => null }));

jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession: jest.fn() }),
}));
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

// LifecycleSummary (Story 2.1.4) calls useStuckBacklogItems() internally —
// stub it so this suite never attempts a real ConnectRPC call.
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => ({ items: [], isLoading: false, error: null }),
}));

const getBacklogItem = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

const baseItem: BacklogItem = {
  id: "item-1",
  title: "Rich text item",
  description: "desc",
  status: "idea",
  priority: 3,
  repoPath: "/tmp/repo",
  skipPlanning: false,
  skipReviewGate: false,
  planApproved: false,
  triageStatus: "completed",
  acCriteria: [],
  linkedSessions: [],
  notes: "",
  createdAt: "2026-07-01T00:00:00Z",
  updatedAt: "2026-07-01T00:00:00Z",
  statusEvents: [],
  totalEstimatedCostUsd: 0,
  progressNotes: [],
};

describe("BacklogItemDetail — description markdown rendering", () => {
  beforeEach(() => {
    getBacklogItem.mockReset();
  });

  it("renders bold text and links instead of literal markdown syntax", async () => {
    getBacklogItem.mockResolvedValue({
      ...baseItem,
      description: "**bold** [link](https://example.com)",
    });

    render(<BacklogItemDetail itemId="item-1" />);

    const rendered = await screen.findByTestId("backlog-description-rendered");
    expect(rendered.querySelector("strong")).toHaveTextContent("bold");
    const link = rendered.querySelector("a");
    expect(link).toHaveAttribute("href", "https://example.com");
    expect(rendered.textContent).not.toContain("**bold**");
  });

  it("renders an embedded image", async () => {
    getBacklogItem.mockResolvedValue({
      ...baseItem,
      description: "![screenshot](/api/local/serve/tmp/shot.png)",
    });

    render(<BacklogItemDetail itemId="item-1" />);

    const rendered = await screen.findByTestId("backlog-description-rendered");
    const img = rendered.querySelector("img");
    expect(img).toHaveAttribute("src", "/api/local/serve/tmp/shot.png");
  });

  it("never executes an injected <script> tag in the description", async () => {
    getBacklogItem.mockResolvedValue({
      ...baseItem,
      description: "<script>window.__pwned = true;</script>",
    });

    render(<BacklogItemDetail itemId="item-1" />);

    const rendered = await screen.findByTestId("backlog-description-rendered");
    expect(rendered.querySelector("script")).not.toBeInTheDocument();
    expect((window as unknown as { __pwned?: boolean }).__pwned).toBeUndefined();
  });
});
