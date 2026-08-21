/**
 * Story 3.2.3: FeaturesPage renders an optional second status-detail line
 * under a flag's description, driven by FeatureFlagMeta.statusDetail
 * (quota-aware-backlog-gating). No layout shift when statusDetail is empty.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import FeaturesPage from "./page";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import type { FeatureFlagMeta } from "@/lib/contexts/FeatureFlagsContext";

jest.mock("@/lib/analytics", () => ({
  usePageView: () => {},
}));

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: jest.fn(),
}));

const mockUseFeatureFlags = useFeatureFlags as jest.MockedFunction<typeof useFeatureFlags>;

function makeFlag(overrides: Partial<FeatureFlagMeta> & Pick<FeatureFlagMeta, "name">): FeatureFlagMeta {
  return {
    enabled: true,
    description: "",
    statusDetail: "",
    ...overrides,
  };
}

function mockFlags(flagList: FeatureFlagMeta[]) {
  mockUseFeatureFlags.mockReturnValue({
    flags: Object.fromEntries(flagList.map((f) => [f.name, f.enabled])),
    flagList,
    isLoading: false,
    error: null,
    setFlag: jest.fn(),
  });
}

// Same benign vanilla-extract jest-mock className warning as
// pipeline-modes/page.test.tsx — see that file's comment for details.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

describe("FeaturesPage", () => {
  it("FeaturesPage_should_RenderSecondLine_When_StatusDetailNonEmpty", () => {
    mockFlags([
      makeFlag({
        name: "backlog",
        enabled: false,
        description: "Backlog management with external sync sources and AI-driven triage",
        statusDetail: "Paused: session-quota headroom below threshold (15% remaining; threshold 20%).",
      }),
    ]);

    render(<FeaturesPage />);

    expect(
      screen.getByText("Paused: session-quota headroom below threshold (15% remaining; threshold 20%).")
    ).toBeInTheDocument();
  });

  it("FeaturesPage_should_RenderNoExtraElement_When_StatusDetailEmpty", () => {
    mockFlags([
      makeFlag({
        name: "backlog",
        enabled: true,
        description: "Backlog management with external sync sources and AI-driven triage",
        statusDetail: "",
      }),
    ]);

    const { container } = render(<FeaturesPage />);

    // Exactly one description-styled line (the flag description) — no second
    // empty line/paragraph rendered for a "" statusDetail.
    const descriptionLine = screen.getByText(
      "Backlog management with external sync sources and AI-driven triage"
    );
    expect(descriptionLine.parentElement?.children.length).toBe(2); // flagName + description only
    expect(container.querySelectorAll("div").length).toBeGreaterThan(0);
  });
});
