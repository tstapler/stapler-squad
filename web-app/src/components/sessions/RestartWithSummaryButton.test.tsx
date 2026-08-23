import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { RestartWithSummaryButton } from "./RestartWithSummaryButton";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import type { HandoffSummaryProto } from "@/gen/session/v1/handoff_summary_pb";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockTrigger = jest.fn();
const mockUseHandoffSummary = jest.fn();

jest.mock("@/lib/hooks/useHandoffSummary", () => ({
  // Keep the real isGenerating export -- only the hook itself is mocked.
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

// A minimal stand-in for ConnectError/Code so `err instanceof ConnectError`
// and `err.code === Code.FailedPrecondition` checks in the component behave
// as they would against the real RPC client. Defined inline in the factory
// (rather than referencing an outer `class`) because jest hoists `jest.mock`
// calls above the rest of the module, including any class declarations that
// would otherwise appear earlier in this file.
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

describe("RestartWithSummaryButton", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("RestartWithSummaryButton_should_TriggerGeneration_When_ClickedWithNoSummary", async () => {
    // A controlled, not-yet-resolved promise so the disabled "Generating..."
    // state can be asserted deterministically before trigger() settles,
    // rather than racing a microtask that resolves immediately.
    let resolveTrigger: () => void = () => {};
    mockTrigger.mockImplementation(
      () => new Promise<void>((resolve) => { resolveTrigger = resolve; }),
    );
    mockHookReturn({ data: null });

    render(<RestartWithSummaryButton sessionId="session-1" />);

    const button = screen.getByTestId("restart-with-summary-button");
    expect(button).toHaveTextContent("Generate restart summary");

    fireEvent.click(button);

    expect(mockTrigger).toHaveBeenCalledTimes(1);
    await waitFor(() => {
      expect(screen.getByTestId("restart-with-summary-button")).toBeDisabled();
    });
    expect(screen.getByTestId("restart-with-summary-button")).toHaveTextContent(
      "Generating summary...",
    );

    // Matches SessionSummaryPanel.test.tsx's pending-action pattern: resolve
    // the controlled promise, then confirm trigger() was actually invoked.
    resolveTrigger();
    await waitFor(() => expect(mockTrigger).toHaveBeenCalledTimes(1));
  });

  it("RestartWithSummaryButton_should_CreateSessionWithSummaryAsPrompt_When_ClickedWhileReady", async () => {
    const summary = makeSummary({ summaryText: "Session recap text" });
    mockHookReturn({ data: summary });
    mockCreateSession.mockResolvedValue({ id: "new-session-42" });

    render(<RestartWithSummaryButton sessionId="source-session-1" />);

    const button = screen.getByTestId("restart-with-summary-button");
    expect(button).toHaveTextContent("Start new session from this summary");

    fireEvent.click(button);

    await waitFor(() => expect(mockCreateSession).toHaveBeenCalledTimes(1));
    expect(mockCreateSession).toHaveBeenCalledWith({
      prompt: "Session recap text",
      restartFromSessionId: "source-session-1",
    });

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/?session=new-session-42"));
  });

  it("RestartWithSummaryButton_should_RenderNothing_When_FeatureDisabled", async () => {
    const { ConnectError, Code } = jest.requireMock("@connectrpc/connect") as {
      ConnectError: new (message: string, code: number) => Error;
      Code: { FailedPrecondition: number };
    };
    mockTrigger.mockRejectedValue(
      new ConnectError("handoff summaries disabled", Code.FailedPrecondition),
    );
    mockHookReturn({ data: null });

    render(<RestartWithSummaryButton sessionId="session-1" />);

    const button = screen.getByTestId("restart-with-summary-button");
    fireEvent.click(button);

    await waitFor(() => {
      expect(screen.queryByTestId("restart-with-summary-button")).not.toBeInTheDocument();
    });
  });

  it("RestartWithSummaryButton_should_RenderErrorStateWithRetry_When_StatusError", async () => {
    mockTrigger.mockResolvedValue(undefined);
    const summary = makeSummary({
      status: HandoffSummaryStatus.ERROR,
      errorMessage: "LLM request timed out",
    });
    mockHookReturn({ data: summary });

    render(<RestartWithSummaryButton sessionId="session-1" />);

    expect(screen.getByTestId("restart-with-summary-error")).toHaveTextContent(
      "LLM request timed out",
    );

    const retryButton = screen.getByTestId("restart-with-summary-retry");
    expect(retryButton).toHaveTextContent("Try again");

    fireEvent.click(retryButton);
    expect(mockTrigger).toHaveBeenCalledTimes(1);
  });
});
