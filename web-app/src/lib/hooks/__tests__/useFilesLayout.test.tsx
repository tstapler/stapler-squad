import React, { useRef } from "react";
import { act, render, screen } from "@testing-library/react";
import { useFilesLayout } from "../useFilesLayout";

let observed: ((entries: { contentRect: { width: number } }[]) => void) | null = null;
const originalRO = globalThis.ResizeObserver;
const originalMM = window.matchMedia;
const screenSize = (width: number, height: number) => {
  Object.defineProperty(window.screen, "width", { configurable: true, value: width });
  Object.defineProperty(window.screen, "height", { configurable: true, value: height });
};
const coarse = (on: boolean) => {
  window.matchMedia = jest.fn().mockImplementation((q: string) => ({
    matches: on && q === "(pointer: coarse)", media: q, addEventListener: jest.fn(), removeEventListener: jest.fn(),
  })) as never;
};

function Probe({ width }: { width: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const { layout, containerWidth } = useFilesLayout(ref);
  return <div ref={ref} data-testid="probe" data-width={width}>{layout}:{containerWidth}</div>;
}

beforeEach(() => {
  observed = null;
  globalThis.ResizeObserver = class {
    constructor(cb: never) { observed = cb; }
    observe() {}
    disconnect() {}
    unobserve() {}
  } as never;
  jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ width: 0 } as DOMRect);
});
afterEach(() => {
  globalThis.ResizeObserver = originalRO;
  window.matchMedia = originalMM;
  jest.restoreAllMocks();
});

describe("useFilesLayout", () => {
  it("useFilesLayout_should_SwitchLayoutWithoutRemount_When_ContainerCrosses768", () => {
    coarse(false);
    screenSize(1920, 1080);
    render(<Probe width={0} />);
    const node = screen.getByTestId("probe");
    act(() => observed?.([{ contentRect: { width: 700 } }]));
    expect(node).toHaveTextContent("mobile:700");
    act(() => observed?.([{ contentRect: { width: 900 } }]));
    expect(screen.getByTestId("probe")).toBe(node);
    expect(node).toHaveTextContent("desktop:900");
  });

  it("useFilesLayout_should_StayMobile_When_PhoneLandscapeAndStayDesktop_When_TabletKeyboardShrinksViewport", () => {
    coarse(true);
    screenSize(844, 390);
    const phone = render(<Probe width={0} />);
    act(() => observed?.([{ contentRect: { width: 844 } }]));
    expect(phone.getByTestId("probe")).toHaveTextContent("mobile:844");
    phone.unmount();

    screenSize(1180, 820);
    render(<Probe width={0} />);
    act(() => observed?.([{ contentRect: { width: 1180 } }]));
    act(() => { Object.defineProperty(window, "innerHeight", { configurable: true, value: 470 }); window.dispatchEvent(new Event("resize")); });
    expect(screen.getByTestId("probe")).toHaveTextContent("desktop:1180");
  });
});
