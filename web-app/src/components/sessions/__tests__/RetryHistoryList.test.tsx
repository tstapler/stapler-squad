import React from "react";
import { render, screen, within } from "@testing-library/react";
import { RetryHistoryList } from "../RetryHistoryList";
import type { RetryAttemptRecord } from "@/gen/session/v1/types_pb";

jest.mock("../CheckpointList.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);

function record(attempt: number, reason: string, seconds: number): RetryAttemptRecord {
  return {
    attempt,
    reason,
    timestamp: { seconds: BigInt(seconds), nanos: 0 },
  } as unknown as RetryAttemptRecord;
}

describe("RetryHistoryList", () => {
  it("RetryHistoryList_should_ShowNoRetriesYetMessage_When_HistoryIsEmpty", () => {
    render(<RetryHistoryList history={[]} />);
    expect(screen.getByText("No retries yet")).toBeInTheDocument();
  });

  it("RetryHistoryList_should_ShowLoadingSkeleton_NotFalseEmptyState_When_DataNotYetFetched", () => {
    render(<RetryHistoryList history={[]} isLoading />);
    expect(screen.queryByText("No retries yet")).not.toBeInTheDocument();
    expect(screen.getByText(/Loading retry history/)).toBeInTheDocument();
  });

  it("RetryHistoryList_should_RenderNewestAttemptFirst_When_MultipleRecordsExist", () => {
    render(
      <RetryHistoryList
        history={[record(1, "crashed", 1000), record(2, "tmux_exited", 2000)]}
      />
    );
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("Attempt 2")).toBeInTheDocument();
    expect(within(items[1]).getByText("Attempt 1")).toBeInTheDocument();
  });

  it("shows a Show all toggle beyond MAX_VISIBLE (10) entries", () => {
    const history = Array.from({ length: 12 }, (_, i) => record(i + 1, "crashed", i));
    render(<RetryHistoryList history={history} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(10);
    expect(screen.getByText("Show all (12)")).toBeInTheDocument();
  });
});
