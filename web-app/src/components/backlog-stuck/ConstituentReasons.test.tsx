import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StuckReason } from "@/gen/session/v1/backlog_pb";
import { ConstituentReasons } from "./ConstituentReasons";

const mockTrack = jest.fn();
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: mockTrack }),
}));

const retryTestId = (reason: StuckReason) => `stuck-constituent-retry-${reason}`;

describe("ConstituentReasons", () => {
  beforeEach(() => mockTrack.mockClear());

  it("renders nothing when there are no reasons", () => {
    const { container } = render(<ConstituentReasons itemId="i1" reasons={[]} onTriggerRemediationNow={jest.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders no Retry button without onTriggerRemediationNow", () => {
    render(<ConstituentReasons itemId="i1" reasons={[StuckReason.PR_NEEDS_FIX]} />);
    expect(screen.getByTestId(`stuck-constituent-reason-${StuckReason.PR_NEEDS_FIX}`)).toBeInTheDocument();
    expect(screen.queryByTestId(retryTestId(StuckReason.PR_NEEDS_FIX))).not.toBeInTheDocument();
  });

  it("shows the error message in an alert when the retry rejects", async () => {
    const onTrigger = jest.fn().mockRejectedValue(new Error("remediation parked"));
    render(<ConstituentReasons itemId="i1" reasons={[StuckReason.PR_NEEDS_FIX]} onTriggerRemediationNow={onTrigger} />);

    fireEvent.click(screen.getByTestId(retryTestId(StuckReason.PR_NEEDS_FIX)));

    expect(await screen.findByRole("alert")).toHaveTextContent("remediation parked");
  });

  it("shows Retrying and disables every Retry while a retry is pending, tracking once", async () => {
    let resolve: () => void = () => {};
    const onTrigger = jest.fn().mockReturnValue(new Promise<void>((r) => (resolve = r)));
    render(
      <ConstituentReasons
        itemId="i1"
        reasons={[StuckReason.PR_NEEDS_FIX, StuckReason.BOUNCING]}
        onTriggerRemediationNow={onTrigger}
      />
    );
    const first = screen.getByTestId(retryTestId(StuckReason.PR_NEEDS_FIX));
    const second = screen.getByTestId(retryTestId(StuckReason.BOUNCING));

    fireEvent.click(first);

    await waitFor(() => expect(first).toHaveTextContent("Retrying…"));
    expect(first).toBeDisabled();
    expect(second).toBeDisabled();
    fireEvent.click(second);
    expect(onTrigger).toHaveBeenCalledTimes(1);
    expect(mockTrack).toHaveBeenCalledTimes(1);

    await act(async () => resolve());
    await waitFor(() => expect(first).toHaveTextContent("Retry"));
    expect(second).toBeEnabled();
  });
});
