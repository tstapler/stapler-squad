/**
 * Tests for GateBlockingSection (ADR-005): renders the item-detail "what's
 * blocking this transition" gate checklist.
 *
 * Covers:
 *  1. Frozen-snapshot notice renders for a stage that IS present in the live
 *     list but disabled — the exact bug adversarial review caught (a
 *     presence-only check would miss this).
 *  2. Notice does not render when the item's stage is live and enabled.
 *  3. N candidates render N GateChecklist sections.
 *  4. An RPC failure shows InlineError with a working Retry.
 */

import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GateBlockingSection } from "./GateBlockingSection";
import { useGateChecklist, useGateApproval } from "@/lib/hooks/useGateChecklist";
import { useBacklogStages, type BacklogStageInfo } from "@/lib/hooks/useBacklogStages";
import type { GateBlockingCandidate } from "@/lib/hooks/useGateChecklist";

jest.mock("@/lib/hooks/useGateChecklist", () => ({
  useGateChecklist: jest.fn(),
  useGateApproval: jest.fn(),
}));
jest.mock("@/lib/hooks/useBacklogStages", () => ({
  ...jest.requireActual("@/lib/hooks/useBacklogStages"),
  useBacklogStages: jest.fn(),
}));

const mockUseGateChecklist = useGateChecklist as jest.Mock;
const mockUseGateApproval = useGateApproval as jest.Mock;
const mockUseBacklogStages = useBacklogStages as jest.Mock;

// Same benign vanilla-extract jest-mock className warning noted in
// PipelineModeForm.test.tsx / pipeline-modes/page.test.tsx.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

function makeStage(overrides: Partial<BacklogStageInfo> = {}): BacklogStageInfo {
  return { slug: "review", name: "Review", enabled: true, ...overrides };
}

function makeCandidate(toStatus: string): GateBlockingCandidate {
  return {
    toStatus,
    gates: [{ gateId: `gate-${toStatus}`, kind: "structural", satisfied: false, description: "blocked" }],
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockUseGateApproval.mockReturnValue({ recordApproval: jest.fn().mockResolvedValue(undefined) });
  mockUseGateChecklist.mockReturnValue({
    candidates: [],
    isLoading: false,
    error: null,
    refetch: jest.fn(),
  });
  mockUseBacklogStages.mockReturnValue({ stages: [makeStage()], isLoading: false, error: null });
});

describe("GateBlockingSection — frozen-snapshot notice", () => {
  it("renders for a stage that is present but disabled (not just absent)", () => {
    mockUseGateChecklist.mockReturnValue({
      candidates: [makeCandidate("design_review")],
      isLoading: false,
      error: null,
      refetch: jest.fn(),
    });
    mockUseBacklogStages.mockReturnValue({
      stages: [makeStage({ slug: "review", enabled: false })],
      isLoading: false,
      error: null,
    });

    render(<GateBlockingSection item={{ id: "item-1", status: "review" }} />);

    expect(screen.getByTestId("gate-blocking-frozen-snapshot-notice")).toBeInTheDocument();
  });

  it("does not render when the item's stage is live and enabled", () => {
    mockUseGateChecklist.mockReturnValue({
      candidates: [makeCandidate("design_review")],
      isLoading: false,
      error: null,
      refetch: jest.fn(),
    });
    mockUseBacklogStages.mockReturnValue({
      stages: [makeStage({ slug: "review", enabled: true })],
      isLoading: false,
      error: null,
    });

    render(<GateBlockingSection item={{ id: "item-1", status: "review" }} />);

    expect(screen.queryByTestId("gate-blocking-frozen-snapshot-notice")).not.toBeInTheDocument();
  });
});

describe("GateBlockingSection — N candidates", () => {
  it("renders one GateChecklist section per gated candidate", () => {
    mockUseGateChecklist.mockReturnValue({
      candidates: [makeCandidate("queued"), makeCandidate("done")],
      isLoading: false,
      error: null,
      refetch: jest.fn(),
    });

    render(<GateBlockingSection item={{ id: "item-1", status: "review" }} />);

    expect(screen.getByText(/What.s blocking Review → Queued\?/)).toBeInTheDocument();
    expect(screen.getByText(/What.s blocking Review → Done\?/)).toBeInTheDocument();
  });

  it("renders nothing when there are no gated candidates", () => {
    const { container } = render(<GateBlockingSection item={{ id: "item-1", status: "review" }} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("GateBlockingSection — RPC failure", () => {
  it("shows InlineError with a working Retry that calls refetch", async () => {
    const refetch = jest.fn();
    mockUseGateChecklist.mockReturnValue({
      candidates: [],
      isLoading: false,
      error: "network error",
      refetch,
    });

    const user = userEvent.setup();
    render(<GateBlockingSection item={{ id: "item-1", status: "review" }} />);

    expect(screen.getByText(/network error/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /retry/i }));

    await waitFor(() => expect(refetch).toHaveBeenCalledTimes(1));
  });
});
