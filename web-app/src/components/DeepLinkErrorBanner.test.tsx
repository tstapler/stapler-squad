/**
 * Story 5.1 component tests (project_plans/backlog-deep-linking) for
 * DeepLinkErrorBanner. validation.md maps this file to two required tests:
 *
 *  1. DeepLinkErrorBanner_should_RenderDistinctHeadlineAndBody_When_EachOfFiveFailureReasonsGiven
 *     — table test over the 5 UX-design cases from ux.md Surfaces 4-8. Surface
 *     4 covers two wire-level `reason` values ("deleted" and "archived") under
 *     one design case with a conditional headline (ux.md row 4) — both are
 *     exercised here and asserted distinct from each other per AC4, so this
 *     table has 6 `reason` entries grouped into 5 cases, matching "5 distinct
 *     assertions, not fewer" (ux.md AC1 cross-check).
 *  2. DeepLinkErrorBanner_should_NeverRenderRawParseErrorOrURL_When_MalformedOrVersionMismatchReasonGiven
 *     — the malformed/version-mismatch banners must never leak a raw parse
 *     error, stack trace, or the verbatim malformed URL (ux.md AC5).
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { DeepLinkErrorBanner, type DeepLinkFailureReason } from "./DeepLinkErrorBanner";

describe("DeepLinkErrorBanner", () => {
  it("DeepLinkErrorBanner_should_RenderDistinctHeadlineAndBody_When_EachOfFiveFailureReasonsGiven", () => {
    const cases: { case: string; reason: DeepLinkFailureReason }[] = [
      { case: "deleted-or-archived (deleted)", reason: "deleted" },
      { case: "deleted-or-archived (archived)", reason: "archived" },
      { case: "unreachable", reason: "unreachable" },
      { case: "not-registered", reason: "not-registered" },
      { case: "malformed", reason: "malformed" },
      { case: "version-mismatch", reason: "version-mismatch" },
    ];

    const seen = new Set<string>();
    for (const { reason } of cases) {
      const { unmount } = render(
        <DeepLinkErrorBanner reason={reason} hostname="otherhost" />
      );
      const banner = screen.getByTestId("deep-link-error-banner");
      const headlineEl = banner.querySelector("div");
      const bodyEl = banner.querySelector("p");
      expect(headlineEl).not.toBeNull();
      expect(bodyEl).not.toBeNull();
      if (!headlineEl || !bodyEl) {
        throw new Error("expected headline and body elements to be present");
      }

      const pair = `${headlineEl.textContent}|||${bodyEl.textContent}`;
      expect(seen.has(pair)).toBe(false);
      seen.add(pair);

      unmount();
    }

    // Every one of the 6 wire-level reason values (5 design cases, with
    // "deleted"/"archived" sharing case 4) produced a unique headline+body
    // pair — no two cases collide (ux.md AC1/AC4).
    expect(seen.size).toBe(cases.length);
  });

  it("deleted and archived render distinguishable headlines (ux.md AC4)", () => {
    const { unmount: unmountDeleted } = render(
      <DeepLinkErrorBanner reason="deleted" />
    );
    const deletedHeadline = screen.getByText(/no longer exists/i);
    expect(deletedHeadline).toBeInTheDocument();
    unmountDeleted();

    render(<DeepLinkErrorBanner reason="archived" />);
    expect(screen.getByText(/has been archived/i)).toBeInTheDocument();
    expect(screen.queryByText(/no longer exists/i)).not.toBeInTheDocument();
  });

  it("names the literal hostname for the unreachable and not-registered cases (ux.md AC3)", () => {
    const { unmount } = render(
      <DeepLinkErrorBanner reason="unreachable" hostname="otherhost" />
    );
    expect(screen.getAllByText(/otherhost/).length).toBeGreaterThan(0);
    expect(screen.queryByText(/an instance/i)).not.toBeInTheDocument();
    unmount();

    render(<DeepLinkErrorBanner reason="not-registered" hostname="thathost" />);
    expect(screen.getAllByText(/thathost/).length).toBeGreaterThan(0);
  });

  it("DeepLinkErrorBanner_should_NeverRenderRawParseErrorOrURL_When_MalformedOrVersionMismatchReasonGiven", () => {
    const rawParseError = "unexpected token at position 4: ssq://broken";
    const rawURL = "ssq://myhost/backlog/v1/not-a-valid-id???";

    const { unmount } = render(<DeepLinkErrorBanner reason="malformed" />);
    const malformedBanner = screen.getByTestId("deep-link-error-banner");
    expect(malformedBanner.textContent).not.toContain(rawParseError);
    expect(malformedBanner.textContent).not.toContain(rawURL);
    expect(malformedBanner.textContent).not.toMatch(/at\s+\S+\.(go|ts|tsx):\d+/);
    unmount();

    render(<DeepLinkErrorBanner reason="version-mismatch" />);
    const versionBanner = screen.getByTestId("deep-link-error-banner");
    expect(versionBanner.textContent).not.toContain(rawParseError);
    expect(versionBanner.textContent).not.toContain(rawURL);
    expect(versionBanner.textContent).not.toMatch(/at\s+\S+\.(go|ts|tsx):\d+/);
  });

  it("every failure reason offers an actionable control that isn't browser Back (ux.md AC2)", () => {
    const cases: DeepLinkFailureReason[] = [
      "deleted",
      "archived",
      "unreachable",
      "not-registered",
      "malformed",
      "version-mismatch",
    ];

    for (const reason of cases) {
      const { unmount } = render(
        <DeepLinkErrorBanner
          reason={reason}
          hostname="otherhost"
          onGoToBoard={jest.fn()}
          onRetry={jest.fn()}
        />
      );
      expect(screen.getAllByRole("button").length).toBeGreaterThan(0);
      unmount();
    }
  });
});
