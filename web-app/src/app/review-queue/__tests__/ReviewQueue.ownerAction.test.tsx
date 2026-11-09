// @jest-environment jsdom
import React from "react";
import { render, screen } from "@testing-library/react";
import ReviewQueuePage from "../page";

let mockPending: Array<Record<string, unknown>> = [];

const mockRouter = { push: jest.fn(), replace: jest.fn() };
const mockSearchParams = { get: () => null };
jest.mock("@/lib/nav", () => ({
  useRouter: () => mockRouter,
  useSearchParams: () => mockSearchParams,
}));
jest.mock("@/lib/analytics/usePageView", () => ({ usePageView: jest.fn() }));
jest.mock("@/lib/hooks/useKeyboard", () => ({ useKeyboard: jest.fn() }));
const mockWatch = { items: [], connectionState: "live" };
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({ useWatchBacklogItems: () => mockWatch }));
const mockIndex = { index: new Map() };
jest.mock("@/lib/hooks/useBacklogService", () => ({ useBacklogSessionIndex: () => mockIndex }));
jest.mock("@/lib/api/guidanceApi", () => ({
  useListAllPendingGuidanceRequestsQuery: () => ({ data: mockPending }),
}));
jest.mock("@/components/sessions/ReviewQueuePanel", () => ({ ReviewQueuePanel: () => <div /> }));
jest.mock("@/components/sessions/SessionDetail", () => ({ SessionDetail: () => <div /> }));
const mockQueueCtx = { acknowledgeSession: jest.fn(), items: [] };
const mockSessionCtx = { sessions: [] };
jest.mock("@/lib/contexts/ReviewQueueContext", () => ({ useReviewQueueContext: () => mockQueueCtx }));
jest.mock("@/lib/contexts/SessionServiceContext", () => ({ useSessionServiceContext: () => mockSessionCtx }));

const req = (over: Record<string, unknown>) => ({
  id: "g1", scope: "backlog-item", itemId: "item-1", questionText: "[Owner action] approve the waiver?", ...over,
});

describe("review queue Pending Questions section", () => {
  it("lists only pending request_owner_action requests", () => {
    mockPending = [req({}), req({ id: "g2", questionText: "ordinary question" })];
    render(<ReviewQueuePage />);
    expect(screen.getByTestId("owner-action-section")).toBeInTheDocument();
    expect(screen.getByTestId("owner-action-item-g1")).toHaveTextContent("approve the waiver?");
    expect(screen.queryByTestId("owner-action-item-g2")).not.toBeInTheDocument();
  });

  it("renders no section when nothing is waiting on the owner", () => {
    mockPending = [req({ id: "g2", questionText: "ordinary question" })];
    render(<ReviewQueuePage />);
    expect(screen.queryByTestId("owner-action-section")).not.toBeInTheDocument();
  });
});
