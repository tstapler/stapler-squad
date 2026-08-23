import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { HandoffSummarySection } from "./HandoffSummarySection";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import type { HandoffSummaryProto } from "@/gen/session/v1/handoff_summary_pb";

// ---------------------------------------------------------------------------
// Mocks -- mirrors RestartWithSummaryButton.test.tsx's harness, since
// HandoffSummarySection embeds the real (unmocked) RestartWithSummaryButton,
// which independently calls the same useHandoffSummary(sessionId) hook.
// Mocking the hook module here drives both call sites from one fixture.
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
    Code: { FailedPrecondition: 9 },
  };
});

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
  });

  it("HandoffSummarySection_should_RenderExplicitEmptyState_When_NoRowExists", () => {
    mockHookReturn({ data: null });

    render(<HandoffSummarySection sessionId="session-1" />);

    expect(
      screen.getByText("No handoff summary generated for this session.")
    ).toBeInTheDocument();
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });

  it("HandoffSummarySection_should_UseListRoles_NotStatusRole_When_Rendered", () => {
    mockHookReturn({ data: makeSummary({ status: HandoffSummaryStatus.READY }) });

    render(<HandoffSummarySection sessionId="session-1" />);

    const list = screen.getByRole("list", { name: /handoff summary/i });
    expect(list).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();

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
});
