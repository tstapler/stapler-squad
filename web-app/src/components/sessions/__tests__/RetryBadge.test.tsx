import React from "react";
import { render, screen } from "@testing-library/react";
import { RetryBadge } from "../RetryBadge";

jest.mock("../RetryBadge.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);

describe("RetryBadge", () => {
  it("RetryBadge_should_NotRender_When_RetryAttemptIsZero", () => {
    const { container } = render(<RetryBadge retryAttempt={0} retryMaxAttempts={3} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("RetryBadge_should_ExposeAriaLabelWithAttemptFraction_When_Rendered", () => {
    render(<RetryBadge retryAttempt={2} retryMaxAttempts={3} />);
    const badge = screen.getByRole("img");
    expect(badge).toHaveAttribute("aria-label", "Retry attempt 2 of 3");
    // The emoji icon itself is aria-hidden — only the badge's own label is announced.
    expect(badge.querySelector('[aria-hidden="true"]')).toBeInTheDocument();
  });

  it("RetryBadge_should_ApplyWarningTokens_When_AttemptEqualsMaxAttempts", () => {
    render(<RetryBadge retryAttempt={3} retryMaxAttempts={3} />);
    const badge = screen.getByRole("img");
    expect(badge.className).toContain("warning");
  });

  it("RetryBadge_should_ApplyNeutralTokens_When_AttemptBelowMaxAttempts", () => {
    render(<RetryBadge retryAttempt={1} retryMaxAttempts={3} />);
    const badge = screen.getByRole("img");
    expect(badge.className).toContain("neutral");
  });

  it("renders compact N/max text", () => {
    render(<RetryBadge retryAttempt={2} retryMaxAttempts={3} compact />);
    expect(screen.getByRole("img").textContent).toContain("2/3");
  });
});
