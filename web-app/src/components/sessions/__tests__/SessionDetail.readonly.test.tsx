/**
 * Story 5.3 (RO-1, RO-8): SessionDetail derives the read-only view from `session.hidden`
 * and mounts the banner above the detail view; a visible session is untouched.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionDetail } from "../SessionDetail";
import type { Session } from "@/gen/session/v1/types_pb";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";

jest.mock("next/dynamic", () => () =>
  function DetailViewStub() {
    return <div data-testid="session-detail-view-stub" />;
  },
);
jest.mock("@/lib/hooks/useSessionActions", () => ({ useSessionActions: () => ({}) }));
jest.mock("@/lib/contexts/SessionVcsContext", () => ({
  SessionVcsProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
jest.mock("@/lib/hooks/useVcsStatus", () => ({ prefetchVcsStatus: jest.fn() }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543" }));
let mockSessionsError: string | null = null;
let mockHistory: unknown[] = [];
let mockFlags: Record<string, boolean> = {};
const mockDispatch = jest.fn();
const mockShowActionToast = jest.fn();
jest.mock("@/lib/store", () => ({
  useAppSelector: (selector: (state: unknown) => unknown) => selector({}),
  useAppDispatch: () => mockDispatch,
}));
jest.mock("@/lib/store/sessionsSlice", () => ({
  selectAllSessions: () => [],
  selectSessionsError: () => mockSessionsError,
  setError: (value: string | null) => ({ type: "sessions/setError", payload: value }),
}));
jest.mock("@/lib/contexts/notificationContexts", () => ({
  useNotificationCommands: () => ({ showActionToast: mockShowActionToast, markAsRead: jest.fn() }),
  useNotificationState: () => ({ notificationHistory: mockHistory }),
}));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (name: string) => mockFlags[name] ?? false,
}));

const session = (hidden: boolean) => ({ id: "s1", title: "review:ee1b4be0", hidden }) as unknown as Session;

function renderDetail(hidden: boolean) {
  const announce = jest.fn();
  render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <SessionDetail session={session(hidden)} onClose={jest.fn()} />
    </AnnouncerContext.Provider>,
  );
  return { announce };
}

describe("SessionDetail read-only banner", () => {
  it("ro1_should_show_the_banner_above_the_view_for_a_hidden_session", () => {
    const { announce } = renderDetail(true);
    const banner = screen.getByTestId("readonly-banner");
    const view = screen.getByTestId("session-detail-view-stub");
    expect(banner.compareDocumentPosition(view) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(announce).toHaveBeenCalledTimes(1);
  });

  it("should_render_no_banner_for_a_visible_session", () => {
    const { announce } = renderDetail(false);
    expect(screen.queryByTestId("readonly-banner")).toBeNull();
    expect(screen.getByTestId("session-detail-view-stub")).toBeInTheDocument();
    expect(announce).not.toHaveBeenCalled();
  });
});

describe("SessionDetail Reply card (Story 5.6)", () => {
  const question = {
    id: "n1",
    sessionId: "s1",
    sessionName: "review:ee1b4be0",
    message: "Which color?",
    timestamp: 5,
    notificationType: "question",
    isRead: false,
    metadata: { question_id: "q1", question_shape: "single", question_options: '["Red","Green"]' },
  };

  beforeEach(() => {
    mockHistory = [question];
    mockFlags = { hidden_session_reply: true };
  });
  afterEach(() => {
    mockHistory = [];
    mockFlags = {};
  });

  it("rp1_should_show_the_card_directly_below_the_banner_for_a_hidden_session_with_a_pending_question", () => {
    renderDetail(true);
    const banner = screen.getByTestId("readonly-banner");
    const card = screen.getByTestId("reply-card");
    const view = screen.getByTestId("session-detail-view-stub");
    expect(banner.compareDocumentPosition(card) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(card.compareDocumentPosition(view) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByTestId("reply-option-2")).toHaveTextContent("2. Green");
    expect(screen.getByTestId("readonly-banner-secondary")).toHaveTextContent(/Reply to Claude's question below/);
  });

  it("rp1_should_never_show_the_card_for_a_visible_session", () => {
    renderDetail(false);
    expect(screen.queryByTestId("reply-card")).toBeNull();
  });

  it("rp1_should_show_no_card_when_no_question_is_pending_or_the_kill_switch_is_off", () => {
    mockHistory = [{ ...question, isRead: true }];
    renderDetail(true);
    expect(screen.queryByTestId("reply-card")).toBeNull();
  });

  it("rp_kill_switch_should_hide_the_card_when_hidden_session_reply_is_off", () => {
    mockFlags = { hidden_session_reply: false };
    renderDetail(true);
    expect(screen.queryByTestId("reply-card")).toBeNull();
  });
});

describe("SessionDetail server write refusal (T-RC-09)", () => {
  const REFUSAL = "[failed_precondition] this session is a background session and is read-only";

  beforeEach(() => {
    mockSessionsError = null;
    mockDispatch.mockClear();
    mockShowActionToast.mockClear();
  });

  it("session_detail_should_toast_this_session_is_read_only_not_stack_trace_when_server_rejects_write", () => {
    mockSessionsError = REFUSAL;
    renderDetail(true);

    expect(mockShowActionToast).toHaveBeenCalledWith("This session is read-only", "error", "readonly-refusal");
    // The raw RPC text is cleared from the store so no other surface shows it.
    expect(mockDispatch).toHaveBeenCalledWith({ type: "sessions/setError", payload: null });
  });

  it("should_leave_other_errors_and_visible_sessions_alone", () => {
    mockSessionsError = REFUSAL;
    renderDetail(false);
    expect(mockShowActionToast).not.toHaveBeenCalled();

    mockSessionsError = "Failed to restart session";
    renderDetail(true);
    expect(mockShowActionToast).not.toHaveBeenCalled();
    expect(mockDispatch).not.toHaveBeenCalled();
  });
});
