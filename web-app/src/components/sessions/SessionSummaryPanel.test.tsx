import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { SessionSummaryPanel } from "./SessionSummaryPanel";
import { SessionSummaryStatus } from "@/gen/session/v1/types_pb";
import type { SessionSummaryProto } from "@/gen/session/v1/session_summary_pb";

// ---------------------------------------------------------------------------
// Mock: useSessionSummary
// ---------------------------------------------------------------------------

const mockRegenerate = jest.fn();
const mockCopy = jest.fn();
const mockUseSessionSummary = jest.fn();

jest.mock("@/lib/hooks/useSessionSummary", () => ({
  useSessionSummary: (sessionId: string) => mockUseSessionSummary(sessionId),
}));

function makeSummary(overrides: Partial<Record<string, unknown>> = {}): SessionSummaryProto {
  return {
    sessionId: "session-1",
    sessionTitle: "fix-login-redirect",
    status: SessionSummaryStatus.READY,
    narrative: "Fixed the login redirect loop.",
    narrativeFallbackUsed: false,
    diff: { filesChanged: 3, added: 42, removed: 7 },
    decisions: {
      autoApproved: 5,
      manuallyApproved: 1,
      denied: 0,
      reviewQueueResolved: 1,
      stillOpen: 0,
    },
    timeline: {
      startedAt: { seconds: BigInt(1000), nanos: 0 },
      stoppedAt: { seconds: BigInt(2000), nanos: 0 },
      durationMs: BigInt(1000000),
    },
    cost: { totalTokens: BigInt(128000), estimatedCostUsd: 1.92, dataUnavailable: false },
    markdown: "## What Was Done\nFixed the login redirect loop.\n",
    errorMessage: "",
    errorStage: "",
    generatedAt: { seconds: BigInt(1700000000), nanos: 0 },
    ...overrides,
  } as unknown as SessionSummaryProto;
}

function mockHookReturn(overrides: Partial<Record<string, unknown>> = {}) {
  mockUseSessionSummary.mockReturnValue({
    data: null,
    loading: false,
    error: null,
    neverResolved: false,
    regenerate: mockRegenerate,
    copy: mockCopy,
    ...overrides,
  });
}

function getLiveRegion(): HTMLElement {
  return screen.getByRole("status");
}

describe("SessionSummaryPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe("SessionSummaryPanel_should_renderExactly17SkeletonBlocks_When_Generating", () => {
    it("renders 17 summary-skeleton-block elements", () => {
      mockHookReturn({
        data: { status: SessionSummaryStatus.GENERATING } as unknown as SessionSummaryProto,
      });

      render(<SessionSummaryPanel sessionId="session-1" />);

      expect(screen.getAllByTestId("summary-skeleton-block")).toHaveLength(17);
      expect(screen.getByRole("region", { name: "Session summary" })).toHaveAttribute(
        "aria-busy",
        "true",
      );
    });

    it("renders 17 blocks when data is still null (row not yet created)", () => {
      mockHookReturn({ data: null });

      render(<SessionSummaryPanel sessionId="session-1" />);

      expect(screen.getAllByTestId("summary-skeleton-block")).toHaveLength(17);
    });
  });

  describe("SessionSummaryPanel_should_renderStaticSkeleton_When_ReducedMotionPreferred", () => {
    const originalMatchMedia = window.matchMedia;

    afterEach(() => {
      window.matchMedia = originalMatchMedia;
    });

    function mockReducedMotion(matches: boolean) {
      window.matchMedia = jest.fn().mockImplementation((query: string) => ({
        matches,
        media: query,
        onchange: null,
        addListener: jest.fn(),
        removeListener: jest.fn(),
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
        dispatchEvent: jest.fn(),
      }));
    }

    it("uses the static (no-shimmer) class when prefers-reduced-motion: reduce is mocked", () => {
      mockReducedMotion(true);
      mockHookReturn({
        data: { status: SessionSummaryStatus.GENERATING } as unknown as SessionSummaryProto,
      });

      render(<SessionSummaryPanel sessionId="session-1" />);

      const blocks = screen.getAllByTestId("summary-skeleton-block");
      expect(blocks.length).toBeGreaterThan(0);
      for (const block of blocks) {
        expect(block.className).not.toMatch(/skeletonBlock(?!Reduced)/);
        expect(block.className).toMatch(/skeletonBlockReducedMotion/);
      }
    });

    it("uses the shimmer class when prefers-reduced-motion is not set", () => {
      mockReducedMotion(false);
      mockHookReturn({
        data: { status: SessionSummaryStatus.GENERATING } as unknown as SessionSummaryProto,
      });

      render(<SessionSummaryPanel sessionId="session-1" />);

      const blocks = screen.getAllByTestId("summary-skeleton-block");
      expect(blocks.length).toBeGreaterThan(0);
      for (const block of blocks) {
        expect(block.className).toMatch(/skeletonBlock(?!Reduced)/);
      }
    });
  });

  describe("SessionSummaryPanel_should_renderMarkdownAndDecisions_When_Ready", () => {
    it("renders the markdown body and the decisions-at-a-glance card", () => {
      mockHookReturn({ data: makeSummary() });

      render(<SessionSummaryPanel sessionId="session-1" />);

      expect(screen.getByTestId("summary-markdown-body")).toBeInTheDocument();
      expect(screen.getByText(/Fixed the login redirect loop/)).toBeInTheDocument();
      expect(screen.getByTestId("summary-decisions-glance")).toBeInTheDocument();
      expect(screen.getByText(/5 auto-approved/)).toBeInTheDocument();
      expect(screen.getByLabelText("Copy summary as Markdown")).toBeInTheDocument();
    });
  });

  describe("SessionSummaryPanel_should_renderBareErrorWithEnabledRegenerate_When_ErrorAndNoMarkdown", () => {
    it("renders the bare error card, not the READY document", () => {
      mockHookReturn({
        data: makeSummary({
          status: SessionSummaryStatus.ERROR,
          markdown: "",
          errorStage: "decisions",
          errorMessage: "boom",
        }),
      });

      render(<SessionSummaryPanel sessionId="session-1" />);

      expect(screen.getByTestId("summary-error-card")).toBeInTheDocument();
      expect(screen.queryByTestId("summary-markdown-body")).not.toBeInTheDocument();
      expect(screen.getByText("Failed while computing approval decisions.")).toBeInTheDocument();

      const regenerateButton = screen.getByRole("button", { name: /Regenerate/ });
      expect(regenerateButton).toBeEnabled();
    });
  });

  describe("SessionSummaryPanel_should_renderStaleDocumentBanner_When_ErrorWithPriorMarkdown", () => {
    it("renders the READY document with a banner instead of the bare error card", () => {
      mockHookReturn({
        data: makeSummary({
          status: SessionSummaryStatus.ERROR,
          markdown: "## What Was Done\nFixed the login redirect loop.\n",
          errorStage: "persist",
          errorMessage: "db write failed",
        }),
      });

      render(<SessionSummaryPanel sessionId="session-1" />);

      expect(screen.queryByTestId("summary-error-card")).not.toBeInTheDocument();
      expect(screen.getByTestId("summary-stale-banner")).toBeInTheDocument();
      expect(screen.getByTestId("summary-markdown-body")).toBeInTheDocument();
      expect(screen.getByTestId("summary-decisions-glance")).toBeInTheDocument();

      const tryAgainButton = screen.getByTestId("summary-try-again");
      expect(tryAgainButton).toHaveTextContent("Try again");

      // No duplicate toolbar Regenerate button.
      const regenerateButtons = screen.queryAllByRole("button", { name: /Regenerate|Try again/ });
      expect(regenerateButtons).toHaveLength(1);
    });
  });

  describe("SessionSummaryPanel_should_renderTerminalEmptyState_When_NeverResolved", () => {
    it("renders the never-resolved empty state", () => {
      mockHookReturn({ data: null, neverResolved: true });

      render(<SessionSummaryPanel sessionId="session-1" />);

      const emptyState = screen.getByTestId("summary-empty-state");
      expect(emptyState).toBeInTheDocument();
      expect(within(emptyState).getByText(/No summary available for this session/)).toBeInTheDocument();
      expect(screen.queryByTestId("summary-skeleton-block")).not.toBeInTheDocument();
    });
  });

  describe("SessionSummaryPanel_should_disableRegenerateButtonWithInFlightLabel_When_RegenerateIsPending", () => {
    it("disables the button and shows 'Regenerating…' while the call is in flight", async () => {
      let resolveRegenerate: () => void = () => {};
      mockRegenerate.mockImplementation(
        () =>
          new Promise<void>((resolve) => {
            resolveRegenerate = resolve;
          }),
      );
      mockHookReturn({
        data: makeSummary({ status: SessionSummaryStatus.ERROR, markdown: "", errorStage: "decisions" }),
      });

      render(<SessionSummaryPanel sessionId="session-1" />);

      const regenerateButton = screen.getByRole("button", { name: "↻ Regenerate" });
      fireEvent.click(regenerateButton);

      await waitFor(() => {
        expect(screen.getByRole("button", { name: /Regenerating…/ })).toBeDisabled();
      });

      resolveRegenerate();
      await waitFor(() => expect(mockRegenerate).toHaveBeenCalledTimes(1));
    });
  });

  describe("SessionSummaryPanel_should_announceCopyOutcome_When_CopyButtonClicked", () => {
    it("announces success via the aria-live region on successful copy", async () => {
      mockCopy.mockResolvedValue(true);
      mockHookReturn({ data: makeSummary() });

      render(<SessionSummaryPanel sessionId="session-1" />);

      fireEvent.click(screen.getByLabelText("Copy summary as Markdown"));

      await waitFor(() => {
        expect(getLiveRegion().textContent).toBe("Summary copied to clipboard.");
      });
      expect(mockCopy).toHaveBeenCalledTimes(1);
    });

    it("announces failure via the aria-live region (not a silent no-op) on copy failure", async () => {
      mockCopy.mockResolvedValue(false);
      mockHookReturn({ data: makeSummary() });

      render(<SessionSummaryPanel sessionId="session-1" />);

      fireEvent.click(screen.getByLabelText("Copy summary as Markdown"));

      await waitFor(() => {
        expect(getLiveRegion().textContent).toBe(
          "Copy failed. Select the text and copy manually.",
        );
      });
      expect(screen.getByTestId("summary-copy-failure")).toHaveTextContent(
        "Copy failed — select the text below and copy manually.",
      );
    });
  });
});
