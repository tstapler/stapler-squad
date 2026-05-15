/**
 * Tests for EscapeAnalyticsPage and its sub-components.
 *
 * Covers:
 *  - SequenceHistogram renders a bar for each sequence type
 *  - SequenceHistogram shows empty state when no entries
 *  - MangleRateIndicator shows green badge for rate < 1%
 *  - MangleRateIndicator shows yellow badge for rate 1-5%
 *  - MangleRateIndicator shows red badge for rate > 5%
 *  - MangleRateIndicator shows absolute counts
 *  - EscapeEventTable renders event rows
 *  - EscapeEventTable highlights mangled rows
 *  - EscapeEventTable shows "Load more" when hasMore=true
 *  - EscapeEventTable renders raw_bytes as hex string (never raw)
 *  - EscapeEventTable shows empty state when no events
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SequenceHistogram } from "./SequenceHistogram";
import { MangleRateIndicator } from "./MangleRateIndicator";
import { EscapeEventTable } from "./EscapeEventTable";
import type { EscapeSequenceCount, EscapeEventProto } from "@/gen/session/v1/session_pb";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeHistogramEntry(
  sequenceType: string,
  count: bigint,
  mangledCount: bigint
): EscapeSequenceCount {
  return {
    sequenceType,
    count,
    mangledCount,
  } as unknown as EscapeSequenceCount;
}

function makeEscapeEvent(overrides: Partial<{
  id: string;
  sessionId: string;
  stage: string;
  sequenceType: string;
  sequenceSubtype: string;
  byteLength: number;
  rawBytes: Uint8Array;
  mangled: boolean;
  mangleType: string;
  wallTime: unknown;
  sessionSeq: bigint;
  payloadHash: string;
}>= {}): EscapeEventProto {
  return {
    id: "evt-1",
    sessionId: "session-1",
    stage: "output",
    sequenceType: "CSI",
    sequenceSubtype: "m",
    byteLength: 4,
    rawBytes: new Uint8Array([0x1b, 0x5b, 0x30, 0x6d]),
    mangled: false,
    mangleType: "",
    wallTime: undefined,
    sessionSeq: 1n,
    payloadHash: "",
    ...overrides,
  } as unknown as EscapeEventProto;
}

// ---------------------------------------------------------------------------
// SequenceHistogram tests
// ---------------------------------------------------------------------------

describe("SequenceHistogram", () => {
  it("renders a listitem for each histogram entry", () => {
    const histogram = [
      makeHistogramEntry("CSI", 100n, 2n),
      makeHistogramEntry("OSC", 50n, 0n),
    ];

    render(<SequenceHistogram histogram={histogram} />);

    expect(screen.getByText("CSI")).toBeInTheDocument();
    expect(screen.getByText("OSC")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });

  it("shows empty state when histogram is empty", () => {
    render(<SequenceHistogram histogram={[]} />);
    expect(screen.getByText(/no escape sequences recorded/i)).toBeInTheDocument();
  });

  it("renders the list with correct aria-label", () => {
    const histogram = [makeHistogramEntry("CSI", 10n, 1n)];
    render(<SequenceHistogram histogram={histogram} />);
    expect(screen.getByRole("list", { name: /escape sequence histogram/i })).toBeInTheDocument();
  });

  it("shows count in label", () => {
    const histogram = [makeHistogramEntry("DCS", 42n, 3n)];
    render(<SequenceHistogram histogram={histogram} />);
    // Count label shows "42" somewhere
    expect(screen.getByText(/42/)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// MangleRateIndicator tests
// ---------------------------------------------------------------------------

describe("MangleRateIndicator", () => {
  it("shows green badge (Healthy) for rate < 1%", () => {
    render(
      <MangleRateIndicator
        mangleRate={0.005}
        totalSequences={1000n}
        totalMangled={5n}
      />
    );
    const badge = screen.getByTestId("mangle-rate-badge");
    expect(badge).toHaveTextContent("Healthy");
    expect(badge).toHaveAttribute("data-severity", "good");
  });

  it("shows yellow badge (Elevated) for rate between 1% and 5%", () => {
    render(
      <MangleRateIndicator
        mangleRate={0.03}
        totalSequences={1000n}
        totalMangled={30n}
      />
    );
    const badge = screen.getByTestId("mangle-rate-badge");
    expect(badge).toHaveTextContent("Elevated");
    expect(badge).toHaveAttribute("data-severity", "warning");
  });

  it("shows red badge (High) for rate > 5%", () => {
    render(
      <MangleRateIndicator
        mangleRate={0.1}
        totalSequences={1000n}
        totalMangled={100n}
      />
    );
    const badge = screen.getByTestId("mangle-rate-badge");
    expect(badge).toHaveTextContent("High");
    expect(badge).toHaveAttribute("data-severity", "error");
  });

  it("displays the percentage value", () => {
    render(
      <MangleRateIndicator
        mangleRate={0.025}
        totalSequences={200n}
        totalMangled={5n}
      />
    );
    expect(screen.getByTestId("mangle-rate-value")).toHaveTextContent("2.50%");
  });

  it("shows total sequences and mangled counts", () => {
    render(
      <MangleRateIndicator
        mangleRate={0.05}
        totalSequences={500n}
        totalMangled={25n}
      />
    );
    const counts = screen.getByTestId("mangle-counts");
    expect(counts).toHaveTextContent("500");
    expect(counts).toHaveTextContent("25");
  });
});

// ---------------------------------------------------------------------------
// EscapeEventTable tests
// ---------------------------------------------------------------------------

describe("EscapeEventTable", () => {
  it("renders a row for each event", () => {
    const events = [
      makeEscapeEvent({ id: "evt-1", sequenceType: "CSI" }),
      makeEscapeEvent({ id: "evt-2", sequenceType: "OSC" }),
    ];

    render(
      <EscapeEventTable events={events} loading={false} onLoadMore={jest.fn()} hasMore={false} />
    );

    const rows = screen.getAllByTestId("escape-event-row");
    expect(rows).toHaveLength(2);
  });

  it("shows empty state when no events", () => {
    render(
      <EscapeEventTable events={[]} loading={false} onLoadMore={jest.fn()} hasMore={false} />
    );
    expect(screen.getByText(/no escape events found/i)).toBeInTheDocument();
  });

  it("highlights mangled rows with data-mangled attribute", () => {
    const events = [
      makeEscapeEvent({ id: "evt-1", mangled: false }),
      makeEscapeEvent({ id: "evt-2", mangled: true }),
    ];

    render(
      <EscapeEventTable events={events} loading={false} onLoadMore={jest.fn()} hasMore={false} />
    );

    const rows = screen.getAllByTestId("escape-event-row");
    expect(rows[0]).toHaveAttribute("data-mangled", "false");
    expect(rows[1]).toHaveAttribute("data-mangled", "true");
  });

  it("shows 'Load more' button when hasMore=true", () => {
    const events = [makeEscapeEvent({ id: "evt-1" })];
    const onLoadMore = jest.fn();

    render(
      <EscapeEventTable events={events} loading={false} onLoadMore={onLoadMore} hasMore={true} />
    );

    const button = screen.getByTestId("load-more-button");
    expect(button).toBeInTheDocument();
    fireEvent.click(button);
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("does not show 'Load more' when hasMore=false", () => {
    const events = [makeEscapeEvent({ id: "evt-1" })];

    render(
      <EscapeEventTable events={events} loading={false} onLoadMore={jest.fn()} hasMore={false} />
    );

    expect(screen.queryByTestId("load-more-button")).not.toBeInTheDocument();
  });

  it("renders raw_bytes as hex string, not raw bytes", () => {
    // 0x1b 0x5b 0x30 0x6d → "1b 5b 30 6d"
    const rawBytes = new Uint8Array([0x1b, 0x5b, 0x30, 0x6d]);
    const events = [makeEscapeEvent({ id: "evt-hex", rawBytes })];

    render(
      <EscapeEventTable events={events} loading={false} onLoadMore={jest.fn()} hasMore={false} />
    );

    // The hex representation must be present in the DOM
    expect(screen.getByText("1b 5b 30 6d")).toBeInTheDocument();

    // The raw ESC character must NOT appear in the DOM
    const allText = document.body.textContent ?? "";
    expect(allText).not.toContain("\x1b");
  });

  it("shows loading indicator while loading", () => {
    render(
      <EscapeEventTable events={[]} loading={true} onLoadMore={jest.fn()} hasMore={false} />
    );
    // Loading indicator is shown (events empty + loading=true shows loading)
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("shows 'Yes' badge for mangled events", () => {
    const events = [makeEscapeEvent({ id: "evt-1", mangled: true, mangleType: "strip" })];

    render(
      <EscapeEventTable events={events} loading={false} onLoadMore={jest.fn()} hasMore={false} />
    );

    expect(screen.getByText("Yes")).toBeInTheDocument();
  });
});
