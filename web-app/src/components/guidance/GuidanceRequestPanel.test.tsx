import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { GuidanceRequestPanel } from "./GuidanceRequestPanel";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";

let mockRequests: PlainGuidanceRequest[] = [];
let mockApplied = true;
const mockAnswer = jest.fn().mockImplementation(() => {
  const promise = Promise.resolve({ data: { applied: mockApplied } });
  return Object.assign(promise, { unwrap: () => promise.then((r) => r.data) });
});

jest.mock("@/lib/api/guidanceApi", () => ({
  useListGuidanceRequestsQuery: () => ({
    data: { requests: mockRequests, pendingCount: mockRequests.filter((r) => r.status === "pending").length, cap: 4 },
    isLoading: false,
    error: null,
  }),
  useAnswerGuidanceRequestMutation: () => [mockAnswer, {}],
}));

function row(overrides: Partial<PlainGuidanceRequest>): PlainGuidanceRequest {
  return {
    id: "gr-1",
    scope: "backlog-item",
    itemId: "item-1",
    sessionUuid: "",
    questionText: "Should we proceed?",
    questionType: "yes-no",
    options: [],
    answer: "",
    status: "pending",
    createdAt: undefined,
    answeredAt: undefined,
    ...overrides,
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockRequests = [];
  mockApplied = true;
});

describe("GuidanceRequestPanel", () => {
  it("renders nothing when there are no requests for the scope", () => {
    const { container } = render(<GuidanceRequestPanel scope="backlog-item" scopeKey="item-1" />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders a pending yes/no question and submits an answer", async () => {
    mockRequests = [row({ id: "gr-yn" })];
    render(<GuidanceRequestPanel scope="backlog-item" scopeKey="item-1" />);

    expect(screen.getByTestId("guidance-request-gr-yn")).toBeInTheDocument();
    expect(screen.getByText("Should we proceed?")).toBeInTheDocument();
    expect(screen.getByTestId("guidance-request-status")).toHaveTextContent("Pending");

    fireEvent.click(screen.getByTestId("guidance-answer-yes-gr-yn"));
    await waitFor(() => expect(mockAnswer).toHaveBeenCalledWith({ id: "gr-yn", answer: "yes" }));
  });

  it("renders multiple-choice options and submits the chosen one", async () => {
    mockRequests = [row({ id: "gr-mc", questionType: "multiple-choice", options: ["A", "B", "C"] })];
    render(<GuidanceRequestPanel scope="backlog-item" scopeKey="item-1" />);

    fireEvent.click(screen.getByTestId("guidance-answer-option-gr-mc-A"));
    await waitFor(() => expect(mockAnswer).toHaveBeenCalledWith({ id: "gr-mc", answer: "A" }));
  });

  it("renders a short-answer form and submits typed text", async () => {
    mockRequests = [row({ id: "gr-sa", questionType: "short-answer" })];
    render(<GuidanceRequestPanel scope="backlog-item" scopeKey="item-1" />);

    fireEvent.change(screen.getByTestId("guidance-answer-input-gr-sa"), { target: { value: "use option B" } });
    fireEvent.click(screen.getByTestId("guidance-answer-submit-gr-sa"));
    await waitFor(() => expect(mockAnswer).toHaveBeenCalledWith({ id: "gr-sa", answer: "use option B" }));
  });

  it("shows an already-answered message when the answer lost the race (applied=false)", async () => {
    mockApplied = false;
    mockRequests = [row({ id: "gr-race" })];
    render(<GuidanceRequestPanel scope="backlog-item" scopeKey="item-1" />);

    fireEvent.click(screen.getByTestId("guidance-answer-yes-gr-race"));
    await waitFor(() => expect(mockAnswer).toHaveBeenCalledWith({ id: "gr-race", answer: "yes" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("This question was already answered by someone else.");
  });

  it("shows the answer and no form for an answered question", () => {
    mockRequests = [row({ id: "gr-done", status: "answered", answer: "yes" })];
    render(<GuidanceRequestPanel scope="backlog-item" scopeKey="item-1" />);

    expect(screen.getByTestId("guidance-request-status")).toHaveTextContent("Answered");
    expect(screen.getByText("Answer: yes")).toBeInTheDocument();
    expect(screen.queryByTestId("guidance-answer-yes-gr-done")).not.toBeInTheDocument();
  });
});
