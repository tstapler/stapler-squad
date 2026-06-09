/**
 * Tests for ProgramDetailPanel — three-state coverage column.
 *
 * Covers:
 *  - TC-FE-01: subcommand with hasRuleCoverage=true, escalate=0 → shows "coverage-covered" badge
 *  - TC-FE-02: subcommand with hasRuleCoverage=true, escalate=5 → shows "coverage-partial" badge
 *  - TC-FE-03: subcommand with hasRuleCoverage=false → shows "coverage-gap" badge AND "Add rule" link
 *  - TC-FE-04: loading state → shows loading indicator
 *  - TC-FE-05: empty subcommands → shows empty message
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { ProgramDetailPanel } from "./ProgramDetailPanel";

// ---------------------------------------------------------------------------
// Mock: useProgramAnalytics
// ---------------------------------------------------------------------------

interface MockSubcommandRow {
  subcommand: string;
  total: number;
  autoAllow: number;
  autoDeny: number;
  escalate: number;
  manualAllow: number;
  manualDeny: number;
  hasRuleCoverage: boolean;
}

interface MockData {
  program: string;
  category: string;
  subcommands: MockSubcommandRow[];
  recentExamples: string[];
  trend: unknown[];
}

let mockData: MockData | null = null;
let mockIsLoading = false;
let mockError: Error | null = null;

jest.mock("@/lib/hooks/useProgramAnalytics", () => ({
  useProgramAnalytics: () => ({
    data: mockData,
    isLoading: mockIsLoading,
    error: mockError,
    refresh: jest.fn(),
  }),
}));

// ---------------------------------------------------------------------------
// Mock: CSS modules (vanilla-extract)
// ---------------------------------------------------------------------------

jest.mock("./ProgramDetailPanel.css", () =>
  new Proxy(
    {},
    {
      get: (_target, key) => {
        if (typeof key === "string") return key;
        return "";
      },
    }
  )
);

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSubcommandRow(overrides: Partial<MockSubcommandRow> = {}): MockSubcommandRow {
  return {
    subcommand: "push",
    total: 10,
    autoAllow: 8,
    autoDeny: 0,
    escalate: 0,
    manualAllow: 0,
    manualDeny: 0,
    hasRuleCoverage: true,
    ...overrides,
  };
}

function makeData(rows: MockSubcommandRow[]): MockData {
  return {
    program: "git",
    category: "vcs",
    subcommands: rows,
    recentExamples: [],
    trend: [],
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ProgramDetailPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockData = null;
    mockIsLoading = false;
    mockError = null;
  });

  it("ProgramDetailPanel_should_showCoveredBadge_When_hasRuleCoverageAndNoEscalate", () => {
    // TC-FE-01
    mockData = makeData([makeSubcommandRow({ hasRuleCoverage: true, escalate: 0 })]);

    render(<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />);

    expect(screen.getByTestId("coverage-covered")).toBeInTheDocument();
    expect(screen.queryByTestId("coverage-partial")).not.toBeInTheDocument();
    expect(screen.queryByTestId("coverage-gap")).not.toBeInTheDocument();
  });

  it("ProgramDetailPanel_should_showPartialBadge_When_hasRuleCoverageAndEscalateGt0", () => {
    // TC-FE-02
    mockData = makeData([makeSubcommandRow({ hasRuleCoverage: true, escalate: 5 })]);

    render(<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />);

    expect(screen.getByTestId("coverage-partial")).toBeInTheDocument();
    expect(screen.queryByTestId("coverage-covered")).not.toBeInTheDocument();
    expect(screen.queryByTestId("coverage-gap")).not.toBeInTheDocument();
  });

  it("ProgramDetailPanel_should_showGapBadge_When_noRuleCoverage", () => {
    // TC-FE-03
    mockData = makeData([makeSubcommandRow({ hasRuleCoverage: false, escalate: 1 })]);

    render(<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />);

    expect(screen.getByTestId("coverage-gap")).toBeInTheDocument();
    expect(screen.queryByTestId("coverage-covered")).not.toBeInTheDocument();
    expect(screen.queryByTestId("coverage-partial")).not.toBeInTheDocument();
    // "Add rule" link must also appear when gap
    expect(screen.getByText("Add rule →")).toBeInTheDocument();
  });

  it("ProgramDetailPanel_should_showAddRuleLink_When_gap", () => {
    // TC-FE-03 (Add rule link assertion)
    mockData = makeData([makeSubcommandRow({ hasRuleCoverage: false })]);

    render(<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />);

    expect(screen.getByText("Add rule →")).toBeInTheDocument();
  });

  it("ProgramDetailPanel_should_notShowAddRuleLink_When_covered", () => {
    // TC-FE-03 (negative) / TC-FE-01
    mockData = makeData([makeSubcommandRow({ hasRuleCoverage: true, escalate: 0 })]);

    render(<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />);

    expect(screen.queryByText("Add rule →")).not.toBeInTheDocument();
  });

  it("ProgramDetailPanel_should_showLoadingIndicator_When_loading", () => {
    // TC-FE-04
    mockIsLoading = true;
    mockData = null;

    render(<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />);

    expect(screen.getByText(/Loading/i)).toBeInTheDocument();
  });

  it("ProgramDetailPanel_should_showEmptyMessage_When_noSubcommands", () => {
    // TC-FE-05
    mockData = makeData([]);

    render(<ProgramDetailPanel program="git" windowDays={7} onClose={() => {}} />);

    expect(screen.getByText(/No subcommand data/i)).toBeInTheDocument();
  });
});
