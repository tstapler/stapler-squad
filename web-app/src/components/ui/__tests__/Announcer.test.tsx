import React from "react";
import { act, render, screen } from "@testing-library/react";
import { AnnouncerProvider } from "@/components/ui/Announcer";
import { useAnnounce } from "@/lib/hooks/useAnnounce";

type Api = ReturnType<typeof useAnnounce>;
let api: Api;

function Capture() {
  api = useAnnounce();
  return null;
}

function mount() {
  return render(
    <AnnouncerProvider>
      <Capture />
    </AnnouncerProvider>,
  );
}

const polite = () => screen.getByTestId("announcer-polite");
const assertive = () => screen.getByTestId("announcer-assertive");

describe("Announcer", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("announcer_should_render_one_polite_and_one_assertive_region_always_mounted", () => {
    const { container } = mount();

    expect(polite()).toHaveAttribute("role", "status");
    expect(assertive()).toHaveAttribute("role", "alert");
    expect(screen.getAllByTestId("announcer-polite")).toHaveLength(1);
    expect(screen.getAllByTestId("announcer-assertive")).toHaveLength(1);
    // A live role plus aria-live="polite" double-announces in some readers.
    expect(container.querySelectorAll('[role="status"][aria-live], [role="alert"][aria-live]')).toHaveLength(0);
  });

  it("announces a plain message and clears it so an identical later message is announced again", () => {
    mount();

    act(() => api.announce("Saved"));
    expect(polite()).toHaveTextContent("Saved");

    act(() => {
      jest.advanceTimersByTime(2_000);
    });
    expect(polite()).toHaveTextContent("");

    act(() => api.announce("Saved"));
    expect(polite()).toHaveTextContent("Saved");
  });

  it("announcer_should_coalesce_burst_per_channel_and_send_one_assertive_summary_for_pinned_burst", () => {
    mount();

    act(() => {
      for (let i = 0; i < 5; i++) api.announceArrival({ title: `info ${i}`, pinned: false });
    });
    expect(polite()).toHaveTextContent("");
    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(polite()).toHaveTextContent("5 new notifications");
    expect(assertive()).toHaveTextContent("");
  });

  it("announces a lone pinned arrival assertively by title", () => {
    mount();

    act(() => api.announceArrival({ title: "Approve rm -rf?", pinned: true }));
    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(assertive()).toHaveTextContent("Approve rm -rf?");
    expect(polite()).toHaveTextContent("");
  });

  it("summarises a pinned burst once, naming the newest", () => {
    mount();

    act(() => {
      for (let i = 1; i <= 5; i++) api.announceArrival({ title: `decision ${i}`, pinned: true });
    });
    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(assertive()).toHaveTextContent("5 notifications need attention, newest: decision 5");
  });

  it("sends one assertive message, and no polite one, for a mixed burst", () => {
    mount();

    act(() => {
      api.announceArrival({ title: "a", pinned: true });
      api.announceArrival({ title: "b", pinned: false });
      api.announceArrival({ title: "c", pinned: false });
      api.announceArrival({ title: "d", pinned: false });
    });
    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(assertive()).toHaveTextContent("4 new notifications, 1 needs attention");
    expect(polite()).toHaveTextContent("");
  });

  it("announcer_should_preempt_polite_with_assertive_dedupe_by_key_and_cap_queue_at_5", () => {
    mount();

    act(() => {
      api.announce("first");
      api.announce("queued-a", "polite", "k");
      api.announce("queued-b", "polite", "k"); // same key: replaces queued-a
      api.announce("urgent", "assertive");
    });
    expect(polite()).toHaveTextContent("first");

    // The assertive channel never waits behind the polite one.
    expect(assertive()).toHaveTextContent("urgent");

    // Replaced by key: only the newer text is ever spoken.
    act(() => {
      jest.advanceTimersByTime(1_000);
    });
    expect(polite()).toHaveTextContent("queued-b");
  });

  it("keeps at most 5 queued polite messages, dropping the oldest", () => {
    mount();

    act(() => {
      for (let i = 0; i < 9; i++) api.announce(`m${i}`);
    });
    const spoken = [polite().textContent];
    for (let i = 0; i < 8; i++) {
      act(() => {
        jest.advanceTimersByTime(1_000);
      });
      spoken.push(polite().textContent);
    }

    // m0 spoke immediately; of the 8 that queued behind it only the newest 5 survive.
    expect(spoken.slice(0, 6)).toEqual(["m0", "m4", "m5", "m6", "m7", "m8"]);
  });

  it("is a harmless no-op outside a provider", () => {
    function Lone() {
      const lone = useAnnounce();
      lone.announce("x");
      lone.announceArrival({ title: "t", pinned: true });
      return <span>ok</span>;
    }
    render(<Lone />);
    expect(screen.getByText("ok")).toBeInTheDocument();
  });
});
