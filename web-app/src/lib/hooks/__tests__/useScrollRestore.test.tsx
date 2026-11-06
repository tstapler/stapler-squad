import React from "react";
import { render, fireEvent, act } from "@testing-library/react";
import { useScrollRestore } from "../useScrollRestore";

function Box({ k }: { k: string }) {
  const ref = useScrollRestore(k);
  return <div ref={ref} data-testid="box" />;
}

// jsdom has no layout, so scrollTop is a plain writable number here.
describe("useScrollRestore", () => {
  it("useScrollRestore_should_notSetScrollTop_When_noSavedPosition", () => {
    const { getByTestId } = render(<Box k="fresh" />);
    expect(getByTestId("box").scrollTop).toBe(0);
  });

  describe("when content is not yet tall enough (browser clamps scrollTop)", () => {
    // Emulates layout: scrollTop is clamped to limit.max, like a virtualized list
    // that has not sized its spacers at ref-attach time.
    function ClampedBox({ k, limit }: { k: string; limit: { max: number } }) {
      const restoreRef = useScrollRestore(k);
      const ref = React.useCallback(
        (el: HTMLElement | null) => {
          if (el) {
            let v = 0;
            Object.defineProperty(el, "scrollTop", {
              configurable: true,
              get: () => v,
              set: (n: number) => {
                v = Math.min(n, limit.max);
              },
            });
          }
          restoreRef(el);
        },
        [restoreRef, limit],
      );
      return <div ref={ref} data-testid="box" />;
    }

    function saveAt(key: string, top: number) {
      const first = render(<Box k={key} />);
      const el = first.getByTestId("box");
      el.scrollTop = top;
      fireEvent.scroll(el);
      first.unmount();
    }

    beforeEach(() => jest.useFakeTimers());
    afterEach(() => jest.useRealTimers());

    it("useScrollRestore_should_retryUntilReachable_When_contentGrowsAfterAttach", () => {
      saveAt("grow", 500);
      const limit = { max: 0 };
      const { getByTestId } = render(<ClampedBox k="grow" limit={limit} />);
      expect(getByTestId("box").scrollTop).toBe(0);

      limit.max = 1000;
      act(() => {
        jest.advanceTimersByTime(32);
      });
      expect(getByTestId("box").scrollTop).toBe(500);
    });

    it("useScrollRestore_should_notOverwriteSavedPosition_When_clampedScrollFiresDuringRestore", () => {
      saveAt("clamp", 500);
      const limit = { max: 100 };
      const clamped = render(<ClampedBox k="clamp" limit={limit} />);
      const el = clamped.getByTestId("box");
      expect(el.scrollTop).toBe(100);
      fireEvent.scroll(el); // the clamp's own scroll event
      clamped.unmount();

      const again = render(<Box k="clamp" />);
      expect(again.getByTestId("box").scrollTop).toBe(500);
    });

    it("useScrollRestore_should_stopRestoring_When_userInteracts", () => {
      saveAt("user", 500);
      const limit = { max: 0 };
      const { getByTestId } = render(<ClampedBox k="user" limit={limit} />);
      const el = getByTestId("box");
      fireEvent.touchStart(el);

      limit.max = 1000;
      act(() => {
        jest.advanceTimersByTime(32);
      });
      expect(el.scrollTop).toBe(0);
    });
  });

  it("useScrollRestore_should_restoreScrollTop_When_remountedAfterScroll", () => {
    const first = render(<Box k="remount" />);
    const el = first.getByTestId("box");
    el.scrollTop = 150;
    fireEvent.scroll(el);
    first.unmount();

    const second = render(<Box k="remount" />);
    expect(second.getByTestId("box").scrollTop).toBe(150);
  });

  it("useScrollRestore_should_keepPositionsIndependent_When_keysDiffer", () => {
    const a = render(<Box k="a" />);
    a.getByTestId("box").scrollTop = 40;
    fireEvent.scroll(a.getByTestId("box"));
    a.unmount();

    const b = render(<Box k="b" />);
    expect(b.getByTestId("box").scrollTop).toBe(0);
  });
});
