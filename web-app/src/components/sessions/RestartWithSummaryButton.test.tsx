import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { RestartWithSummaryButton } from "./RestartWithSummaryButton";
import { useHandoffSummary } from "@/lib/hooks/useHandoffSummary";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import type { HandoffSummaryProto } from "@/gen/session/v1/handoff_summary_pb";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// The component no longer calls useHandoffSummary itself (Finding 2 -- the
// hook's result is passed in as the `handoff` prop, lifted to the caller so
// two button instances don't run two independent polls), so this module is
// no longer mocked -- the real `isGenerating` export is used as-is, and
// `useHandoffSummary` itself is unused at runtime by the component under
// test (only referenced at the type level for the prop's shape).
const mockTrigger = jest.fn();

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
    Code: { FailedPrecondition: 9, NotFound: 5 },
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

/** Builds the `handoff` prop RestartWithSummaryButton now takes directly. */
function makeHandoff(
  overrides: Partial<Record<string, unknown>> = {},
): ReturnType<typeof useHandoffSummary> {
  return {
    data: null,
    loading: false,
    error: null,
    neverResolved: false,
    trigger: mockTrigger,
    refetch: jest.fn(),
    ...overrides,
  } as unknown as ReturnType<typeof useHandoffSummary>;
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

    render(<RestartWithSummaryButton sessionId="session-1" handoff={makeHandoff({ data: null })} />);

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
    mockCreateSession.mockResolvedValue({ id: "new-session-42" });

    render(
      <RestartWithSummaryButton
        sessionId="source-session-1"
        handoff={makeHandoff({ data: summary })}
      />,
    );

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

  it("RestartWithSummaryButton_should_ShowLiveSourceWarningAndRequireExplicitConfirm_When_RestartRejectsWithFailedPrecondition", async () => {
    // Finding 1: the still-live-source guard (connect.CodeFailedPrecondition)
    // must NOT be silently retried -- the first click's rejection has to
    // surface a real, visible warning, and only a second, distinctly-labeled
    // "Restart anyway" click may retry with confirmRestartWithLiveSource:true.
    const { ConnectError, Code } = jest.requireMock("@connectrpc/connect") as {
      ConnectError: new (message: string, code: number) => Error;
      Code: { FailedPrecondition: number };
    };
    const summary = makeSummary({ summaryText: "Session recap text" });
    mockCreateSession
      .mockRejectedValueOnce(
        new ConnectError("source session is still live", Code.FailedPrecondition),
      )
      .mockResolvedValueOnce({ id: "new-session-42" });

    render(
      <RestartWithSummaryButton
        sessionId="source-session-1"
        handoff={makeHandoff({ data: summary })}
      />,
    );

    fireEvent.click(screen.getByTestId("restart-with-summary-button"));

    // First click alone must not create the second session -- no silent
    // retry, and no navigation yet.
    await waitFor(() => expect(mockCreateSession).toHaveBeenCalledTimes(1));
    expect(mockCreateSession).toHaveBeenNthCalledWith(1, {
      prompt: "Session recap text",
      restartFromSessionId: "source-session-1",
    });
    expect(pushMock).not.toHaveBeenCalled();

    // The warning renders, and the plain "Start new session..." button is
    // gone -- the only affordance left is the distinctly-labeled confirm.
    const warning = await screen.findByTestId("restart-with-summary-live-source-warning");
    expect(warning).toHaveTextContent(/still running/i);
    expect(screen.queryByTestId("restart-with-summary-button")).not.toBeInTheDocument();

    const confirmButton = screen.getByTestId("restart-with-summary-confirm-live-source");
    expect(confirmButton).toHaveTextContent("Restart anyway");

    fireEvent.click(confirmButton);

    await waitFor(() => expect(mockCreateSession).toHaveBeenCalledTimes(2));
    expect(mockCreateSession).toHaveBeenNthCalledWith(2, {
      prompt: "Session recap text",
      restartFromSessionId: "source-session-1",
      confirmRestartWithLiveSource: true,
    });

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/?session=new-session-42"));
    expect(screen.queryByTestId("restart-with-summary-restart-error")).not.toBeInTheDocument();
  });

  it("RestartWithSummaryButton_should_ReturnToReadyAndShowError_When_RestartFailsWithNonRetryableError", async () => {
    // design/ux.md's "no dead ends" acceptance criterion #2: a restart
    // failure that ISN'T the live-source guard (the FailedPrecondition path
    // above) must revert the button to READY, re-clickable, with an inline
    // error.
    const summary = makeSummary({ summaryText: "Session recap text" });
    mockCreateSession.mockRejectedValue(new Error("network error"));

    render(
      <RestartWithSummaryButton
        sessionId="source-session-1"
        handoff={makeHandoff({ data: summary })}
      />,
    );

    const button = screen.getByTestId("restart-with-summary-button");
    fireEvent.click(button);

    // design/ux.md's restart-session-creation-failure flow: the raw
    // transport message is never shown -- an unrecognized/generic failure
    // maps to the plain-language fallback reason, verbatim.
    await waitFor(() =>
      expect(screen.getByTestId("restart-with-summary-restart-error")).toHaveTextContent(
        "Couldn't start the new session. Something went wrong — try again.",
      ),
    );
    expect(mockCreateSession).toHaveBeenCalledTimes(1);
    expect(button).not.toBeDisabled();
    expect(button).toHaveTextContent("Start new session from this summary");
    expect(screen.queryByTestId("restart-with-summary-live-source-warning")).not.toBeInTheDocument();

    fireEvent.click(button);
    await waitFor(() => expect(mockCreateSession).toHaveBeenCalledTimes(2));
  });

  it("RestartWithSummaryButton_should_ShowSourceGoneMessage_When_RestartFailsWithNotFound", async () => {
    // design/ux.md's restart-session-creation-failure flow's CodeNotFound
    // branch: the source session was archived/deleted between generating
    // the summary and clicking restart.
    const { ConnectError, Code } = jest.requireMock("@connectrpc/connect") as {
      ConnectError: new (message: string, code: number) => Error;
      Code: { NotFound: number };
    };
    const summary = makeSummary({ summaryText: "Session recap text" });
    mockCreateSession.mockRejectedValue(
      new ConnectError("session not found", Code.NotFound),
    );

    render(
      <RestartWithSummaryButton
        sessionId="source-session-1"
        handoff={makeHandoff({ data: summary })}
      />,
    );

    fireEvent.click(screen.getByTestId("restart-with-summary-button"));

    await waitFor(() =>
      expect(screen.getByTestId("restart-with-summary-restart-error")).toHaveTextContent(
        "Couldn't start the new session. The original session no longer exists.",
      ),
    );
  });

  it("RestartWithSummaryButton_should_RenderNothing_When_FeatureDisabled", async () => {
    const { ConnectError, Code } = jest.requireMock("@connectrpc/connect") as {
      ConnectError: new (message: string, code: number) => Error;
      Code: { FailedPrecondition: number };
    };
    mockTrigger.mockRejectedValue(
      new ConnectError("handoff summaries disabled", Code.FailedPrecondition),
    );

    render(<RestartWithSummaryButton sessionId="session-1" handoff={makeHandoff({ data: null })} />);

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

    render(<RestartWithSummaryButton sessionId="session-1" handoff={makeHandoff({ data: summary })} />);

    expect(screen.getByTestId("restart-with-summary-error")).toHaveTextContent(
      "LLM request timed out",
    );

    const retryButton = screen.getByTestId("restart-with-summary-retry");
    expect(retryButton).toHaveTextContent("Try again");

    fireEvent.click(retryButton);
    expect(mockTrigger).toHaveBeenCalledTimes(1);
  });

  // design/ux.md UX AC #3: the primary error text must be a plain-language
  // message mapped from error_stage, never the raw error_message -- the raw
  // text may still be present, but only inside a collapsed disclosure.
  it("RestartWithSummaryButton_should_ShowPlainLanguageMessage_When_ErrorStageIsTranscript", () => {
    const summary = makeSummary({
      status: HandoffSummaryStatus.ERROR,
      errorStage: "transcript",
      errorMessage: "conversation file not found for session ID: sess-1",
    });

    render(<RestartWithSummaryButton sessionId="session-1" handoff={makeHandoff({ data: summary })} />);

    const primaryText = screen.getByTestId("restart-with-summary-error-message");
    expect(primaryText).toHaveTextContent("Couldn't read this session's conversation history.");
    expect(primaryText).not.toHaveTextContent("conversation file not found");

    // The raw detail is still present, but only inside a <details> disclosure.
    const details = screen.getByText("Details").closest("details");
    expect(details).not.toBeNull();
    expect(details).toHaveTextContent("conversation file not found for session ID: sess-1");
  });

  // design/ux.md's error table requires this exact string for stage
  // "generation" -- distinct from the "transcript" stage's message.
  it("RestartWithSummaryButton_should_ShowPlainLanguageMessage_When_ErrorStageIsGeneration", () => {
    const summary = makeSummary({
      status: HandoffSummaryStatus.ERROR,
      errorStage: "generation",
      errorMessage: "pool call failed: context deadline exceeded",
    });

    render(<RestartWithSummaryButton sessionId="session-1" handoff={makeHandoff({ data: summary })} />);

    expect(screen.getByTestId("restart-with-summary-error-message")).toHaveTextContent(
      "Failed while generating the handoff summary.",
    );
  });

  // design/ux.md's error table's explicit fallback row: an unrecognized or
  // future stage string must never surface as the primary text.
  it("RestartWithSummaryButton_should_ShowGenericFallbackMessage_When_ErrorStageIsUnrecognized", () => {
    const summary = makeSummary({
      status: HandoffSummaryStatus.ERROR,
      errorStage: "some-future-stage",
      errorMessage: "raw internal detail",
    });

    render(<RestartWithSummaryButton sessionId="session-1" handoff={makeHandoff({ data: summary })} />);

    expect(screen.getByTestId("restart-with-summary-error-message")).toHaveTextContent(
      "Something went wrong while generating this summary.",
    );
  });
});
