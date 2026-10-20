/**
 * useHashRowFocus (Story 1.3.3, UX-37/UX-47): terminal-state gating, 10 s ceiling,
 * no-steal rules and hashchange/popstate handling.
 */

import { act, fireEvent, render } from "@testing-library/react";
import { HASH_FOCUS_CEILING_MS, useHashRowFocus, type ListState } from "./useHashRowFocus";

const ROW_A = "flag-row-a";
const ROW_B = "flag-row-b";

function HashFocusHarness({ state }: { state: ListState }) {
  useHashRowFocus(state);
  return (
    <main id="main-content">
      <button type="button" data-testid="other">
        other
      </button>
      <div id={ROW_A} tabIndex={-1} />
      <div id={ROW_B} tabIndex={-1} />
    </main>
  );
}

function setHashNow(hash: string) {
  // history.replaceState changes the URL without queueing a hashchange event.
  window.history.replaceState(null, "", hash ? `#${hash}` : window.location.pathname);
}

const scrollIntoView = jest.fn();

beforeEach(() => {
  jest.useFakeTimers();
  scrollIntoView.mockReset();
  Element.prototype.scrollIntoView = scrollIntoView;
  setHashNow("");
});

afterEach(() => {
  jest.useRealTimers();
});

describe("useHashRowFocus", () => {
  it("focuses and centres the row once the list is loaded", () => {
    setHashNow(ROW_A);
    const { rerender } = render(<HashFocusHarness state="loading" />);
    expect(document.body).toHaveFocus();

    rerender(<HashFocusHarness state="loaded" />);
    expect(document.getElementById(ROW_A)).toHaveFocus();
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
    expect(scrollIntoView).toHaveBeenCalledWith({ block: "center" });
  });

  it("ignores a hash that names no row", () => {
    setHashNow("flag-row-missing");
    render(<HashFocusHarness state="loaded" />);
    expect(document.body).toHaveFocus();
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it("waits through a failed load and focuses once after a successful retry", () => {
    setHashNow(ROW_A);
    const { rerender } = render(<HashFocusHarness state="failed" />);
    expect(document.body).toHaveFocus();

    rerender(<HashFocusHarness state="loading" />);
    rerender(<HashFocusHarness state="loaded" />);
    expect(document.getElementById(ROW_A)).toHaveFocus();
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("does not move focus again when a later refetch re-renders the list", () => {
    setHashNow(ROW_A);
    const { rerender } = render(<HashFocusHarness state="loaded" />);
    (document.getElementById(ROW_B) as HTMLElement).focus();

    rerender(<HashFocusHarness state="loading" />);
    rerender(<HashFocusHarness state="loaded" />);
    expect(document.getElementById(ROW_B)).toHaveFocus();
    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("gives up with no focus change when the list is still loading after 10 s", () => {
    setHashNow(ROW_A);
    const { rerender } = render(<HashFocusHarness state="loading" />);
    act(() => {
      jest.advanceTimersByTime(HASH_FOCUS_CEILING_MS + 1);
    });
    rerender(<HashFocusHarness state="loaded" />);
    expect(document.body).toHaveFocus();
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it("does not steal focus after a key press", () => {
    setHashNow(ROW_A);
    const { rerender } = render(<HashFocusHarness state="loading" />);
    fireEvent.keyDown(document.body, { key: "Tab" });
    rerender(<HashFocusHarness state="loaded" />);
    expect(document.getElementById(ROW_A)).not.toHaveFocus();
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it("does not steal focus after a pointer press", () => {
    setHashNow(ROW_A);
    const { rerender } = render(<HashFocusHarness state="loading" />);
    fireEvent.pointerDown(document.body);
    rerender(<HashFocusHarness state="loaded" />);
    expect(document.getElementById(ROW_A)).not.toHaveFocus();
  });

  it("does not steal focus from an element other than body or the page container", () => {
    setHashNow(ROW_A);
    const { rerender, getByTestId } = render(<HashFocusHarness state="loading" />);
    getByTestId("other").focus();
    rerender(<HashFocusHarness state="loaded" />);
    expect(getByTestId("other")).toHaveFocus();
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it("focuses another row on hashchange and on popstate, once each", () => {
    setHashNow(ROW_A);
    render(<HashFocusHarness state="loaded" />);
    expect(document.getElementById(ROW_A)).toHaveFocus();

    setHashNow(ROW_B);
    act(() => {
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    expect(document.getElementById(ROW_B)).toHaveFocus();

    setHashNow(ROW_A);
    act(() => {
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(document.getElementById(ROW_A)).toHaveFocus();
    expect(scrollIntoView).toHaveBeenCalledTimes(3);
  });

  it("does nothing when there is no hash", () => {
    render(<HashFocusHarness state="loaded" />);
    expect(document.body).toHaveFocus();
    expect(scrollIntoView).not.toHaveBeenCalled();
  });
});
