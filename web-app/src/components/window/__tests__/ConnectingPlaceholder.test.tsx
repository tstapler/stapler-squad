import { render, screen, act, fireEvent } from "@testing-library/react";
import { ConnectingPlaceholder } from "../ConnectingPlaceholder";
import { announce } from "@/lib/nav/announce";
import {
  clearTerminalConnectionStatus,
  setTerminalConnectionStatus,
} from "@/lib/terminal/terminalConnectionStatus";

jest.mock("@/lib/nav/announce", () => ({ announce: jest.fn() }));

const A = { id: "a", title: "api" };
const B = { id: "b", title: "web" };

beforeEach(() => {
  jest.useFakeTimers();
  (announce as jest.Mock).mockClear();
});
afterEach(() => {
  clearTerminalConnectionStatus("a");
  clearTerminalConnectionStatus("b");
  jest.useRealTimers();
});

describe("ConnectingPlaceholder", () => {
  it("placeholder_should_BeSilentAndShowTitle_When_PaneConnecting", () => {
    render(<ConnectingPlaceholder sessions={[A]} active />);
    expect(screen.getByText("Connecting api...")).toBeInTheDocument();
    const root = screen.getByTestId("connecting-placeholder");
    expect(root.closest("[aria-live],[role=status],[role=alert]")).toBeNull();
    act(() => void jest.advanceTimersByTime(999));
    expect(announce).not.toHaveBeenCalled();
  });

  it("placeholder_should_AnnounceStillConnectingOnce_When_PaneNotConnectedAfter1s", () => {
    render(<ConnectingPlaceholder sessions={[A]} active />);
    act(() => void jest.advanceTimersByTime(1000));
    act(() => void jest.advanceTimersByTime(5000));
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith("Still connecting api");
  });

  it("placeholder_should_SummarizeCount_When_MultiplePanesStillConnecting", () => {
    render(<ConnectingPlaceholder sessions={[A, B]} active />);
    act(() => void jest.advanceTimersByTime(1000));
    expect(announce).toHaveBeenCalledWith("Still connecting 2 sessions");
  });

  it("placeholder_should_NotAnnounceAndRenderNothing_When_AllConnectedBefore1s", () => {
    setTerminalConnectionStatus("a", "connected", jest.fn());
    const { container } = render(<ConnectingPlaceholder sessions={[A]} active />);
    act(() => void jest.advanceTimersByTime(2000));
    expect(announce).not.toHaveBeenCalled();
    expect(container).toBeEmptyDOMElement();
  });

  it("placeholder_should_NotAnnounce_When_WindowHidden", () => {
    render(<ConnectingPlaceholder sessions={[A]} active={false} />);
    act(() => void jest.advanceTimersByTime(2000));
    expect(announce).not.toHaveBeenCalled();
  });

  it("placeholder_should_ShowFailureAndRetry_When_TerminalFailed", () => {
    const retry = jest.fn();
    setTerminalConnectionStatus("a", "failed", retry);
    render(<ConnectingPlaceholder sessions={[A]} active />);
    expect(screen.getByText("Could not connect to api")).toBeInTheDocument();
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith("Could not connect to api");
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it("placeholder_should_UpdateToConnectedAndHide_When_StatusPublished", () => {
    render(<ConnectingPlaceholder sessions={[A]} active />);
    act(() => setTerminalConnectionStatus("a", "connected", jest.fn()));
    expect(screen.queryByTestId("connecting-placeholder")).toBeNull();
  });
});
