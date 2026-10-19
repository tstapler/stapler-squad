import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { useUnsavedChangesGuard } from "./useUnsavedChangesGuard";

const replace = jest.fn();
const push = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push }),
}));

function Harness({ dirty, savePending = false }: { dirty: boolean; savePending?: boolean }) {
  const guard = useUnsavedChangesGuard({ dirty, savePending });
  return (
    <div>
      <a href="/elsewhere" data-testid="inapp">in-app</a>
      <a href="/elsewhere" target="_blank" rel="noopener" data-testid="blank">blank</a>
      <a href="https://example.org/x" data-testid="external">external</a>
      {guard.promptOpen && (
        <div data-testid="prompt">
          <button onClick={guard.keepEditing}>Keep editing</button>
          <button onClick={guard.discard}>Discard</button>
        </div>
      )}
    </div>
  );
}

const pop = () => act(() => { window.dispatchEvent(new PopStateEvent("popstate")); });

let pushState: jest.SpyInstance;
let back: jest.SpyInstance;
let addSpy: jest.SpyInstance;

beforeEach(() => {
  replace.mockClear();
  push.mockClear();
  pushState = jest.spyOn(window.history, "pushState").mockImplementation(() => undefined);
  back = jest.spyOn(window.history, "back").mockImplementation(() => undefined);
  addSpy = jest.spyOn(window, "addEventListener");
});
afterEach(() => jest.restoreAllMocks());

const beforeUnloadRegistrations = () => addSpy.mock.calls.filter(([t]) => t === "beforeunload").length;

describe("useUnsavedChangesGuard", () => {
  it("registers no beforeunload handler and pushes no sentinel when clean", () => {
    render(<Harness dirty={false} />);
    expect(beforeUnloadRegistrations()).toBe(0);
    expect(pushState).not.toHaveBeenCalled();
  });

  it("registers beforeunload when dirty and when a save is pending", () => {
    const { unmount } = render(<Harness dirty />);
    expect(beforeUnloadRegistrations()).toBe(1);
    unmount();
    addSpy.mockClear();
    render(<Harness dirty={false} savePending />);
    expect(beforeUnloadRegistrations()).toBe(1);
  });

  it("pushes one same-URL sentinel when the form turns dirty", () => {
    const { rerender } = render(<Harness dirty={false} />);
    rerender(<Harness dirty />);
    expect(pushState).toHaveBeenCalledTimes(1);
    expect(pushState).toHaveBeenCalledWith({ ssqLeaveGuard: true }, "", window.location.href);
  });

  it("Back opens the dialog; Keep editing re-arms; a second Back re-opens it; Discard completes the Back", () => {
    render(<Harness dirty />);
    pop();
    expect(screen.getByTestId("prompt")).toBeInTheDocument();
    expect(back).not.toHaveBeenCalled();

    fireEvent.click(screen.getByText("Keep editing"));
    expect(screen.queryByTestId("prompt")).toBeNull();
    expect(pushState).toHaveBeenCalledTimes(2);

    pop();
    expect(screen.getByTestId("prompt")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Discard"));
    expect(back).toHaveBeenCalledTimes(1);
    expect(pushState).toHaveBeenCalledTimes(2);
  });

  it("skips the sentinel with an immediate back() when the form became clean", () => {
    const { rerender } = render(<Harness dirty />);
    rerender(<Harness dirty={false} />);
    pop();
    expect(screen.queryByTestId("prompt")).toBeNull();
    expect(back).toHaveBeenCalledTimes(1);
  });

  it("intercepts a same-origin link click and Discard uses router.replace while armed", () => {
    render(<Harness dirty />);
    fireEvent.click(screen.getByTestId("inapp"));
    expect(screen.getByTestId("prompt")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Discard"));
    expect(replace).toHaveBeenCalledWith("/elsewhere");
    expect(push).not.toHaveBeenCalled();
  });

  it("lets external, target=_blank and modifier-key clicks pass through", () => {
    render(<Harness dirty />);
    fireEvent.click(screen.getByTestId("external"));
    fireEvent.click(screen.getByTestId("blank"));
    fireEvent.click(screen.getByTestId("inapp"), { metaKey: true });
    expect(screen.queryByTestId("prompt")).toBeNull();
  });

  it("does not intercept links when clean", () => {
    render(<Harness dirty={false} />);
    fireEvent.click(screen.getByTestId("inapp"));
    expect(screen.queryByTestId("prompt")).toBeNull();
  });
});
