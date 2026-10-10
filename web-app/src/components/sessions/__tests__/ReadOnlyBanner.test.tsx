import React from "react";
import { render, screen } from "@testing-library/react";
import { ReadOnlyBanner } from "../ReadOnlyBanner";
import {
  READ_ONLY_BANNER_PRIMARY,
  READ_ONLY_SECONDARY,
  READ_ONLY_SECONDARY_STREAM_ONLY,
  READ_ONLY_SECONDARY_WITH_REPLY,
} from "../readOnlyCopy";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";

function renderWithAnnouncer(ui: React.ReactElement) {
  const announce = jest.fn();
  const utils = render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>{ui}</AnnouncerContext.Provider>,
  );
  return { announce, ...utils };
}

describe("ReadOnlyBanner (RO-1, RO-5, RO-8, RO-12)", () => {
  it("ro8_should_announce_once_via_announcer_and_render_no_live_role", () => {
    const { announce, rerender } = renderWithAnnouncer(<ReadOnlyBanner />);

    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith(expect.stringContaining(READ_ONLY_BANNER_PRIMARY), "polite", expect.any(String));

    rerender(
      <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
        <ReadOnlyBanner replyCardPresent />
      </AnnouncerContext.Provider>,
    );
    expect(announce).toHaveBeenCalledTimes(1);

    const banner = screen.getByTestId("readonly-banner");
    expect(banner.getAttribute("role")).toBeNull();
    expect(banner.querySelector("[role],[aria-live]")).toBeNull();
  });

  it("ro1_should_show_the_primary_text_exactly", () => {
    renderWithAnnouncer(<ReadOnlyBanner />);
    expect(screen.getByTestId("readonly-banner")).toHaveTextContent("Background session - read-only");
  });

  it("ro12_should_use_the_stream_only_secondary_text_until_pr_5u_and_the_reply_text_with_a_card", () => {
    const { rerender } = renderWithAnnouncer(<ReadOnlyBanner />);
    expect(screen.getByTestId("readonly-banner-secondary")).toHaveTextContent(READ_ONLY_SECONDARY);
    expect(READ_ONLY_SECONDARY).toBe(READ_ONLY_SECONDARY_STREAM_ONLY);
    expect(READ_ONLY_SECONDARY_STREAM_ONLY).toBe("Terminal input is disabled in this view.");

    rerender(<ReadOnlyBanner replyCardPresent />);
    expect(screen.getByTestId("readonly-banner-secondary")).toHaveTextContent(READ_ONLY_SECONDARY_WITH_REPLY);
  });
});
