import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ClaimConflictBanner } from "./ClaimConflictBanner";

const LINK = "ssq://hostA/backlog/v1/bl_01J";

beforeEach(() => {
  Object.assign(navigator, { clipboard: { writeText: jest.fn().mockResolvedValue(undefined) } });
});

describe("ClaimConflictBanner", () => {
  it("is a polite status region naming the host literally", () => {
    render(<ClaimConflictBanner hostname="hostA" itemDeepLink={LINK} lastSeenAt="2h ago" />);
    const banner = screen.getByTestId("claim-conflict-banner");
    expect(banner).toHaveAttribute("role", "status");
    expect(banner).toHaveAttribute("aria-live", "polite");
    expect(banner).not.toHaveAttribute("role", "alert");
    expect(banner.textContent).toContain('claimed by "hostA"');
    expect(banner.textContent).toContain("2h ago");
    expect(banner.textContent).not.toMatch(/elsewhere"/);
  });

  it("announces the copied state via the live region and copies the deep link", async () => {
    render(<ClaimConflictBanner hostname="hostA" itemDeepLink={LINK} />);
    fireEvent.click(screen.getByTestId("claim-banner-copy"));
    await waitFor(() => expect(screen.getByTestId("claim-banner-copy-status").textContent).toMatch(/copied/i));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(LINK);
    expect(screen.getByTestId("claim-banner-copy")).toHaveTextContent("✓ Copied");
  });

  it("Override requires a reason of at least 5 characters", async () => {
    const onOverride = jest.fn().mockResolvedValue(undefined);
    render(<ClaimConflictBanner hostname="hostA" itemDeepLink={LINK} onOverride={onOverride} />);
    fireEvent.click(screen.getByTestId("claim-banner-action"));
    expect(screen.getByTestId("claim-banner-action")).toHaveTextContent("Override — work on it here");
    const confirm = screen.getByTestId("claim-override-confirm");
    expect(confirm).toBeDisabled();
    fireEvent.change(screen.getByTestId("claim-override-reason"), { target: { value: "host is gone" } });
    expect(confirm).toBeEnabled();
    fireEvent.click(confirm);
    await waitFor(() => expect(onOverride).toHaveBeenCalledWith("host is gone"));
  });

  it("disputed variant says so and offers Resolve, not Override", () => {
    const onResolveDispute = jest.fn();
    render(<ClaimConflictBanner hostname="hostA" itemDeepLink={LINK} disputed onResolveDispute={onResolveDispute} />);
    const banner = screen.getByTestId("claim-conflict-banner");
    expect(banner.textContent).toMatch(/disputed/i);
    expect(banner.textContent).not.toMatch(/Override/);
    expect(screen.getByTestId("claim-banner-action")).toHaveTextContent("Resolve — work on it here");
  });

  it("shows the form error when the action rejects", async () => {
    const onOverride = jest.fn().mockRejectedValue(new Error("nope"));
    render(<ClaimConflictBanner hostname="hostA" itemDeepLink={LINK} onOverride={onOverride} />);
    fireEvent.click(screen.getByTestId("claim-banner-action"));
    fireEvent.change(screen.getByTestId("claim-override-reason"), { target: { value: "long enough" } });
    fireEvent.click(screen.getByTestId("claim-override-confirm"));
    await waitFor(() => expect(screen.getByTestId("claim-override-error")).toHaveTextContent("nope"));
  });

  it("indeterminate variant offers only Retry check", () => {
    const onRetry = jest.fn();
    render(<ClaimConflictBanner hostname="hostA" itemDeepLink={LINK} indeterminate onRetry={onRetry} onOverride={jest.fn()} />);
    expect(screen.queryByTestId("claim-banner-action")).toBeNull();
    fireEvent.click(screen.getByTestId("claim-banner-retry"));
    expect(onRetry).toHaveBeenCalled();
  });
});
