import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { MAX_REMEDIATION_ATTEMPTS } from "@/components/backlog-stuck/stuckReason";
import { BlockerChip } from "./BlockerChip";

function makeItem(overrides: Partial<StuckBacklogItem> = {}): StuckBacklogItem {
  return {
    itemId: "f9fcef32-c27e-434d-b23f-c873c18afa92",
    title: "fix: benchmark job CI",
    status: "in_progress",
    reason: StuckReason.REWORK_CAP,
    firstDetectedAt: timestampFromDate(new Date(Date.now() - 3 * 24 * 60 * 60 * 1000)),
    lastCheckedAt: timestampFromDate(new Date(Date.now() - 30 * 1000)),
    remediationAttempts: 0,
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

  it("BlockerChip_should_renderAsButton_When_OnTriggerRemediationNowProvidedAndNotParked", () => {
    const onTriggerRemediationNow = jest.fn().mockResolvedValue(undefined);
    render(<BlockerChip item={makeItem()} variant="full" onTriggerRemediationNow={onTriggerRemediationNow} />);

    expect(screen.getByTestId("blocker-chip-retry")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /retry now/i })).toBeEnabled();
  });

  it("BlockerChip_should_callOnTriggerRemediationNow_When_Clicked", async () => {
    const user = userEvent.setup();
    const onTriggerRemediationNow = jest.fn().mockResolvedValue(undefined);
    render(<BlockerChip item={makeItem()} variant="full" onTriggerRemediationNow={onTriggerRemediationNow} />);

    await user.click(screen.getByRole("button", { name: /retry now/i }));

    expect(onTriggerRemediationNow).toHaveBeenCalledTimes(1);
    expect(onTriggerRemediationNow).toHaveBeenCalledWith(
      "f9fcef32-c27e-434d-b23f-c873c18afa92",
      StuckReason.REWORK_CAP
    );
  });

  it("BlockerChip_should_disableAndShowPending_When_RetryInFlight", async () => {
    const user = userEvent.setup();
    let resolveRetry: () => void = () => {};
    const onTriggerRemediationNow = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveRetry = resolve;
        })
    );
    render(<BlockerChip item={makeItem()} variant="full" onTriggerRemediationNow={onTriggerRemediationNow} />);

    const button = screen.getByTestId("blocker-chip-retry");
    await user.click(button);

    expect(button).toBeDisabled();
    expect(screen.getByTestId("blocker-chip-duration")).toHaveTextContent("Retrying…");

    // Double-click / rapid re-click during pending state must not fire a second call.
    await user.click(button);
    expect(onTriggerRemediationNow).toHaveBeenCalledTimes(1);

    resolveRetry();
    await waitFor(() => expect(button).not.toBeDisabled());
  });

  it("BlockerChip_should_showInlineError_When_OnTriggerRemediationNowRejects", async () => {
    const user = userEvent.setup();
    const onTriggerRemediationNow = jest.fn().mockRejectedValue(new Error("item is no longer stuck"));
    render(<BlockerChip item={makeItem()} variant="full" onTriggerRemediationNow={onTriggerRemediationNow} />);

    await user.click(screen.getByTestId("blocker-chip-retry"));

    const errorEl = await screen.findByTestId("blocker-chip-error");
    expect(errorEl).toHaveTextContent("item is no longer stuck");
    expect(screen.getByTestId("blocker-chip-retry")).toBeEnabled();
  });

  it("BlockerChip_should_disableWithParkedLabel_When_ItemIsParked", () => {
    const onTriggerRemediationNow = jest.fn().mockResolvedValue(undefined);
    render(
      <BlockerChip
        item={makeItem({ remediationAttempts: MAX_REMEDIATION_ATTEMPTS })}
        variant="full"
        onTriggerRemediationNow={onTriggerRemediationNow}
      />
    );

    const button = screen.getByRole("button", { name: /retry unavailable/i });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("title");
  });

  it("BlockerChip_should_renderReadOnlySpan_When_VariantCompactOrHandlerOmitted", () => {
    const onTriggerRemediationNow = jest.fn().mockResolvedValue(undefined);
    render(<BlockerChip item={makeItem()} variant="compact" onTriggerRemediationNow={onTriggerRemediationNow} />);

    expect(screen.queryByTestId("blocker-chip-retry")).not.toBeInTheDocument();
    expect(screen.getByTestId("blocker-chip")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
  describe("claim-blocked reason", () => {
    const claimItem = () => makeItem({ reason: StuckReason.BLOCKED_BY_CLAIM });

    it("BlockerChip_should_OpenReasonFormInsteadOfFiringImmediately_When_ReasonIsBlockedByClaim", async () => {
      const user = userEvent.setup();
      const onOverrideClaimBlock = jest.fn().mockResolvedValue(undefined);
      const onTriggerRemediationNow = jest.fn().mockResolvedValue(undefined);
      render(
        <BlockerChip
          item={claimItem()}
          variant="full"
          onTriggerRemediationNow={onTriggerRemediationNow}
          onOverrideClaimBlock={onOverrideClaimBlock}
        />
      );

      await user.click(screen.getByTestId("blocker-chip-claim-override"));

      expect(screen.getByTestId("claim-override-form")).toBeInTheDocument();
      expect(onOverrideClaimBlock).not.toHaveBeenCalled();
      expect(onTriggerRemediationNow).not.toHaveBeenCalled();
    });

    it("keeps Confirm disabled below 5 characters, then submits the trimmed reason", async () => {
      const user = userEvent.setup();
      const onOverrideClaimBlock = jest.fn().mockResolvedValue(undefined);
      render(<BlockerChip item={claimItem()} variant="full" onOverrideClaimBlock={onOverrideClaimBlock} />);

      await user.click(screen.getByTestId("blocker-chip-claim-override"));
      const confirm = screen.getByTestId("claim-override-confirm");
      expect(confirm).toBeDisabled();

      await user.type(screen.getByTestId("claim-override-reason"), "  ab  ");
      expect(confirm).toBeDisabled();

      await user.type(screen.getByTestId("claim-override-reason"), "cdef");
      expect(confirm).toBeEnabled();
      await user.click(confirm);

      await waitFor(() => expect(onOverrideClaimBlock).toHaveBeenCalledWith(claimItem().itemId, "ab  cdef"));
      await waitFor(() => expect(screen.queryByTestId("claim-override-form")).not.toBeInTheDocument());
    });

    it("shows the server error and keeps the form open when the override fails", async () => {
      const user = userEvent.setup();
      const onOverrideClaimBlock = jest.fn().mockRejectedValue(new Error("no free work slot"));
      render(<BlockerChip item={claimItem()} variant="full" onOverrideClaimBlock={onOverrideClaimBlock} />);

      await user.click(screen.getByTestId("blocker-chip-claim-override"));
      await user.type(screen.getByTestId("claim-override-reason"), "valid reason");
      await user.click(screen.getByTestId("claim-override-confirm"));

      expect(await screen.findByTestId("claim-override-error")).toHaveTextContent("no free work slot");
      expect(screen.getByTestId("claim-override-form")).toBeInTheDocument();
    });

    it("renders read-only when no override handler is supplied", () => {
      render(<BlockerChip item={claimItem()} variant="full" />);
      expect(screen.getByTestId("blocker-chip")).toBeInTheDocument();
      expect(screen.queryByRole("button")).not.toBeInTheDocument();
    });
  });
});
