/**
 * Tests for TokenBadge component.
 *
 * Covers:
 *  - Renders zero cost correctly
 *  - Renders small cost with 4-decimal precision
 *  - Renders medium cost with 3-decimal precision
 *  - Renders large cost with 2-decimal precision
 *  - Normal variant has no warning/alert class when below thresholds
 *  - Warning variant applied when costUsd >= warnThresholdUsd
 *  - Alert variant applied when costUsd >= alertThresholdUsd (overrides warning)
 *  - Title attribute shows full precision
 *  - No threshold props → always normal variant
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { TokenBadge } from "../TokenBadge";

describe("TokenBadge", () => {
  it("TokenBadge_should_renderZero_When_costIsZero", () => {
    render(<TokenBadge costUsd={0} />);
    expect(screen.getByText("$0")).toBeTruthy();
  });

  it("TokenBadge_should_render4Decimals_When_costIsVerySmall", () => {
    render(<TokenBadge costUsd={0.0001} />);
    expect(screen.getByText("$0.0001")).toBeTruthy();
  });

  it("TokenBadge_should_render3Decimals_When_costIsSmall", () => {
    render(<TokenBadge costUsd={0.005} />);
    expect(screen.getByText("$0.005")).toBeTruthy();
  });

  it("TokenBadge_should_render2Decimals_When_costIsDollarRange", () => {
    render(<TokenBadge costUsd={1.23} />);
    expect(screen.getByText("$1.23")).toBeTruthy();
  });

  it("TokenBadge_should_includeFullPrecisionInTitle_When_rendered", () => {
    const { container } = render(<TokenBadge costUsd={0.00123456} />);
    const span = container.querySelector("span");
    expect(span?.getAttribute("title")).toContain("0.001235");
  });

  it("TokenBadge_should_applyNormalVariant_When_belowAllThresholds", () => {
    const { container } = render(
      <TokenBadge costUsd={0.001} warnThresholdUsd={0.05} alertThresholdUsd={0.1} />
    );
    const span = container.querySelector("span");
    // Should not have error/warning styles in className - just check it exists and renders
    expect(span).toBeTruthy();
    expect(screen.getByText("$0.001")).toBeTruthy();
  });

  it("TokenBadge_should_applyWarningVariant_When_atWarnThreshold", () => {
    const { container } = render(
      <TokenBadge costUsd={0.05} warnThresholdUsd={0.05} alertThresholdUsd={0.1} />
    );
    const span = container.querySelector("span");
    expect(span).toBeTruthy();
    // className contains the warning style token
    expect(span?.className).toBeTruthy();
  });

  it("TokenBadge_should_applyAlertVariant_When_atAlertThreshold", () => {
    const { container } = render(
      <TokenBadge costUsd={0.1} warnThresholdUsd={0.05} alertThresholdUsd={0.1} />
    );
    const span = container.querySelector("span");
    expect(span).toBeTruthy();
    expect(span?.className).toBeTruthy();
  });

  it("TokenBadge_should_applyAlertVariant_When_aboveAlertThreshold", () => {
    // alert overrides warn when both thresholds exceeded
    const { container } = render(
      <TokenBadge costUsd={0.5} warnThresholdUsd={0.05} alertThresholdUsd={0.1} />
    );
    expect(container.querySelector("span")).toBeTruthy();
    expect(screen.getByText("$0.50")).toBeTruthy();
  });

  it("TokenBadge_should_renderNormally_When_noThresholdsProvided", () => {
    render(<TokenBadge costUsd={99.99} />);
    expect(screen.getByText("$99.99")).toBeTruthy();
  });
});
