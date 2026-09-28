import React from "react";
import { render, screen } from "@testing-library/react";
import { TokenBreakdownBar } from "./TokenBreakdownBar";

const baseProps = {
  input: 100n,
  output: 50n,
  cacheCreation: 25n,
  cacheRead: 25n,
};

describe("TokenBreakdownBar", () => {
  it("TokenBreakdownBar_should_showFormattedCostPerSegment_when_costPropsProvided", () => {
    render(
      <TokenBreakdownBar
        {...baseProps}
        inputCostUsd={0.98}
        outputCostUsd={1.5}
        cacheCreationCostUsd={0.02}
        cacheReadCostUsd={0.075}
      />
    );

    expect(screen.getByTestId("token-segment-cost-input")).toHaveTextContent("Input: 100 · $0.980");
    expect(screen.getByTestId("token-segment-cost-output")).toHaveTextContent("Output: 50 · $1.50");
    expect(screen.getByTestId("token-segment-cost-cacheCreation")).toHaveTextContent("Cache write: 25 · $0.020");
    expect(screen.getByTestId("token-segment-cost-cacheRead")).toHaveTextContent("Cache read: 25 · $0.075");
  });

  it("TokenBreakdownBar_should_omitCost_when_costPropsUndefined", () => {
    render(<TokenBreakdownBar {...baseProps} />);

    for (const key of ["input", "output", "cacheCreation", "cacheRead"]) {
      const el = screen.getByTestId(`token-segment-cost-${key}`);
      expect(el.textContent).not.toContain("$");
    }
  });
});
