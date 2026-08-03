import React from "react";
import { render, screen } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { BlockerChip } from "./BlockerChip";

function makeItem(overrides: Partial<StuckBacklogItem> = {}): StuckBacklogItem {
  return {
    itemId: "f9fcef32-c27e-434d-b23f-c873c18afa92",
    title: "fix: benchmark job CI",
    status: "in_progress",
    reason: StuckReason.REWORK_CAP,
    firstDetectedAt: timestampFromDate(new Date(Date.now() - 3 * 24 * 60 * 60 * 1000)),
    lastCheckedAt: timestampFromDate(new Date(Date.now() - 30 * 1000)),
    prNumber: 0,
    prUrl: "",
    context: "",
    ...overrides,
  } as StuckBacklogItem;
}

describe("BlockerChip", () => {
  // The jest styleMock for `.css.ts` files wraps every export in a callable
  // proxy function, which triggers a benign "Invalid value for prop
  // className" React warning here (getStuckReasonClass resolves through
  // stuckReason.css's mocked exports). Pre-existing jest/vanilla-extract mock
  // limitation — see BacklogItemDetail.test.tsx for the same silencing.
  beforeAll(() => {
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterAll(() => {
    jest.restoreAllMocks();
  });

  it("BlockerChip_should_RenderIconLabelAndDuration_When_VariantIsFull", () => {
    render(<BlockerChip item={makeItem()} variant="full" />);

    expect(screen.getByText("🔴")).toBeInTheDocument();
    expect(screen.getByText("Rework cap hit")).toBeInTheDocument();
    expect(screen.getByTestId("blocker-chip-duration")).toHaveTextContent("3d");
  });

  it("BlockerChip_should_OmitDurationText_When_VariantIsCompact", () => {
    render(<BlockerChip item={makeItem()} variant="compact" />);

    expect(screen.getByText("🔴")).toBeInTheDocument();
    expect(screen.getByText("Rework cap hit")).toBeInTheDocument();
    expect(screen.queryByTestId("blocker-chip-duration")).not.toBeInTheDocument();
    expect(screen.queryByText("3d")).not.toBeInTheDocument();
  });

  it("renders icon and label together (never color-only) for a second StuckReason", () => {
    render(
      <BlockerChip
        item={makeItem({
          reason: StuckReason.PUSH_FAILED,
          firstDetectedAt: timestampFromDate(new Date(Date.now() - 45 * 60 * 1000)),
        })}
        variant="full"
      />
    );

    expect(screen.getByText("⛔")).toBeInTheDocument();
    expect(screen.getByText("Push/PR-create failed")).toBeInTheDocument();
    expect(screen.getByTestId("blocker-chip-duration")).toHaveTextContent("45m");
  });

  it("gives the chip an accessible label matching the reason text, not color alone", () => {
    render(<BlockerChip item={makeItem()} variant="full" />);
    expect(screen.getByTestId("blocker-chip")).toHaveAttribute("aria-label", "Rework cap hit");
  });

  it("BlockerChip_should_RenderRemediationCountSuffix_When_AttemptsGreaterThanZero", () => {
    render(
      <BlockerChip
        item={makeItem({
          remediationAttempts: 3,
          nextRemediationAt: timestampFromDate(new Date(Date.now() + 10 * 60 * 1000)),
        })}
        variant="compact"
      />
    );
    expect(screen.getByTestId("blocker-chip-attempts")).toHaveTextContent("×3");
    expect(screen.getByTestId("blocker-chip-attempts")).toHaveTextContent("retrying in 10m");
  });

  it("BlockerChip_should_RenderNoSuffix_When_AttemptsIsZero", () => {
    render(<BlockerChip item={makeItem({ remediationAttempts: 0 })} variant="compact" />);
    expect(screen.queryByTestId("blocker-chip-attempts")).not.toBeInTheDocument();
  });

  it("BlockerChip_should_IncludeExhaustedWording_When_AttemptsHitMax", () => {
    render(<BlockerChip item={makeItem({ remediationAttempts: 5 })} variant="compact" />);
    expect(screen.getByTestId("blocker-chip-attempts")).toHaveTextContent("×5");
    expect(screen.getByTestId("blocker-chip-attempts")).toHaveTextContent("(parked)");
    expect(screen.getByTestId("blocker-chip")).toHaveAttribute(
      "aria-label",
      expect.stringContaining("parked")
    );
  });
});
