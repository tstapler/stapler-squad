/**
 * Tests that BacklogItemDetail renders the description as markdown (bold,
 * links, images) and never executes injected HTML/script — the description
 * text is user-authored and untrusted.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

jest.mock("./SessionMonitor", () => require("./backlogItemDetailTestFixtures").sessionMonitorMock());
jest.mock("./GateVerdictBox", () => require("./backlogItemDetailTestFixtures").gateVerdictBoxMock());
jest.mock("./TriageReviewPanel", () => require("./backlogItemDetailTestFixtures").triageReviewPanelMock());
jest.mock("./TriageLoadingIndicator", () => require("./backlogItemDetailTestFixtures").triageLoadingIndicatorMock());

jest.mock("@/lib/hooks/useSessionRepoPaths", () => require("./backlogItemDetailTestFixtures").useSessionRepoPathsMock());
jest.mock("@/lib/hooks/usePathCompletions", () => require("./backlogItemDetailTestFixtures").usePathCompletionsMock());
jest.mock("@/lib/hooks/useSessionService", () => require("./backlogItemDetailTestFixtures").useSessionServiceMock());
jest.mock("@/lib/analytics", () => require("./backlogItemDetailTestFixtures").analyticsMock());

// Epic 5.3 (backlog-event-driven-updates): BacklogItemDetail now also
// subscribes via useWatchBacklogItems + a Redux selector, and opens its own
// lightweight raw watch stream for archive/removal terminal-state detection
// (Task 5.3.1b/5.3.1c). None of these tests exercise that live-update path,
// so everything is stubbed inert: no live item ever arrives, and the raw
// terminal stream yields no events.
jest.mock("@/lib/hooks/useWatchBacklogItems", () => require("./backlogItemDetailTestFixtures").useWatchBacklogItemsMock());
jest.mock("@/lib/store", () => require("./backlogItemDetailTestFixtures").storeMock());
jest.mock("@connectrpc/connect", () => require("./backlogItemDetailTestFixtures").connectMock());
jest.mock("@connectrpc/connect-web", () => require("./backlogItemDetailTestFixtures").connectWebMock());

// BacklogItemDetail calls useStuckBacklogItems() once and passes the
// resolved StuckBacklogItem down to LifecycleSummary as a prop — stub it so
// this suite never attempts a real ConnectRPC call.
jest.mock("@/lib/hooks/useStuckBacklogItems", () => require("./backlogItemDetailTestFixtures").useStuckBacklogItemsMock());

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
  activityNotes: [],
  autoSpawnSession: false,
  autoCreatePR: false,
};

describe("BacklogItemDetail — description markdown rendering", () => {
  beforeEach(() => {
    getBacklogItem.mockReset();
    // Story 3.1.4's useSectionExpandState persists collapse state to
    // localStorage keyed by itemId — clear between tests so one test's
    // stored preference never leaks into the next test reusing "item-1".
    localStorage.clear();
  });

  // Description now defaults expanded (backlog-description-prominence) — no
  // click needed before asserting on rendered content. This suite renders
  // the real BacklogItemDetail (not the isolated DescriptionSection), so it
  // is the only place that verifies the seed value
  // (BacklogItemDetail.tsx's useSectionExpandState default) and the
  // threaded defaultExpanded prop stay in sync end-to-end — don't assume
  // DescriptionSection.test.tsx already covers this.
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

  it("keeps Description collapsed when a stored per-item preference says so, even though the new default is expanded", async () => {
    getBacklogItem.mockResolvedValue({
      ...baseItem,
      description: "**bold**",
    });
    localStorage.setItem("backlog-detail-section-item-1-description", "false");

    render(<BacklogItemDetail itemId="item-1" />);

    const header = await screen.findByTestId("collapsible-header-description");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByTestId("backlog-description-rendered")).not.toBeInTheDocument();
  });
});
