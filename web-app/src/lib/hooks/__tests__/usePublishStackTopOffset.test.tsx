import React, { useRef } from "react";
import { act, render } from "@testing-library/react";
import { usePublishDeckTopOffset, usePublishStackTopOffset } from "@/lib/hooks/usePublishStackTopOffset";
import { getStackTopOffset, setDeckTopOffset, subscribeStackTopOffset } from "@/lib/utils/toastDock";

let bottom = 120;
let height = 40;

function Row() {
  const ref = useRef<HTMLDivElement>(null);
  usePublishStackTopOffset(ref);
  return <div ref={ref} data-testid="row" />;
}

const cssVar = () => document.documentElement.style.getPropertyValue("--mobile-stack-top-offset");

describe("usePublishStackTopOffset", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    bottom = 120;
    height = 40;
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      () => ({ bottom, top: bottom - 40, left: 0, right: 390, width: 390, height, x: 0, y: bottom - 40, toJSON: () => ({}) }) as DOMRect,
    );
  });
  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it("stack_top_offset_var_should_equal_tab_row_bottom_within_1px_when_banner_shown_dismissed_or_wrapped_and_be_0px_without_tab_row", () => {
    const { unmount } = render(<Row />);
    expect(cssVar()).toBe("120px");
    expect(getStackTopOffset()).toBe(120);

    // A banner appears above the row: nothing resizes, but the DOM mutates.
    bottom = 168;
    act(() => {
      document.body.appendChild(document.createElement("div"));
      jest.advanceTimersByTime(50);
    });
    expect(Math.abs(Number.parseFloat(cssVar()) - 168)).toBeLessThanOrEqual(1);

    // The banner wraps to two lines.
    bottom = 200;
    act(() => {
      window.dispatchEvent(new Event("resize"));
      jest.advanceTimersByTime(50);
    });
    expect(cssVar()).toBe("200px");

    unmount();
    expect(cssVar()).toBe("0px");
    expect(getStackTopOffset()).toBeNull();
  });

  it("usePublishBottomEdge_should_PublishOncePerChangedValue_When_8SessionDetailViewsMounted", () => {
    const onChange = jest.fn();
    const unsubscribe = subscribeStackTopOffset(onChange);
    const views = Array.from({ length: 8 }, () => render(<Row />));
    // Eight views measure the same edge, but toastDock only notifies on a changed value.
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(getStackTopOffset()).toBe(120);

    bottom = 150;
    act(() => {
      window.dispatchEvent(new Event("resize"));
      jest.advanceTimersByTime(50);
    });
    expect(onChange).toHaveBeenCalledTimes(2);
    expect(getStackTopOffset()).toBe(150);

    unsubscribe();
    views.forEach((v) => v.unmount());
  });

  describe("usePublishDeckTopOffset", () => {
    const deckVar = () => document.documentElement.style.getPropertyValue("--mobile-deck-top-offset");
    function Toolbar() {
      const ref = useRef<HTMLDivElement>(null);
      usePublishDeckTopOffset(ref);
      return <div ref={ref} />;
    }

    it("publishes the toolbar's bottom edge and clears it on unmount", () => {
      bottom = 330;
      const { unmount } = render(<Toolbar />);
      expect(deckVar()).toBe("330px");
      unmount();
      expect(deckVar()).toBe("");
    });

    it("publishes nothing while the toolbar is hidden so the deck keeps the tab-row anchor", () => {
      height = 0;
      bottom = 0;
      render(<Toolbar />);
      expect(deckVar()).toBe("");
    });

    it("a disabled (background) terminal neither publishes nor clears the visible terminal's anchor", () => {
      function Pane({ visible }: { visible: boolean }) {
        const ref = useRef<HTMLDivElement>(null);
        usePublishDeckTopOffset(ref, visible);
        return <div ref={ref} />;
      }
      bottom = 330;
      render(<Pane visible />);
      expect(deckVar()).toBe("330px");
      const background = render(<Pane visible={false} />);
      background.unmount();
      expect(deckVar()).toBe("330px");
    });

    it("setDeckTopOffset skips the style write when the value is unchanged", () => {
      const setProperty = jest.spyOn(document.documentElement.style, "setProperty");
      const owner = Symbol("o");
      setDeckTopOffset(250, owner);
      setDeckTopOffset(250, owner);
      expect(setProperty).toHaveBeenCalledTimes(1);
      setDeckTopOffset(null, owner);
    });

    it("only the owner may clear the anchor when two terminals are mounted", () => {
      function Pane() {
        const ref = useRef<HTMLDivElement>(null);
        usePublishDeckTopOffset(ref);
        return <div ref={ref} />;
      }
      bottom = 330;
      const first = render(<Pane />);
      bottom = 400;
      const second = render(<Pane />);
      expect(deckVar()).toBe("400px");
      first.unmount(); // not the owner
      expect(deckVar()).toBe("400px");
      second.unmount();
      expect(deckVar()).toBe("");
    });
  });
});
