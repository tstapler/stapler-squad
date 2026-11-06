import React, { useState } from "react";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { lazyComponent } from "../impl.bun";
import { dirtyCount, isTracked, page, resetStaleChunkForTests } from "../stale-chunk";

const chunkError = () => Object.assign(new Error("Loading chunk 42 failed."), { name: "ChunkLoadError" });
const reload = jest.fn();

function ControlledInput({ testId = "field" }: { testId?: string }) {
  const [v, setV] = useState("");
  return <input data-testid={testId} value={v} onChange={(e) => setV(e.target.value)} />;
}

class Boundary extends React.Component<{ children: React.ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? <p>boundary</p> : this.props.children;
  }
}

function mountLazy(loader: Parameters<typeof lazyComponent>[0]) {
  const Lazy = lazyComponent(loader, { loading: () => <p>loading</p> });
  return render(
    <Boundary>
      <Lazy />
    </Boundary>
  );
}

const mountFailingLazy = () => mountLazy(() => Promise.reject(chunkError()));

beforeEach(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
  resetStaleChunkForTests();
  reload.mockReset();
  jest.spyOn(page, "reload").mockImplementation(reload);
  document.getElementById("stale-chunk-banner")?.remove();
});

afterEach(() => {
  jest.restoreAllMocks();
  jest.useRealTimers();
});

describe("lazyComponent (bun impl)", () => {
  it("renders the loading fallback, then the component", async () => {
    const Lazy = lazyComponent(async () => ({ default: () => <p>loaded</p> }), { loading: () => <p>loading</p> });
    render(<Lazy />);
    expect(screen.getByText("loading")).toBeInTheDocument();
    expect(await screen.findByText("loaded")).toBeInTheDocument();
  });

  it("reloads once on a chunk-load error when no field has input", async () => {
    mountFailingLazy();
    await act(async () => {});
    expect(reload).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("renders the ErrorBoundary on the second failure instead of reloading again", async () => {
    mountFailingLazy();
    await act(async () => {});
    mountFailingLazy();
    await act(async () => {});
    expect(reload).toHaveBeenCalledTimes(1);
    expect(screen.getByText("boundary")).toBeInTheDocument();
  });

  it("shows the banner, not a reload, when a real controlled input holds text", async () => {
    const user = userEvent.setup();
    render(<ControlledInput />);
    await user.type(screen.getByTestId("field"), "draft");
    mountFailingLazy();
    await act(async () => {});
    expect(reload).not.toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveTextContent("A new version is available. Reload when ready.");
  });

  it("does not guard on an emptied field", async () => {
    const user = userEvent.setup();
    render(<ControlledInput />);
    await user.type(screen.getByTestId("field"), "x");
    await user.clear(screen.getByTestId("field"));
    expect(dirtyCount()).toBe(0);
    mountFailingLazy();
    await act(async () => {});
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("prunes an edited field that was unmounted, then reloads once", async () => {
    const user = userEvent.setup();
    const { unmount } = render(<ControlledInput />);
    const node = screen.getByTestId("field");
    await user.type(node, "draft");
    expect(isTracked(node)).toBe(true);
    unmount();
    expect(isTracked(node)).toBe(false);
    expect(dirtyCount()).toBe(0);
    mountFailingLazy();
    await act(async () => {});
    expect(screen.queryByRole("status")).toBeNull();
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("ignores search boxes", async () => {
    const user = userEvent.setup();
    render(<input type="search" data-testid="s" />);
    await user.type(screen.getByTestId("s"), "query");
    mountFailingLazy();
    await act(async () => {});
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("clears the flag after a settled successful load so a later deploy recovers again", async () => {
    jest.useFakeTimers();
    mountFailingLazy();
    await act(async () => {});
    expect(reload).toHaveBeenCalledTimes(1);

    const Ok = lazyComponent(async () => ({ default: () => <p>ok</p> }));
    render(<Ok />);
    await act(async () => {});
    act(() => {
      jest.advanceTimersByTime(10_000);
    });

    mountFailingLazy();
    await act(async () => {});
    expect(reload).toHaveBeenCalledTimes(2);
  });

  it("still reloads at most once when sessionStorage throws", async () => {
    jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    mountFailingLazy();
    await act(async () => {});
    mountFailingLazy();
    await act(async () => {});
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("records a performance mark per recovery event", async () => {
    // jsdom has no performance.mark
    const mark = jest.fn();
    Object.defineProperty(performance, "mark", { configurable: true, writable: true, value: mark });
    mountFailingLazy();
    await act(async () => {});
    expect(mark).toHaveBeenCalledTimes(1);
    expect(mark).toHaveBeenCalledWith("stale-chunk-reload", { detail: { kind: "reload" } });
    delete (performance as { mark?: unknown }).mark;
  });

  it("passes non-chunk errors straight to the ErrorBoundary", async () => {
    mountLazy(() => Promise.reject(new Error("boom")));
    await act(async () => {});
    expect(reload).not.toHaveBeenCalled();
    expect(screen.getByText("boundary")).toBeInTheDocument();
  });
});
