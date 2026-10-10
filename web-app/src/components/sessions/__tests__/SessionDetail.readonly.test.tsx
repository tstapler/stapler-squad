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
jest.mock("@/lib/store", () => ({ useAppSelector: () => [] }));
jest.mock("@/lib/store/sessionsSlice", () => ({ selectAllSessions: () => [] }));

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
