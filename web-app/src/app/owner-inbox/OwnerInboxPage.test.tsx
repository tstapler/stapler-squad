import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { OwnerInboxPage } from "./OwnerInboxPage";

let mockPending: Array<Record<string, unknown>> = [];
const mockAnswer = jest.fn().mockImplementation(() => {
  const p = Promise.resolve({ applied: true });
  return { unwrap: () => p };
});
const mockMutation = [mockAnswer, {}];

jest.mock("@/lib/nav", () => ({
  Link: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}));
jest.mock("@/lib/api/guidanceApi", () => ({
  useListAllPendingGuidanceRequestsQuery: () => ({ data: mockPending, isLoading: false, error: undefined }),
  useAnswerGuidanceRequestMutation: () => mockMutation,
}));

const req = (over: Record<string, unknown>) => ({
  id: "g1", scope: "backlog-item", itemId: "item-1", questionText: "[Owner action] waive mac proof?",
  options: ["record waiver", "reject"], ...over,
});

describe("OwnerInboxPage", () => {
  beforeEach(() => mockAnswer.mockClear());

  it("lists only owner-action requests and answers with the chosen response", async () => {
    mockPending = [req({}), req({ id: "g2", questionText: "ordinary" })];
    render(<OwnerInboxPage />);
    expect(screen.getByTestId("owner-inbox-item-g1")).toHaveTextContent("waive mac proof?");
    expect(screen.queryByTestId("owner-inbox-item-g2")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("owner-inbox-respond-g1-record waiver"));
    await waitFor(() => expect(mockAnswer).toHaveBeenCalledWith({ id: "g1", answer: "record waiver" }));
  });

  it("shows an empty state when nothing is waiting", () => {
    mockPending = [];
    render(<OwnerInboxPage />);
    expect(screen.getByTestId("owner-inbox-empty")).toBeInTheDocument();
  });
});
