import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { HandoffSummarySection } from "./HandoffSummarySection";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import type { HandoffSummaryProto } from "@/gen/session/v1/handoff_summary_pb";

// ---------------------------------------------------------------------------
// Mocks -- HandoffSummarySection calls useHandoffSummary(sessionId) itself
// (mocked here) and passes the result down as the `handoff` prop to every
// embedded (real, unmocked) RestartWithSummaryButton instance, so one
// fixture drives both.
// ---------------------------------------------------------------------------

const mockTrigger = jest.fn();
const mockUseHandoffSummary = jest.fn();

jest.mock("@/lib/hooks/useHandoffSummary", () => ({
  ...jest.requireActual("@/lib/hooks/useHandoffSummary"),
  useHandoffSummary: (sessionId: string) => mockUseHandoffSummary(sessionId),
}));

const mockCreateSession = jest.fn();
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ createSession: mockCreateSession }),
}));

const pushMock = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

jest.mock("@connectrpc/connect", () => {
  class MockConnectError extends Error {
    code: number;
    constructor(message: string, code: number) {
      super(message);
      this.name = "ConnectError";
      this.code = code;
    }
  }
  return {
    ConnectError: MockConnectError,
    Code: { FailedPrecondition: 9, NotFound: 5 },
  };
});

// The feature defaults to enabled server-side, so every existing test in
// this file (written before the disabled-state fix) expects enabled
// behavior by default -- individual disabled-state tests below override
// these per-test.
const mockUseFeatureFlags = jest.fn();
const mockUseFeatureFlag = jest.fn();
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => mockUseFeatureFlags(),
  useFeatureFlag: (name: string) => mockUseFeatureFlag(name),
}));

function makeSummary(overrides: Partial<HandoffSummaryProto> = {}): HandoffSummaryProto {
  return {
    sessionId: "session-1",
    status: HandoffSummaryStatus.READY,
    summaryText: "Fixed the login redirect loop.",
    errorMessage: "",
    generatedAt: { seconds: BigInt(Math.floor(Date.now() / 1000) - 120), nanos: 0 },
    ...overrides,
  } as unknown as HandoffSummaryProto;
}

function mockHookReturn(overrides: Partial<Record<string, unknown>> = {}) {
  mockUseHandoffSummary.mockReturnValue({
    data: null,
    loading: false,
    error: null,
    neverResolved: false,
    trigger: mockTrigger,
    refetch: jest.fn(),
    ...overrides,
  });
}

// The jest styleMock for `.css.ts` files wraps every export (including plain
// `style()` string exports) in a callable proxy, which triggers a benign
// "Invalid value for prop className" React warning -- see
// SessionDetailView.summary-tab.test.tsx, BacklogItemPanel.test.tsx, and
// RadioGroup.test.tsx, which silence it the same way.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

describe("HandoffSummarySection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseFeatureFlags.mockReturnValue({
      flags: { "handoff-summary": true },
      flagList: [],
      isLoading: false,
      error: null,
      setFlag: jest.fn(),
    });
    mockUseFeatureFlag.mockReturnValue(true);
  });

  it("HandoffSummarySection_should_RenderExplicitEmptyState_When_NoRowExists", () => {
    mockHookReturn({ data: null });

    render(<HandoffSummarySection sessionId="session-1" />);

    expect(
      screen.getByText("No handoff summary generated for this session.")
    ).toBeInTheDocument();
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);

    // The empty state is the common/default case -- the restart button must
    // still be reachable here, since it's the feature's only entry point for
    // generating a first summary (finding 1: it was previously gated behind
    // `data !== null`, making generation unreachable through the UI).
    const button = screen.getByTestId("restart-with-summary-button");
    expect(button).toBeInTheDocument();
    expect(button).not.toBeDisabled();
    expect(button).toHaveTextContent("Generate restart summary");
  });

  it("HandoffSummarySection_should_UseListRoles_NotStatusRole_When_Rendered", () => {
    mockHookReturn({ data: makeSummary({ status: HandoffSummaryStatus.READY }) });

    render(<HandoffSummarySection sessionId="session-1" />);

    const list = screen.getByRole("list", { name: /handoff summary/i });
    expect(list).toBeInTheDocument();
    // The row's own status label/icon must not be wrapped in role="status" --
    // this section is a user-reviewed historical record (list/listitem), not
    // a live-announced region. This doesn't rule out RestartWithSummaryButton
    // having its OWN, separate aria-live region for its click-triggered
    // action's state transitions (Finding 4) -- a distinct, legitimate
    // purpose nested inside the row -- so the assertion is scoped to the
    // status label itself rather than the whole document.
    expect(screen.getByText("Ready").closest('[role="status"]')).toBeNull();

    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(1);

    // Status icon must be visually decorative (aria-hidden) with a separate
    // visible text label alongside it -- not icon-only.
    expect(screen.getByText("✓")).toHaveAttribute("aria-hidden", "true");
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  it("HandoffSummarySection_should_EmbedRestartButton_When_StatusReady", () => {
    mockHookReturn({ data: makeSummary({ status: HandoffSummaryStatus.READY }) });

    render(<HandoffSummarySection sessionId="session-1" />);

    const button = screen.getByTestId("restart-with-summary-button");
    expect(button).toHaveTextContent("Start new session from this summary");

    // The button lives inside the row's listitem, not detached elsewhere.
    const item = screen.getByRole("listitem");
    expect(item).toContainElement(button);
  });

  it("HandoffSummarySection_should_RenderErrorRowWithRetry_When_StatusError", async () => {
    mockTrigger.mockResolvedValue(undefined);
    mockHookReturn({
      data: makeSummary({
        status: HandoffSummaryStatus.ERROR,
        errorMessage: "LLM request timed out",
      }),
    });

    render(<HandoffSummarySection sessionId="session-1" />);

    // The row itself still renders (not the no-row empty state).
    expect(
      screen.queryByText("No handoff summary generated for this session.")
    ).not.toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(1);

    // Delegated to RestartWithSummaryButton's own error-state retry.
    expect(screen.getByTestId("restart-with-summary-error")).toHaveTextContent(
      "LLM request timed out"
    );
    const retryButton = screen.getByTestId("restart-with-summary-retry");
    expect(retryButton).toHaveTextContent("Try again");

    fireEvent.click(retryButton);
    await waitFor(() => expect(mockTrigger).toHaveBeenCalledTimes(1));
  });

  it("HandoffSummarySection_should_RenderReadyRowDetails_When_StatusReady", () => {
    mockHookReturn({
      data: makeSummary({
        status: HandoffSummaryStatus.READY,
        middleMessagesSummarized: 12,
        activeTask: "Fix the flaky TestFoo assertion and re-run make test",
        summaryText: "REFERENCE-ONLY handoff text for the new session.",
      }),
    });

    render(<HandoffSummarySection sessionId="session-1" />);

    expect(screen.getByText("12 turns summarized")).toBeInTheDocument();
    expect(screen.getByText(/^ready \d/)).toBeInTheDocument();
    expect(
      screen.getByText("Active task: Fix the flaky TestFoo assertion and re-run make test"),
    ).toBeInTheDocument();

    const summaryToggle = screen.getByText("Preview full handoff text");
    const details = summaryToggle.closest("details");
    expect(details).not.toBeNull();
    expect(details).not.toHaveAttribute("open");
    expect(details).toHaveTextContent("REFERENCE-ONLY handoff text for the new session.");
  });

  it("HandoffSummarySection_should_ShowStartedRelativeTime_When_StatusGenerating", () => {
    mockHookReturn({
      data: makeSummary({
        status: HandoffSummaryStatus.GENERATING,
        generatedAt: undefined,
        generationStartedAt: { seconds: BigInt(Math.floor(Date.now() / 1000) - 4), nanos: 0 } as HandoffSummaryProto["generationStartedAt"],
      }),
    });

    render(<HandoffSummarySection sessionId="session-1" />);

    expect(screen.getByText(/^started \d+s ago$/)).toBeInTheDocument();
  });

  it("HandoffSummarySection_should_ShowDisabledText_When_FeatureDisabledAndNoRowExists", () => {
    mockUseFeatureFlags.mockReturnValue({
      flags: { "handoff-summary": false },
      flagList: [],
      isLoading: false,
      error: null,
      setFlag: jest.fn(),
    });
    mockUseFeatureFlag.mockReturnValue(false);
    mockHookReturn({ data: null });

    render(<HandoffSummarySection sessionId="session-1" />);

    expect(
      screen.getByText("Restart-with-summary is disabled for this workspace."),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("No handoff summary generated for this session."),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId("restart-with-summary-button")).not.toBeInTheDocument();
  });

  it("HandoffSummarySection_should_SuppressButtonOnly_When_FeatureDisabledAndRowExists", () => {
    mockUseFeatureFlags.mockReturnValue({
      flags: { "handoff-summary": false },
      flagList: [],
      isLoading: false,
      error: null,
      setFlag: jest.fn(),
    });
    mockUseFeatureFlag.mockReturnValue(false);
    mockHookReturn({
      data: makeSummary({
        status: HandoffSummaryStatus.READY,
        middleMessagesSummarized: 3,
      }),
    });

    render(<HandoffSummarySection sessionId="session-1" />);

    // The row's read-only info still renders...
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("3 turns summarized")).toBeInTheDocument();
    // ...but the action button is suppressed.
    expect(screen.queryByTestId("restart-with-summary-button")).not.toBeInTheDocument();
  });

  it("HandoffSummarySection_should_BehaveAsEnabled_When_FeatureFlagsStillLoading", () => {
    // The feature defaults to enabled server-side, but useFeatureFlag
    // defaults to `false` while still loading -- naively branching on that
    // would flash a false "disabled" message on every page load.
    mockUseFeatureFlags.mockReturnValue({
      flags: {},
      flagList: [],
      isLoading: true,
      error: null,
      setFlag: jest.fn(),
    });
    mockUseFeatureFlag.mockReturnValue(false);
    mockHookReturn({ data: null });

    render(<HandoffSummarySection sessionId="session-1" />);

    expect(
      screen.getByText("No handoff summary generated for this session."),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Restart-with-summary is disabled for this workspace."),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId("restart-with-summary-button")).toBeInTheDocument();
  });
});
