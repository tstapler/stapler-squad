import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ItemClaimBanner } from "./ItemClaimBanner";

const checkCrossHostClaim = jest.fn();
const resolveClaimDispute = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({ checkCrossHostClaim, resolveClaimDispute }),
}));

const URL_ = "https://github.com/o/r/issues/1";
const heldStatus = (disputed = false) => ({
  enabled: true,
  checked: true,
  claimedAtUnix: 0,
  claim: { externalUrl: URL_, claimingHostId: "host_01K", itemDeepLink: "ssq://hostA/backlog/v1/bl_1", disputed },
});

beforeEach(() => {
  checkCrossHostClaim.mockReset();
  resolveClaimDispute.mockReset();
});

describe("ItemClaimBanner", () => {
  it.each([
    ["RPC failure", null],
    ["feature off", { enabled: false, checked: false, claimedAtUnix: 0 }],
    ["unclaimed", { enabled: true, checked: true, claimedAtUnix: 0 }],
  ])("renders nothing on %s", async (_name, result) => {
    checkCrossHostClaim.mockResolvedValue(result);
    render(<ItemClaimBanner externalUrl={URL_} />);
    await waitFor(() => expect(checkCrossHostClaim).toHaveBeenCalledWith(URL_));
    expect(screen.queryByTestId("claim-conflict-banner")).toBeNull();
  });

  it("names the claiming host when another host holds the claim", async () => {
    checkCrossHostClaim.mockResolvedValue(heldStatus());
    render(<ItemClaimBanner externalUrl={URL_} />);
    expect(await screen.findByTestId("claim-conflict-banner")).toHaveTextContent('claimed by "hostA"');
  });

  it("Override dismisses the banner for the session", async () => {
    jest.spyOn(console, "info").mockImplementation(() => {});
    checkCrossHostClaim.mockResolvedValue(heldStatus());
    render(<ItemClaimBanner externalUrl={URL_} />);
    fireEvent.click(await screen.findByTestId("claim-banner-action"));
    fireEvent.change(screen.getByTestId("claim-override-reason"), { target: { value: "host hostA is gone" } });
    fireEvent.click(screen.getByTestId("claim-override-confirm"));
    await waitFor(() => expect(screen.queryByTestId("claim-conflict-banner")).toBeNull());
  });

  it("Resolve calls ResolveClaimDispute then refetches", async () => {
    checkCrossHostClaim.mockResolvedValueOnce(heldStatus(true)).mockResolvedValue(heldStatus(false));
    resolveClaimDispute.mockResolvedValue(undefined);
    render(<ItemClaimBanner externalUrl={URL_} />);
    fireEvent.click(await screen.findByTestId("claim-banner-action"));
    fireEvent.change(screen.getByTestId("claim-override-reason"), { target: { value: "hostA is right" } });
    fireEvent.click(screen.getByTestId("claim-override-confirm"));
    await waitFor(() => expect(resolveClaimDispute).toHaveBeenCalledWith(URL_, "hostA is right"));
    await waitFor(() => expect(checkCrossHostClaim).toHaveBeenCalledTimes(2));
    expect(await screen.findByTestId("claim-conflict-banner")).toHaveTextContent('claimed by "hostA"');
  });
});
