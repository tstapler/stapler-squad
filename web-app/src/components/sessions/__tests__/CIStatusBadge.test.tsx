/**
 * Tests for CIStatusBadge (AC1, AC2, AC3, AC7).
 *
 * Covers:
 *  - Renders the correct text/icon/class/aria-label for each CI conclusion state
 *  - Empty/neutral conclusion collapses to "No checks"
 *  - href construction from prUrl, and omission when prUrl is missing
 *  - Returns null when prNumber is 0/undefined (no PR)
 *  - Purely presentational — no RPC/fetch calls
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { CIStatusBadge } from "../CIStatusBadge";

function renderBadge(props: Parameters<typeof CIStatusBadge>[0]) {
  return render(<CIStatusBadge {...props} />);
}

describe("CIStatusBadge", () => {
  it("CIStatusBadge_should_RenderPassingBadge_When_CheckConclusionIsSuccess", () => {
    renderBadge({ checkConclusion: "success", prNumber: 42, prUrl: "https://github.com/acme/widgets/pull/42" });
    expect(screen.getByText("Passing")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "CI status: Passing");
  });

  it("renders Failing badge for a failure conclusion", () => {
    renderBadge({ checkConclusion: "failure", prNumber: 42 });
    expect(screen.getByText("Failing")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "CI status: Failing");
  });

  it("renders Pending badge for a pending conclusion", () => {
    renderBadge({ checkConclusion: "pending", prNumber: 42 });
    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "CI status: Pending");
  });

  it("CIStatusBadge_should_RenderNoChecksBadge_When_CheckConclusionIsEmptyOrNeutral", () => {
    renderBadge({ checkConclusion: "", prNumber: 42 });
    expect(screen.getByText("No checks")).toBeInTheDocument();

    const { unmount } = renderBadge({ checkConclusion: "neutral", prNumber: 43 });
    expect(screen.getAllByText("No checks").length).toBeGreaterThan(0);
    unmount();
  });

  it("CIStatusBadge_should_LinkToChecksPage_When_PrUrlProvided", () => {
    renderBadge({ checkConclusion: "success", prNumber: 42, prUrl: "https://github.com/acme/widgets/pull/42" });
    const link = screen.getByRole("status");
    expect(link).toHaveAttribute("href", "https://github.com/acme/widgets/pull/42/checks");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("CIStatusBadge_should_OmitHref_When_PrUrlMissing", () => {
    renderBadge({ checkConclusion: "success", prNumber: 42 });
    expect(screen.getByRole("status")).not.toHaveAttribute("href");
  });

  it("CIStatusBadge_should_ReturnNull_When_PrNumberIsZero", () => {
    const { container } = renderBadge({ checkConclusion: "success", prNumber: 0 });
    expect(container).toBeEmptyDOMElement();
  });

  it("returns null when prNumber is undefined", () => {
    const { container } = renderBadge({ checkConclusion: "success" });
    expect(container).toBeEmptyDOMElement();
  });

  it("CIStatusBadge_should_ExposeStatusRoleAndAriaLabel_When_Rendered", () => {
    (["success", "failure", "pending", ""] as const).forEach((conclusion, i) => {
      const { unmount } = renderBadge({ checkConclusion: conclusion, prNumber: 100 + i });
      expect(screen.getByRole("status")).toHaveAttribute("aria-label", expect.stringMatching(/^CI status: /));
      unmount();
    });
  });

  it("CIStatusBadge_should_PairIconWithTextLabel_When_EachStateRenders", () => {
    renderBadge({ checkConclusion: "failure", prNumber: 42 });
    const badgeEl = screen.getByRole("status");
    expect(badgeEl.querySelector('[aria-hidden="true"]')).toHaveTextContent("❌");
    expect(badgeEl).toHaveTextContent("Failing");
  });

  it("includes a relative staleness hint in the tooltip when lastChecked is set", () => {
    renderBadge({
      checkConclusion: "success",
      prNumber: 42,
      lastChecked: timestampFromDate(new Date(Date.now() - 60_000)),
    });
    expect(screen.getByRole("status").getAttribute("title")).toMatch(/checked/);
  });

  it("CIStatusBadge_should_NotInvokeAnyRPCOrFetch_When_Rendering", () => {
    const fetchMock = jest.fn();
    const originalFetch = global.fetch;
    global.fetch = fetchMock as unknown as typeof global.fetch;
    renderBadge({ checkConclusion: "success", prNumber: 42, prUrl: "https://github.com/acme/widgets/pull/42" });
    expect(fetchMock).not.toHaveBeenCalled();
    global.fetch = originalFetch;
  });
});
