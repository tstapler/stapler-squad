import React from "react";
import { act, fireEvent, render, renderHook, screen } from "@testing-library/react";
import {
  ActionsDisabledReasonContext,
  OfflineBanner,
  PendingNotice,
  actionsDisabledReason,
  backoffDelayMs,
  useActionsDisabledReason,
  useAwaitingFresh,
} from "./OfflineBanner";
import { useOnlineStatus } from "@/lib/hooks/useOnlineStatus";

const advance = (ms: number) => act(() => void jest.advanceTimersByTime(ms));

describe("OfflineBanner", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("OfflineBanner_should_showLoadingOnlyAfter150ms_And_StillLoadingWithRetryAfter8s", () => {
    const onRetry = jest.fn();
    render(<OfflineBanner online loading hasData={false} onRetry={onRetry} />);
    advance(149);
    expect(screen.queryByText("Loading...")).toBeNull();
    advance(2);
    expect(screen.getByText("Loading...")).toBeInTheDocument();
    advance(8_000);
    expect(screen.getByText("Still loading.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("OfflineBanner_ShouldShowReconnectingThenCantReachWithRetryAfter30s_When_onlineButPollFails", () => {
    const onRetry = jest.fn();
    render(<OfflineBanner online failed onRetry={onRetry} />);
    expect(screen.getByText("Reconnecting to the server. Showing last known state.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    advance(30_000);
    expect(screen.getByText("Can't reach the server.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("OfflineBanner_should_autoRetryWithBackoff1sDoublingTo15s_While_reconnecting", () => {
    expect([0, 1, 2, 3, 4, 5].map(backoffDelayMs)).toEqual([1000, 2000, 4000, 8000, 15000, 15000]);
    const onRetry = jest.fn();
    render(<OfflineBanner online failed onRetry={onRetry} />);
    advance(1_000);
    expect(onRetry).toHaveBeenCalledTimes(1);
    advance(1_999);
    expect(onRetry).toHaveBeenCalledTimes(1);
    advance(1);
    expect(onRetry).toHaveBeenCalledTimes(2);
  });

  it("OfflineBanner_ShouldBeNonLiveText_And_PanelHasOneLiveRegion", () => {
    const { container, rerender } = render(<OfflineBanner online={false} />);
    expect(screen.getByTestId("offline-banner")).toHaveTextContent("Offline. Showing last known state.");
    rerender(<OfflineBanner online failed />);
    advance(30_000);
    expect(container.querySelectorAll("[role='status'], [role='alert'], [aria-live]")).toHaveLength(0);
  });

  it("OfflineBanner_should_leaveATypedDraftAndFocusUntouched_When_connectivityFlipsOfflineThenOnline", () => {
    const Host = () => {
      const online = useOnlineStatus();
      const [draft, setDraft] = React.useState("");
      return (
        <>
          <OfflineBanner online={online} />
          <textarea aria-label="Answer draft" value={draft} onChange={(e) => setDraft(e.target.value)} />
        </>
      );
    };
    const setOnline = (value: boolean) => {
      Object.defineProperty(window.navigator, "onLine", { configurable: true, value });
      act(() => void window.dispatchEvent(new Event(value ? "online" : "offline")));
    };
    render(<Host />);
    const draft = screen.getByLabelText("Answer draft");
    fireEvent.change(draft, { target: { value: "half-typed answer" } });
    act(() => draft.focus());

    setOnline(false);
    expect(screen.getByText("Offline. Showing last known state.")).toBeInTheDocument();
    expect(draft).toHaveValue("half-typed answer");
    expect(document.activeElement).toBe(draft);

    setOnline(true);
    expect(screen.queryByText("Offline. Showing last known state.")).toBeNull();
    expect(draft).toHaveValue("half-typed answer");
    expect(document.activeElement).toBe(draft);
  });

  it("OfflineBanner_should_renderNothing_When_onlineAndSettled", () => {
    render(<OfflineBanner online />);
    expect(screen.queryByTestId("offline-banner")).toBeNull();
  });

  it("OfflineBanner_ShouldShowStillSendingAt10sAndNoResponseCopyAt30s", () => {
    const onRetry = jest.fn();
    render(<PendingNotice pending onRetry={onRetry} />);
    advance(9_999);
    expect(screen.queryByTestId("pending-notice")).toBeNull();
    advance(1);
    expect(screen.getByTestId("pending-notice")).toHaveTextContent("Still sending...");
    expect(screen.queryByRole("button")).toBeNull();
    advance(20_000);
    expect(screen.getByTestId("pending-notice")).toHaveTextContent("No response from the server.");
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});

describe("actions-disabled contract", () => {
  it("actionsDisabledReason_should_rankOfflineThenReconnectingThenUpdating", () => {
    const base = { online: true, reconnecting: false, awaitingFresh: false };
    expect(actionsDisabledReason(base)).toBeUndefined();
    expect(actionsDisabledReason({ ...base, awaitingFresh: true })).toBe("Updating...");
    expect(actionsDisabledReason({ ...base, awaitingFresh: true, reconnecting: true })).toBe("Reconnecting");
    expect(actionsDisabledReason({ online: false, reconnecting: true, awaitingFresh: true })).toBe("Offline");
  });

  it("useActionsDisabledReason_should_readTheProvidedReason", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <ActionsDisabledReasonContext.Provider value="Offline">{children}</ActionsDisabledReasonContext.Provider>
    );
    expect(renderHook(() => useActionsDisabledReason(), { wrapper }).result.current).toBe("Offline");
    expect(renderHook(() => useActionsDisabledReason()).result.current).toBeUndefined();
  });

  it("useAwaitingFresh_should_holdUntilRefreshPassesOutageStart", () => {
    const { result, rerender } = renderHook(({ down, at }) => useAwaitingFresh(down, at), {
      initialProps: { down: false, at: 100 as number | null },
    });
    expect(result.current).toBe(false);
    rerender({ down: true, at: 100 });
    expect(result.current).toBe(false);
    rerender({ down: false, at: 100 });
    expect(result.current).toBe(true);
    rerender({ down: false, at: 200 });
    expect(result.current).toBe(false);
  });

  it("useAwaitingFresh_should_notWait_When_theRecoveryIsTheRefreshItself", () => {
    const { result, rerender } = renderHook(({ down, at }) => useAwaitingFresh(down, at), {
      initialProps: { down: true, at: 100 as number | null },
    });
    rerender({ down: false, at: 200 });
    expect(result.current).toBe(false);
  });
});
