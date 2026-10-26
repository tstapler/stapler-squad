import { render, screen, act } from "@testing-library/react";

// Every RPC made through a connect client is recorded; the archived-session test asserts none revive.
const rpcCalls: string[] = [];
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () =>
    new Proxy({}, { get: (_t, method: string) => (...args: unknown[]) => { rpcCalls.push(method); return Promise.resolve(args); } }),
}));
import { WindowKeepAliveHost } from "./WindowKeepAliveHost";
import { useWindowFocusMemory } from "@/lib/window/useWindowFocusMemory";
import { initialPaneState } from "@/lib/pane/paneReducer";
import type { NamedWindow } from "@/lib/window/windowTypes";
import type { PaneState } from "@/lib/pane/paneTypes";

const makeWindow = (id: string, sessionId: string | null = null): NamedWindow => {
  const state: PaneState = initialPaneState();
  if (state.root.type === "split" && state.root.second.type === "leaf") {
    state.root = { ...state.root, second: { ...state.root.second, sessionId } };
  }
  return { id, name: id, paneState: state };
};

interface HarnessProps {
  windows: NamedWindow[];
  current: string;
  keepAlive?: boolean;
  assign?: { version: number } | null;
  seen?: Record<string, unknown>;
  panelRef?: React.RefObject<HTMLDivElement | null>;
  connecting?: boolean;
}

function Harness({ windows, current, keepAlive = true, assign = null, seen, panelRef, connecting = false }: HarnessProps) {
  const memory = useWindowFocusMemory();
  return (
    <WindowKeepAliveHost
      windows={windows}
      currentWindowId={current}
      keepAlive={keepAlive}
      focusMemory={memory}
      panelRef={panelRef}
      externalAssign={assign}
      renderConnecting={connecting ? (w) => <div data-testid={`connecting-${w.id}`} /> : undefined}
      renderWindow={(w, ctx) => {
        if (seen) seen[w.id] = ctx.externalAssign;
        return (
          <div data-testid={`pane-leaf-${w.id}-p`}>
            <button>{`btn-${w.id}`}</button>
          </div>
        );
      }}
    />
  );
}

const mountedIds = () => Array.from(document.querySelectorAll('[data-testid="window-panel"]')).map((el) => el.id);

describe("WindowKeepAliveHost", () => {
  it("host_should_MountW2W3W4AndUnmountW1_When_FourWindowsVisitedInOrderAndKis3", () => {
    const windows = ["w1", "w2", "w3", "w4"].map((id) => makeWindow(id));
    const { rerender } = render(<Harness windows={windows} current="w1" />);
    for (const id of ["w2", "w3", "w4"]) rerender(<Harness windows={windows} current={id} />);
    expect(mountedIds()).toEqual(["window-panel-w2", "window-panel-w3", "window-panel-w4"]);
    expect(screen.getByRole("tabpanel")).toHaveAttribute("id", "window-panel-w4");
    expect(document.getElementById("window-panel-w3")).toHaveAttribute("hidden");
    expect(document.getElementById("window-panel-w3")).toHaveAttribute("inert");
  });

  it("host_should_RenderSingleContainerInOneTabpanel_When_FlagOff", () => {
    const windows = ["w1", "w2"].map((id) => makeWindow(id));
    const { rerender } = render(<Harness windows={windows} current="w1" keepAlive={false} />);
    const panel = screen.getByRole("tabpanel");
    rerender(<Harness windows={windows} current="w2" keepAlive={false} />);
    expect(mountedIds()).toEqual(["window-panel-w2"]);
    expect(screen.getByRole("tabpanel")).toBe(panel);
    expect(screen.getByText("btn-w2")).toBeInTheDocument();
    expect(screen.queryByText("btn-w1")).toBeNull();
  });

  it("host_should_DeliverExternalAssignOnlyToActiveWindow_When_TwoWindowsMounted", () => {
    const windows = ["w1", "w2"].map((id) => makeWindow(id));
    const seen: Record<string, unknown> = {};
    const { rerender } = render(<Harness windows={windows} current="w1" />);
    rerender(<Harness windows={windows} current="w2" assign={{ version: 1 }} seen={seen} />);
    expect(mountedIds()).toHaveLength(2);
    expect(seen.w2).toEqual({ version: 1 });
    expect(seen.w1).toBeNull();
  });

  it("host_should_UnmountOtherWindow_When_SameSessionAssignedInIncomingWindow", () => {
    const w1 = makeWindow("w1", "sess-a");
    const w2 = makeWindow("w2", null);
    const { rerender } = render(<Harness windows={[w1, w2]} current="w1" />);
    rerender(<Harness windows={[w1, w2]} current="w2" />);
    expect(mountedIds()).toEqual(["window-panel-w1", "window-panel-w2"]);
    rerender(<Harness windows={[w1, makeWindow("w2", "sess-a")]} current="w2" />);
    expect(mountedIds()).toEqual(["window-panel-w2"]);
  });

  it("focusMemory_should_HandOffFocusBeforeInert_When_DeactivatingWindowHoldsFocus", () => {
    const windows = ["w1", "w2"].map((id) => makeWindow(id));
    const panelRef = { current: null as HTMLDivElement | null };
    const { rerender } = render(<Harness windows={windows} current="w1" panelRef={panelRef} />);
    rerender(<Harness windows={windows} current="w2" panelRef={panelRef} />);
    rerender(<Harness windows={windows} current="w1" panelRef={panelRef} />);
    act(() => screen.getByText("btn-w1").focus());
    const w1Panel = document.getElementById("window-panel-w1") as HTMLElement;

    // Capture the focus owner at the instant `inert` is applied (a real browser would blur to body then).
    const activeAtInert: Array<Element | null> = [];
    const setAttribute = w1Panel.setAttribute.bind(w1Panel);
    jest.spyOn(w1Panel, "setAttribute").mockImplementation((name, value) => {
      if (name === "inert") activeAtInert.push(document.activeElement);
      setAttribute(name, value);
    });
    rerender(<Harness windows={windows} current="w2" panelRef={panelRef} />);

    expect(activeAtInert).toEqual([document.getElementById("window-panel-w2")]);
    expect(document.activeElement).not.toBe(document.body);
    expect(panelRef.current?.id).toBe("window-panel-w2");
  });

  it("host_should_RenderConnectingOverlayInsidePanel_When_FlagOnAndNotWhenFlagOff", () => {
    const windows = ["w1", "w2"].map((id) => makeWindow(id));
    const { rerender } = render(<Harness windows={windows} current="w1" connecting />);
    expect(document.getElementById("window-panel-w1")).toContainElement(screen.getByTestId("connecting-w1"));
    rerender(<Harness windows={windows} current="w1" keepAlive={false} connecting />);
    expect(screen.queryByTestId("connecting-w1")).toBeNull();
  });

  it("activation_should_IssueNoResumeOrStartRpc_When_RetainedWindowHoldsArchivedSession", () => {
    rpcCalls.length = 0;
    const windows = [makeWindow("w1", "archived-session"), makeWindow("w2")];
    const { rerender } = render(<Harness windows={windows} current="w1" />);
    rerender(<Harness windows={windows} current="w2" />);
    rerender(<Harness windows={windows} current="w1" />); // reactivate the retained window holding the archived session
    expect(document.getElementById("window-panel-w1")).not.toHaveAttribute("hidden");
    expect(rpcCalls.filter((m) => /resume|start|restart|revive|unarchive|retry/i.test(m))).toEqual([]);
  });
});
