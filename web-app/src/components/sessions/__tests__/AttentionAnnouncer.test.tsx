import React from "react";
import { act, render, screen } from "@testing-library/react";
import { AttentionAnnouncer } from "../AttentionAnnouncer";
import { announceAttention, announceNeedsYou } from "@/lib/attention/announceAttention";
import { AnnouncementQueue, MAX_AGE_MS } from "@/lib/attention/announcementQueue";
import { findLiveNodes, isTimeLike } from "@/test-utils/attentionFixtures";

describe("AnnouncementQueue", () => {
  const fmt = (t: string) => () => t;

  it("speaks the highest priority first", () => {
    const q = new AnnouncementQueue();
    q.enqueue("connection", fmt("Live updates paused"), 0);
    q.enqueue("action-result", fmt("Marked done"), 0);
    q.enqueue("needs-you", fmt("1 session needs you"), 0);
    expect(q.take(100)).toBe("1 session needs you");
    expect(q.take(100)).toBe("Marked done");
    expect(q.take(100)).toBe("Live updates paused");
    expect(q.take(100)).toBeNull();
  });

  it("coalesces same-kind counts inside the window", () => {
    const q = new AnnouncementQueue();
    const fmtN = (n: number) => `${n} sessions need you`;
    q.enqueue("needs-you", fmtN, 0, 1);
    q.enqueue("needs-you", fmtN, 1000, 1);
    q.enqueue("needs-you", fmtN, 1900, 1);
    expect(q.size()).toBe(1);
    expect(q.take(2000)).toBe("3 sessions need you");
  });

  it("does not coalesce same-kind entries further apart than the window", () => {
    const q = new AnnouncementQueue();
    q.enqueue("action-result", fmt("a"), 0);
    q.enqueue("action-result", fmt("b"), 5000);
    expect(q.size()).toBe(2);
  });

  it("keeps at most 3 entries, dropping the lowest priority", () => {
    const q = new AnnouncementQueue();
    q.enqueue("connection", fmt("c"), 0);
    q.enqueue("action-result", fmt("r1"), 0);
    q.enqueue("action-result", fmt("r2"), 5000);
    q.enqueue("needs-you", fmt("n"), 6000);
    expect(q.size()).toBe(3);
    const spoken = [q.take(6001), q.take(6001), q.take(6001)];
    expect(spoken).toEqual(["n", "r1", "r2"]);
  });

  it("drops entries older than 10 s", () => {
    const q = new AnnouncementQueue();
    q.enqueue("needs-you", fmt("old"), 0);
    expect(q.take(MAX_AGE_MS + 1)).toBeNull();
  });
});

describe("AttentionAnnouncer", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("renders exactly one polite live region", () => {
    const { container } = render(<AttentionAnnouncer />);
    const live = findLiveNodes(container);
    expect(live).toHaveLength(1);
    expect(live[0]).toHaveAttribute("aria-live", "polite");
  });

  it("speaks one coalesced message for a burst and never moves focus", () => {
    render(
      <>
        <button>anchor</button>
        <AttentionAnnouncer />
      </>,
    );
    screen.getByText("anchor").focus();
    act(() => {
      announceNeedsYou(1);
      jest.advanceTimersByTime(700);
      announceNeedsYou(1);
      jest.advanceTimersByTime(700);
      announceNeedsYou(1);
    });
    expect(screen.getByTestId("attention-announcer")).toHaveTextContent("");
    act(() => {
      jest.advanceTimersByTime(3000);
    });
    expect(screen.getByTestId("attention-announcer")).toHaveTextContent("3 sessions need you");
    expect(screen.getByText("anchor")).toHaveFocus();
  });

  it("speaks queued messages in priority order, spaced apart", () => {
    render(<AttentionAnnouncer />);
    act(() => {
      announceAttention("connection", "Live updates paused");
      announceAttention("needs-you", "1 session needs you");
      jest.advanceTimersByTime(2600);
    });
    const region = screen.getByTestId("attention-announcer");
    expect(region).toHaveTextContent("1 session needs you");
    act(() => {
      jest.advanceTimersByTime(2100);
    });
    expect(region).toHaveTextContent("Live updates paused");
  });

  it("still speaks when the rest of the page is inert", () => {
    render(
      <>
        <div inert data-testid="page">
          page
        </div>
        <AttentionAnnouncer />
      </>,
    );
    const region = screen.getByTestId("attention-announcer");
    expect(region.closest("[inert], [aria-hidden='true']")).toBeNull();
    act(() => {
      announceAttention("needs-you", "1 session needs you");
      jest.advanceTimersByTime(2600);
    });
    expect(region).toHaveTextContent("1 session needs you");
  });

  it("speaks no timestamps for the standard messages", () => {
    for (const text of ["3 sessions need you", "Live updates paused", "Updates resumed", "Alerts muted for 1 hour"]) {
      expect(isTimeLike(text)).toBe(false);
    }
    expect(isTimeLike("Live updates paused, last update 14:02:11")).toBe(true);
    expect(isTimeLike("waiting 5m")).toBe(true);
  });
});
