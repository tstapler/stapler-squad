import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { ReplyCard } from "../ReplyCard";
import { ReplyOutcome } from "@/gen/session/v1/session_pb";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import * as connectivity from "@/lib/hooks/useNotificationConnectivity";

jest.mock("@/lib/hooks/useNotificationConnectivity", () => ({
  useNotificationConnectivity: jest.fn(() => ({ state: "connected", isOffline: false })),
}));

const replyable = {
  id: "n1",
  notificationType: "question" as const,
  message: "Which color should the spike use?",
  metadata: { question_id: "q1", question_shape: "single", question_options: '["Red","Green","Blue"]' },
};

type Send = jest.Mock<Promise<{ outcome: ReplyOutcome; retryAfterSeconds: number }>, [unknown]>;

function setup(send: Send, extra: Partial<React.ComponentProps<typeof ReplyCard>> = {}) {
  const announce = jest.fn();
  let n = 0;
  const utils = render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <ReplyCard
        sessionId="s1"
        notification={replyable}
        hookOverrides={{ sendReply: send, newReplyId: () => `rid-${++n}`, now: () => Date.UTC(2026, 9, 10, 10, 42) }}
        {...extra}
      />
    </AnnouncerContext.Provider>,
  );
  return { ...utils, announce };
}

const respond = (outcome: ReplyOutcome, retryAfterSeconds = 0): Send =>
  jest.fn().mockResolvedValue({ outcome, retryAfterSeconds });

describe("ReplyCard", () => {
  beforeEach(() => {
    (connectivity.useNotificationConnectivity as jest.Mock).mockReturnValue({ state: "connected", isOffline: false });
  });

  it("renders_one_numbered_44px_button_per_option_and_no_text_field", () => {
    setup(respond(ReplyOutcome.SENT));
    expect(screen.getByRole("group", { name: "Claude has a question" })).toBeInTheDocument();
    expect(screen.getByTestId("reply-option-1")).toHaveTextContent("1. Red");
    expect(screen.getByTestId("reply-option-2")).toHaveTextContent("2. Green");
    expect(screen.getByTestId("reply-option-3")).toHaveTextContent("3. Blue");
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.getByText(/Sent to the background session's terminal once. This reply is logged./)).toBeInTheDocument();
  });

  it("sends_the_option_digit_once_and_shows_a_receipt_with_the_notification_marked_read", async () => {
    const send = respond(ReplyOutcome.SENT);
    const onSent = jest.fn();
    const { announce } = setup(send, { onSent });

    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-2"));
    });

    expect(send).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenCalledWith({ sessionId: "s1", questionId: "q1", replyText: "2", replyId: "rid-1" });
    expect(screen.getByTestId("reply-receipt")).toHaveTextContent(/Sent: 2\. Green at .* - the question closed/);
    expect(screen.queryByTestId("reply-option-1")).toBeNull();
    expect(onSent).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith(expect.stringMatching(/^Sent: 2\. Green/), "polite", "reply:q1");
  });

  it("disables_every_button_on_the_first_tap_so_a_double_tap_sends_one_reply", async () => {
    let resolve!: (v: { outcome: ReplyOutcome; retryAfterSeconds: number }) => void;
    const send: Send = jest.fn().mockReturnValue(new Promise((r) => (resolve = r)));
    setup(send);

    fireEvent.click(screen.getByTestId("reply-option-1"));
    fireEvent.click(screen.getByTestId("reply-option-1"));
    fireEvent.click(screen.getByTestId("reply-option-3"));

    expect(send).toHaveBeenCalledTimes(1);
    for (const n of [1, 2, 3]) expect(screen.getByTestId(`reply-option-${n}`)).toBeDisabled();
    expect(screen.getByTestId("reply-card")).toHaveAttribute("aria-busy", "true");
    await act(async () => resolve({ outcome: ReplyOutcome.SENT, retryAfterSeconds: 0 }));
  });

  it("shows_indeterminate_with_no_retry_and_keeps_the_chosen_option", async () => {
    const onViewOutput = jest.fn();
    const { announce } = setup(respond(ReplyOutcome.SEND_INDETERMINATE), { onViewOutput });
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-1"));
    });
    expect(screen.getByTestId("reply-status")).toHaveTextContent("Sent? Check the terminal output to confirm");
    expect(screen.queryByTestId("reply-retry")).toBeNull();
    expect(screen.getByTestId("reply-option-1")).toBeDisabled();
    fireEvent.click(screen.getByTestId("reply-view-output"));
    expect(onViewOutput).toHaveBeenCalled();
    expect(announce).toHaveBeenCalledWith("Sent? Check the terminal output to confirm", "assertive", "reply:q1");
  });

  it("retries_with_the_same_reply_id_after_not_sent", async () => {
    const send: Send = jest
      .fn()
      .mockResolvedValueOnce({ outcome: ReplyOutcome.NOT_SENT, retryAfterSeconds: 0 })
      .mockResolvedValueOnce({ outcome: ReplyOutcome.SENT, retryAfterSeconds: 0 });
    setup(send);
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-3"));
    });
    expect(screen.getByTestId("reply-status")).toHaveTextContent("Could not send");
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-retry"));
    });
    expect(send).toHaveBeenCalledTimes(2);
    expect(send.mock.calls[1][0]).toEqual(send.mock.calls[0][0]);
    expect(screen.getByTestId("reply-receipt")).toBeInTheDocument();
  });

  it("uses_a_new_reply_id_when_a_different_option_is_chosen_after_a_failure", async () => {
    const send: Send = jest.fn().mockResolvedValue({ outcome: ReplyOutcome.NOT_SENT, retryAfterSeconds: 0 });
    setup(send);
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-1"));
    });
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-2"));
    });
    expect((send.mock.calls[0][0] as { replyId: string }).replyId).toBe("rid-1");
    expect((send.mock.calls[1][0] as { replyId: string }).replyId).toBe("rid-2");
  });

  it.each([
    [ReplyOutcome.NO_PENDING, "This question is no longer waiting. You may have answered it elsewhere."],
    [ReplyOutcome.STALE_PROMPT, "This question is no longer on screen. You may have answered it already. Check the terminal."],
    [ReplyOutcome.NOT_WAITING, "The question isn't showing yet - try again in a moment"],
  ])("shows_the_exact_copy_for_outcome_%s", async (outcome, copy) => {
    setup(respond(outcome));
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-1"));
    });
    expect(screen.getByTestId("reply-status")).toHaveTextContent(copy);
  });

  it("shows_stale_prompt_with_view_output_and_no_retry", async () => {
    setup(respond(ReplyOutcome.STALE_PROMPT), { onViewOutput: jest.fn() });
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-1"));
    });
    expect(screen.getByTestId("reply-view-output")).toBeInTheDocument();
    expect(screen.queryByTestId("reply-retry")).toBeNull();
  });

  it("keeps_the_options_and_re_enables_them_after_the_rate_limit_wait", async () => {
    jest.useFakeTimers();
    try {
      setup(respond(ReplyOutcome.RATE_LIMITED, 4));
      await act(async () => {
        fireEvent.click(screen.getByTestId("reply-option-1"));
      });
      expect(screen.getByTestId("reply-status")).toHaveTextContent("Please wait 4 seconds");
      expect(screen.getByTestId("reply-option-1")).toBeDisabled();
      await act(async () => {
        jest.advanceTimersByTime(4000);
      });
      expect(screen.getByTestId("reply-option-1")).toBeEnabled();
    } finally {
      jest.useRealTimers();
    }
  });

  it("maps_connect_errors_invalid_argument_and_permission_denied_without_retry", async () => {
    const { ConnectError, Code } = await import("@connectrpc/connect");
    const denied: Send = jest.fn().mockRejectedValue(new ConnectError("no", Code.PermissionDenied));
    const view = setup(denied);
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-1"));
    });
    expect(screen.getByTestId("reply-status")).toHaveTextContent("This device is not allowed to reply");
    expect(screen.queryByTestId("reply-retry")).toBeNull();
    view.unmount();

    const invalid: Send = jest.fn().mockRejectedValue(new ConnectError("bad", Code.InvalidArgument));
    setup(invalid);
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-1"));
    });
    expect(screen.getByTestId("reply-status")).toHaveTextContent("That option is not available. Check the terminal.");
  });

  it("treats_an_internal_error_or_network_failure_as_retryable", async () => {
    const { ConnectError, Code } = await import("@connectrpc/connect");
    const send: Send = jest.fn().mockRejectedValue(new ConnectError("audit", Code.Internal));
    setup(send);
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-1"));
    });
    expect(screen.getByTestId("reply-retry")).toBeInTheDocument();
  });

  it("disables_the_buttons_with_the_offline_reason_and_sends_nothing", () => {
    (connectivity.useNotificationConnectivity as jest.Mock).mockReturnValue({ state: "disconnected", isOffline: true });
    const send = respond(ReplyOutcome.SENT);
    setup(send);
    expect(screen.getByTestId("reply-option-1")).toBeDisabled();
    expect(screen.getByTestId("reply-status")).toHaveTextContent("Offline - reconnect to reply");
    fireEvent.click(screen.getByTestId("reply-option-1"));
    expect(send).not.toHaveBeenCalled();
  });

  it("removes_the_card_when_the_session_turned_visible_or_reply_is_disabled", async () => {
    setup(respond(ReplyOutcome.NOT_HIDDEN));
    await act(async () => {
      fireEvent.click(screen.getByTestId("reply-option-1"));
    });
    expect(screen.queryByTestId("reply-card")).toBeNull();
  });

  it("truncates_a_long_prompt_at_280_characters_with_a_show_full_prompt_disclosure", () => {
    const long = "w".repeat(400);
    render(
      <ReplyCard
        sessionId="s1"
        notification={{ ...replyable, message: long }}
        hookOverrides={{ sendReply: respond(ReplyOutcome.SENT) }}
      />,
    );
    expect(screen.getByTestId("reply-prompt").textContent).toHaveLength(281); // 280 + ellipsis
    fireEvent.click(screen.getByTestId("reply-show-full"));
    expect(screen.getByTestId("reply-prompt")).toHaveTextContent(long);
  });

  it.each([
    ["shape", { question_shape: "multi" }, "Answer in the terminal"],
    ["no_proof", { reply_unavailable: "no_proof", question_shape: "single" }, "Reply unavailable for this session. Answer in the terminal."],
    ["bad_proof", { reply_unavailable: "bad_proof", question_shape: "single" }, "Reply unavailable for this session. Answer in the terminal."],
  ])("shows_no_buttons_for_an_unavailable_question_cause_%s", (_c, md, copy) => {
    render(<ReplyCard sessionId="s1" notification={{ ...replyable, metadata: md }} />);
    expect(screen.getByTestId("reply-unavailable")).toHaveTextContent(copy);
    expect(screen.queryByTestId("reply-option-1")).toBeNull();
  });

  it("renders_nothing_for_a_notification_that_is_not_a_question", () => {
    const { container } = render(
      <ReplyCard sessionId="s1" notification={{ ...replyable, notificationType: "error" }} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
