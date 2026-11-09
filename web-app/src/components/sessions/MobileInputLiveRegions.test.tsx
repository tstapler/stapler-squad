import { act, render, screen } from "@testing-library/react";
import { useMobileInputLiveRegions } from "./MobileInputLiveRegions";

let api: ReturnType<typeof useMobileInputLiveRegions>;
function Host() {
  api = useMobileInputLiveRegions();
  return <div>{api.regions}</div>;
}

describe("useMobileInputLiveRegions", () => {
  it("regions_should_ExistEmptyBeforeAnyMessage_When_Mounted", () => {
    render(<Host />);
    const polite = screen.getByRole("status");
    const assertive = screen.getByRole("alert");
    expect(polite).toHaveAttribute("aria-live", "polite");
    expect(assertive).toHaveAttribute("aria-live", "assertive");
    expect(polite).toBeEmptyDOMElement();
    expect(assertive).toBeEmptyDOMElement();
  });

  it("regions_should_WriteIntoNamedRegionOnlyAndMutateOnce_When_SameStateAnnouncedFiveTimes", () => {
    render(<Host />);
    const polite = screen.getByRole("status");
    const assertive = screen.getByRole("alert");
    const mutations = jest.fn();
    const observer = new MutationObserver(mutations);
    observer.observe(polite, { childList: true, characterData: true, subtree: true });
    act(() => {
      for (let i = 0; i < 5; i++) api.announcer.announce("chip-disconnected", "Disconnected", "disc");
    });
    expect(polite).toHaveTextContent("Disconnected");
    expect(assertive).toBeEmptyDOMElement();
    const records = observer.takeRecords();
    expect(records.length).toBe(1);
    observer.disconnect();
  });

  it("regions_should_RouteDiscardToAssertiveOnly_When_OverflowNotice", () => {
    render(<Host />);
    act(() => api.announcer.announce("discard-overflow", "Typed text was dropped", "ovf"));
    expect(screen.getByRole("alert")).toHaveTextContent("Typed text was dropped");
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
  });
});
