import React, { useRef } from "react";
import { act, render } from "@testing-library/react";
import { usePublishStackTopOffset } from "@/lib/hooks/usePublishStackTopOffset";
import { getStackTopOffset } from "@/lib/utils/toastDock";

let bottom = 120;

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
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      () => ({ bottom, top: bottom - 40, left: 0, right: 390, width: 390, height: 40, x: 0, y: bottom - 40, toJSON: () => ({}) }) as DOMRect,
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
});
