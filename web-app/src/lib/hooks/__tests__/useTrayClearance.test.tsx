import React from "react";
import { act, render } from "@testing-library/react";
import { trayClearance, useTrayClearanceRef } from "../useTrayClearance";

const box = (left: number, right: number) => ({ left, right });

describe("trayClearance", () => {
  it("is_zero_when_the_box_ends_before_the_right_bell_column", () => {
    expect(trayClearance(box(56, 376), 1400, 44)).toBe(0);
    expect(trayClearance(box(0, 1356), 1400, 44)).toBe(0);
  });

  it("is_the_overlap_when_the_box_reaches_the_right_bell_column", () => {
    expect(trayClearance(box(0, 1400), 1400, 44)).toBe(44);
    expect(trayClearance(box(0, 1380), 1400, 44)).toBe(24);
  });

  it("measures_the_left_edge_when_the_bell_is_on_the_left", () => {
    expect(trayClearance(box(0, 1400), 1400, 44, "left")).toBe(44);
    expect(trayClearance(box(30, 1400), 1400, 44, "left")).toBe(14);
    expect(trayClearance(box(56, 376), 1400, 44, "left")).toBe(0);
  });

  it("is_zero_when_the_bell_is_not_shown", () => {
    expect(trayClearance(box(0, 1400), 1400, 0)).toBe(0);
  });
});

function Box({ left = 0, right }: { left?: number; right: number }) {
  const ref = useTrayClearanceRef();
  return (
    <div
      data-testid="box"
      ref={(el) => {
        if (el) el.getBoundingClientRect = () => ({ left, right }) as DOMRect;
        ref(el);
      }}
    />
  );
}

const style = () => document.documentElement.style;
const read = (el: HTMLElement) => ({
  right: el.style.getPropertyValue("--tray-clearance"),
  left: el.style.getPropertyValue("--tray-clearance-left"),
});

describe("useTrayClearanceRef", () => {
  beforeEach(() => {
    Object.defineProperty(window, "innerWidth", { value: 1000, configurable: true });
  });
  afterEach(() => {
    style().removeProperty("--tray-handle-width");
    style().removeProperty("--tray-handle-edge");
  });

  it("sets_full_right_clearance_on_a_full_width_box_while_the_bell_is_published", () => {
    style().setProperty("--tray-handle-width", "44px");
    const { getByTestId } = render(<Box right={1000} />);
    expect(read(getByTestId("box"))).toEqual({ right: "44px", left: "0px" });
  });

  it("sets_zero_clearance_on_a_narrow_box_that_never_reaches_the_bell", () => {
    style().setProperty("--tray-handle-width", "44px");
    const { getByTestId } = render(<Box right={320} />);
    expect(read(getByTestId("box"))).toEqual({ right: "0px", left: "0px" });
  });

  it("sets_zero_clearance_when_no_bell_is_published", () => {
    const { getByTestId } = render(<Box right={1000} />);
    expect(read(getByTestId("box"))).toEqual({ right: "0px", left: "0px" });
  });

  it("pads_the_left_instead_when_the_floating_bell_is_on_the_left_edge", () => {
    style().setProperty("--tray-handle-width", "44px");
    style().setProperty("--tray-handle-edge", "left");
    const { getByTestId } = render(<Box left={0} right={1000} />);
    expect(read(getByTestId("box"))).toEqual({ right: "0px", left: "44px" });
  });

  it("updates_when_the_bell_is_published_after_the_box_mounted", async () => {
    const { getByTestId } = render(<Box right={1000} />);
    expect(read(getByTestId("box")).right).toBe("0px");
    await act(async () => {
      style().setProperty("--tray-handle-width", "44px");
      await Promise.resolve();
    });
    expect(read(getByTestId("box")).right).toBe("44px");
  });

  it("moves_the_padding_to_the_other_side_when_the_bell_edge_flips_after_mount", async () => {
    style().setProperty("--tray-handle-width", "44px");
    style().setProperty("--tray-handle-edge", "right");
    const { getByTestId } = render(<Box left={0} right={1000} />);
    expect(read(getByTestId("box"))).toEqual({ right: "44px", left: "0px" });
    await act(async () => {
      style().setProperty("--tray-handle-edge", "left");
      await Promise.resolve();
    });
    expect(read(getByTestId("box"))).toEqual({ right: "0px", left: "44px" });
  });
});
