import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { PatternsView } from "./PatternsView";
import type { LogEntry } from "@/lib/hooks/useLogViewer";

// PatternsView calls useAnalytics() directly for pattern-expand tracking —
// mock it rather than standing up a real AnalyticsContextProvider, matching
// BacklogItemDetail.test.tsx's convention for the same situation.
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

function makeEntry(overrides: Partial<LogEntry> = {}): LogEntry {
  return {
    id: `id-${Math.random()}`,
    timestamp: "2026-08-25T00:00:00.000Z",
    level: "WARN",
    message: "worktree directory missing, marking as paused path=/tmp/wt-1",
    raw: "worktree directory missing, marking as paused path=/tmp/wt-1",
    ...overrides,
  };
}

describe("PatternsView", () => {
  it("shows an empty state when there are no entries", () => {
    render(<PatternsView entries={[]} />);
    expect(screen.getByText(/no log entries loaded/i)).toBeInTheDocument();
  });

  it("collapses repeated messages into one pattern row with a count", () => {
    const entries = [
      makeEntry({ message: "worktree directory missing, marking as paused path=/tmp/wt-1" }),
      makeEntry({ message: "worktree directory missing, marking as paused path=/tmp/wt-2" }),
      makeEntry({ message: "worktree directory missing, marking as paused path=/tmp/wt-3" }),
    ];

    render(<PatternsView entries={entries} />);

    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText(/worktree directory missing, marking as paused/)).toBeInTheDocument();
    // The raw example messages are not shown until the row is expanded.
    expect(screen.queryByText(/path=\/tmp\/wt-1/)).not.toBeInTheDocument();
  });

  it("expands a pattern row to reveal its raw matching entries", () => {
    const entries = [
      makeEntry({ message: "queue full, dropping path=/tmp/a" }),
      makeEntry({ message: "queue full, dropping path=/tmp/b" }),
    ];

    render(<PatternsView entries={entries} />);

    fireEvent.click(screen.getByRole("button", { expanded: false }));

    expect(screen.getByText(/path=\/tmp\/a/)).toBeInTheDocument();
    expect(screen.getByText(/path=\/tmp\/b/)).toBeInTheDocument();
  });

  it("collapses an expanded row on a second click", () => {
    const entries = [makeEntry({ message: "queue full, dropping path=/tmp/a" })];
    render(<PatternsView entries={entries} />);

    const row = screen.getByRole("button");
    fireEvent.click(row);
    expect(screen.getByText(/path=\/tmp\/a/)).toBeInTheDocument();

    fireEvent.click(row);
    expect(screen.queryByText(/path=\/tmp\/a/)).not.toBeInTheDocument();
  });

  it("caps shown examples and reports how many more exist", () => {
    const entries = Array.from({ length: 25 }, (_, i) =>
      makeEntry({ message: `queue full, dropping path=/tmp/item-${i}` }),
    );

    render(<PatternsView entries={entries} maxExamplesPerPattern={5} />);
    fireEvent.click(screen.getByRole("button"));

    expect(screen.getByText(/…and 20 more/)).toBeInTheDocument();
  });

  it("sorts the most frequent pattern first", () => {
    const entries = [
      makeEntry({ message: "rare one-off event" }),
      makeEntry({ message: "common event" }),
      makeEntry({ message: "common event" }),
      makeEntry({ message: "common event" }),
    ];

    render(<PatternsView entries={entries} />);

    const rows = screen.getAllByRole("button");
    expect(rows[0]).toHaveTextContent("common event");
    expect(rows[1]).toHaveTextContent("rare one-off event");
  });
});
