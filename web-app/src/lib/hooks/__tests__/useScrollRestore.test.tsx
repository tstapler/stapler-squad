import React from "react";
import { render } from "@testing-library/react";
import { fireEvent } from "@testing-library/react";
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
