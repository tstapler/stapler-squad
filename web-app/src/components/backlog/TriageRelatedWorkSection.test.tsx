import React from "react";
import { render, screen, act, fireEvent } from "@testing-library/react";
import { TriageRelatedWorkSection } from "./TriageRelatedWorkSection";
import type { SearchResultItem } from "@/lib/hooks/useHistoryFullTextSearch";

const mockSearch = jest.fn();
const mockClearSearch = jest.fn();
let mockState: {
  results: SearchResultItem[];
  loading: boolean;
  error: Error | null;
} = { results: [], loading: false, error: null };

jest.mock("@/lib/hooks/useHistoryFullTextSearch", () => ({
  useHistoryFullTextSearch: () => ({
    results: mockState.results,
    loading: mockState.loading,
    error: mockState.error,
    search: mockSearch,
    clearSearch: mockClearSearch,
  }),
}));

function makeHit(overrides: Partial<SearchResultItem> = {}): SearchResultItem {
  return {
    sessionId: "session-1",
    sessionName: "Add dark mode toggle",
    project: "/repo",
    messageIndex: 4,
    score: 5.5,
    snippets: [{ text: "we discussed dark mode toggle here", highlightRanges: [], messageRole: "user", messageTime: null }],
    metadata: { isMetadataMatch: false, matchSource: "message_content", model: "", createdAt: new Date("2026-01-01") },
    moreMatchesInSessionCount: 0,
    contextWindow: [],
    bookendFirst: [],
    bookendLast: [],
    ...overrides,
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  jest.useFakeTimers();
  mockState = { results: [], loading: false, error: null };
});

afterEach(() => {
  jest.useRealTimers();
});

describe("TriageRelatedWorkSection", () => {
  it("pre-populates query with backlog item title on mount", () => {
    render(<TriageRelatedWorkSection itemTitle="Add dark mode toggle to settings page" repoPath="/repo" />);

    expect(screen.getByTestId("triage-related-work-input")).toHaveValue("Add dark mode toggle to settings page");

    act(() => {
      jest.advanceTimersByTime(300);
    });

    expect(mockSearch).toHaveBeenCalledWith({
      query: "Add dark mode toggle to settings page",
      project: "/repo",
      groupBySession: true,
      includeContext: true,
      excludeAutomationSessions: true,
      limit: 5,
    });
  });

  it("does not auto-search when itemTitle is empty", () => {
    render(<TriageRelatedWorkSection itemTitle="" repoPath="/repo" />);

    act(() => {
      jest.advanceTimersByTime(300);
    });

    expect(mockSearch).not.toHaveBeenCalled();
    const input = screen.getByTestId("triage-related-work-input");
    expect(input).toHaveValue("");
    expect(input).not.toHaveFocus();
  });

  it("shows reassuring copy when zero matches found", () => {
    mockState = { results: [], loading: false, error: null };
    render(<TriageRelatedWorkSection itemTitle="Add dark mode toggle" repoPath="/repo" />);

    act(() => {
      jest.advanceTimersByTime(300);
    });

    expect(screen.getByTestId("triage-related-work-empty")).toHaveTextContent(
      "No related past sessions found — this looks like new territory."
    );
  });

  it("shows inline alert with retry on search failure", () => {
    mockState = { results: [], loading: false, error: new Error("boom") };
    render(<TriageRelatedWorkSection itemTitle="Add dark mode toggle" repoPath="/repo" />);

    const alert = screen.getByTestId("triage-related-work-error");
    expect(alert).toHaveAttribute("role", "alert");
    expect(alert).toHaveTextContent("Search failed");
    expect(screen.queryByTestId("triage-related-work-empty")).not.toBeInTheDocument();
  });

  it("clicking Retry re-invokes search with the same query and options", () => {
    mockState = { results: [], loading: false, error: new Error("boom") };
    render(<TriageRelatedWorkSection itemTitle="Add dark mode toggle" repoPath="/repo" />);

    act(() => {
      jest.advanceTimersByTime(300);
    });
    mockSearch.mockClear();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(mockSearch).toHaveBeenCalledWith({
      query: "Add dark mode toggle",
      project: "/repo",
      groupBySession: true,
      includeContext: true,
      excludeAutomationSessions: true,
      limit: 5,
    });
  });

  it("SessionHitCard renders as a link targeting the session's history page with sessionId and messageIndex", () => {
    mockState = { results: [makeHit()], loading: false, error: null };
    render(<TriageRelatedWorkSection itemTitle="Add dark mode toggle" repoPath="/repo" />);

    const card = screen.getByTestId("triage-related-work-hit-session-1");
    expect(card.tagName).toBe("A");
    expect(card).toHaveAttribute("href", "/history?sessionId=session-1&messageIndex=4");
    expect(card).toHaveAttribute("target", "_blank");
    expect(card).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("renders results inside a ul/li list and shows the more-matches count", () => {
    mockState = { results: [makeHit({ moreMatchesInSessionCount: 3 })], loading: false, error: null };
    render(<TriageRelatedWorkSection itemTitle="Add dark mode toggle" repoPath="/repo" />);

    const list = screen.getByTestId("triage-related-work-results");
    expect(list.tagName).toBe("UL");
    expect(list.querySelector("li")).not.toBeNull();
    expect(screen.getByText("+3 more matches in this session")).toBeInTheDocument();
  });

  it("exposes an aria-label naming the item title on the search input", () => {
    render(<TriageRelatedWorkSection itemTitle="Add dark mode toggle" repoPath="/repo" />);

    expect(screen.getByTestId("triage-related-work-input")).toHaveAttribute(
      "aria-label",
      "Search past sessions for Add dark mode toggle"
    );
  });
});
