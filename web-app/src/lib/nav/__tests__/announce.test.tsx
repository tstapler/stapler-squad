import { act, render, screen } from "@testing-library/react";
import { MemoryRouter, useNavigate } from "react-router";
import { RouteAnnouncer } from "../RouteAnnouncer";
import { announce, formatWindowAnnouncement } from "../announce";

let navigateTo: (path: string) => void = () => undefined;
function NavCapture() {
  const navigate = useNavigate();
  navigateTo = (path) => void navigate(path);
  return null;
}

function setup() {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <NavCapture />
      <RouteAnnouncer />
    </MemoryRouter>,
  );
  return () => screen.getByTestId("route-announcer");
}

const flush = (ms: number) => act(async () => void jest.advanceTimersByTime(ms));

beforeEach(() => {
  jest.useFakeTimers();
  document.title = "Home - Stapler Squad";
});
afterEach(() => jest.useRealTimers());

describe("announce", () => {
  it("windowSwitch_should_AnnounceOnceInSingleLiveRegion_When_NoPathnameChange", async () => {
    const region = setup();
    const seen: string[] = [];
    new MutationObserver(() => seen.push(region().textContent ?? "")).observe(region(), {
      childList: true,
      characterData: true,
      subtree: true,
    });
    act(() => announce(formatWindowAnnouncement(2, 3, "api")));
    await flush(150);
    expect(region().textContent).toBe("Window 2 of 3: api");
    expect(seen.filter((text) => text === "Window 2 of 3: api")).toHaveLength(1);
    expect(document.querySelectorAll("[aria-live]")).toHaveLength(1);
    expect(document.querySelectorAll("[role=status]")).toHaveLength(1);
  });

  it("formatWindowAnnouncement_should_OmitName_When_NameEmpty", () => {
    expect(formatWindowAnnouncement(2, 3, "")).toBe("Window 2 of 3");
    expect(formatWindowAnnouncement(2, 3, "  ")).toBe("Window 2 of 3");
  });

  it("announce_should_ReAnnounce_When_IdenticalMessageSentTwice", async () => {
    const region = setup();
    const seen: string[] = [];
    new MutationObserver(() => seen.push(region().textContent ?? "")).observe(region(), {
      childList: true,
      characterData: true,
      subtree: true,
    });
    act(() => announce("Window 2 of 3: api"));
    await flush(150);
    await flush(1500);
    act(() => announce("Window 2 of 3: api"));
    expect(region().textContent).toBe("");
    await flush(150);
    expect(region().textContent).toBe("Window 2 of 3: api");
    expect(seen.filter((text) => text === "Window 2 of 3: api")).toHaveLength(2);
  });

  it("announce_should_LetPathnameAnnouncementWin_When_SameTickConflict", async () => {
    const region = setup();
    act(() => {
      navigateTo("/history");
      announce("Window 2 of 3: api");
    });
    await flush(150);
    expect(region().textContent).toBe("Home - Stapler Squad");
  });

  it("announce_should_ReplaceEarlierMessage_When_LaterAnnounceInSameTick", async () => {
    const region = setup();
    act(() => {
      announce("first");
      announce("second");
    });
    await flush(150);
    expect(region().textContent).toBe("second");
  });
});
