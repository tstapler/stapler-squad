import React from "react";
import { render } from "@testing-library/react";
import { trayClearance, useTrayClearanceRef } from "../useTrayClearance";

describe("trayClearance", () => {
  it("is_zero_when_the_box_ends_before_the_bell_column", () => {
    expect(trayClearance(376, 1400, 44)).toBe(0);
    expect(trayClearance(1356, 1400, 44)).toBe(0);
  });

  it("is_the_overlap_when_the_box_reaches_the_bell_column", () => {
    expect(trayClearance(1400, 1400, 44)).toBe(44);
    expect(trayClearance(1380, 1400, 44)).toBe(24);
  });

  it("is_zero_when_the_bell_is_not_shown", () => {
    expect(trayClearance(1400, 1400, 0)).toBe(0);
  });
});

function Box({ right }: { right: number }) {
  const ref = useTrayClearanceRef();
  return (
    <div
      data-testid="box"
      ref={(el) => {
        if (el) el.getBoundingClientRect = () => ({ right }) as DOMRect;
        ref(el);
      }}
    />
  );
}

describe("useTrayClearanceRef", () => {
  beforeEach(() => {
    Object.defineProperty(window, "innerWidth", { value: 1000, configurable: true });
  });
  afterEach(() => document.documentElement.style.removeProperty("--tray-handle-width"));

  it("sets_full_clearance_on_a_full_width_box_while_the_bell_is_published", () => {
    document.documentElement.style.setProperty("--tray-handle-width", "44px");
    const { getByTestId } = render(<Box right={1000} />);
    expect(getByTestId("box").style.getPropertyValue("--tray-clearance")).toBe("44px");
  });

  it("sets_zero_clearance_on_a_narrow_box_that_never_reaches_the_bell", () => {
    document.documentElement.style.setProperty("--tray-handle-width", "44px");
    const { getByTestId } = render(<Box right={320} />);
    expect(getByTestId("box").style.getPropertyValue("--tray-clearance")).toBe("0px");
  });

  it("sets_zero_clearance_when_no_bell_is_published", () => {
    const { getByTestId } = render(<Box right={1000} />);
    expect(getByTestId("box").style.getPropertyValue("--tray-clearance")).toBe("0px");
  });
});
