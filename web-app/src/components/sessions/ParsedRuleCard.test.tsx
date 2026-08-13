/**
 * Tests for ParsedRuleCard component.
 * Covers UT-FE-19 through UT-FE-22.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { ParsedRuleCard } from "./ParsedRuleCard";
import { AutoDecision } from "@/gen/session/v1/types_pb";
import type { ParsedRuleResult } from "@/gen/session/v1/session_pb";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeResult(
  name: string,
  valid: boolean,
  errors: string[] = [],
  decision: AutoDecision = AutoDecision.ALLOW,
): ParsedRuleResult {
  return {
    originalName: name,
    valid,
    errors,
    rule: valid
      ? {
          id: `rule-${name}`,
          name,
          toolName: "Bash",
          toolPattern: "",
          commandPattern: "",
          filePattern: "",
          criteriaPrograms: [],
          criteriaSubcommands: [],
          decision,
          riskLevel: "low",
          reason: "",
          alternative: "",
          priority: 10,
          enabled: true,
          source: "user",
        }
      : undefined,
  } as unknown as ParsedRuleResult;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ParsedRuleCard", () => {
  it("UT-FE-19: ParsedRuleCard_renders_valid_rule", () => {
    const result = makeResult("Allow git log", true, []);
    render(<ParsedRuleCard result={result} status="valid" />);

    expect(screen.getByTestId("parsed-rule-card-valid")).toBeInTheDocument();
    expect(screen.getByText("Allow git log")).toBeInTheDocument();
    expect(screen.getByText("Allow")).toBeInTheDocument();
    expect(screen.queryByTestId("error-list")).not.toBeInTheDocument();
    expect(screen.queryByTestId("overwrite-badge")).not.toBeInTheDocument();
    expect(screen.queryByTestId("skip-badge")).not.toBeInTheDocument();
  });

  it("UT-FE-20: ParsedRuleCard_renders_error_rule_with_messages", () => {
    const result = makeResult("Bad Rule", false, ["name is required", "invalid decision"]);
    render(<ParsedRuleCard result={result} status="error" />);

    expect(screen.getByTestId("parsed-rule-card-error")).toBeInTheDocument();
    expect(screen.getByText("Bad Rule")).toBeInTheDocument();

    const errorList = screen.getByTestId("error-list");
    expect(errorList).toBeInTheDocument();
    expect(errorList).toHaveTextContent("name is required");
    expect(errorList).toHaveTextContent("invalid decision");
  });

  it("UT-FE-21: ParsedRuleCard_renders_overwrite_badge", () => {
    const result = makeResult("Allow git log", true, []);
    render(<ParsedRuleCard result={result} status="overwrite" />);

    expect(screen.getByTestId("parsed-rule-card-overwrite")).toBeInTheDocument();
    expect(screen.getByTestId("overwrite-badge")).toBeInTheDocument();
    expect(screen.getByTestId("overwrite-badge")).toHaveTextContent("will overwrite");
    expect(screen.queryByTestId("skip-badge")).not.toBeInTheDocument();
  });

  it("UT-FE-22: ParsedRuleCard_renders_skip_badge", () => {
    const result = makeResult("Allow git log", true, []);
    render(<ParsedRuleCard result={result} status="skip" />);

    expect(screen.getByTestId("parsed-rule-card-skip")).toBeInTheDocument();
    expect(screen.getByTestId("skip-badge")).toBeInTheDocument();
    expect(screen.getByTestId("skip-badge")).toHaveTextContent("will skip");
    expect(screen.queryByTestId("overwrite-badge")).not.toBeInTheDocument();
  });
});
