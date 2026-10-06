import { act, fireEvent, render, screen } from "@testing-library/react";
import { ProbeStatusBadge } from "./ProbeStatusBadge";
import type { ProbeUiState } from "@/lib/hooks/useProbeProgram";
import { probeStates } from "@/lib/hooks/__mocks__/probeProgramMock";

const onRetry = jest.fn();
const onConfirm = jest.fn();

function renderBadge(state: ProbeUiState, token = "claude") {
  return render(
    <ProbeStatusBadge state={state} checkedToken={token} onRetry={onRetry} onConfirm={onConfirm} testId="badge" />,
  );
}

const statusText = () => screen.getByRole("status").textContent ?? "";

beforeEach(() => jest.clearAllMocks());

describe("ProbeStatusBadge", () => {
  it("ProbeStatusBadge_should_RenderFoundPathAndCheckedOnServerOnly_When_Found", () => {
    renderBadge(probeStates.found);
    expect(screen.getByRole("status")).toHaveTextContent("Found: /usr/bin/claude");
    expect(screen.getByRole("status")).toHaveTextContent("14 flags detected");
    expect(screen.getByTestId("badge")).toHaveTextContent("Checked on this server only");
    expect(screen.getByTestId("badge").querySelector('svg[data-icon="check"]')).toHaveAttribute("aria-hidden", "true");
    fireEvent.click(screen.getByRole("button", { name: "Show full path" }));
    expect(screen.getByRole("button", { name: "Hide full path" })).toBeInTheDocument();
  });

  it("ProbeStatusBadge_should_RenderNoFlagsTimeoutAndWrapperVariants_When_Given", () => {
    const { rerender } = renderBadge(probeStates.noFlags);
    const props = { checkedToken: "npx", onRetry, onConfirm };
    expect(statusText()).toContain("Couldn't read flags from --help; flag suggestions unavailable.");
    rerender(<ProbeStatusBadge state={probeStates.timeout} {...props} />);
    expect(statusText()).toContain("Found: /usr/bin/claude. Timed out reading flags — try Check again.");
    expect(statusText()).not.toContain("Couldn't read flags");
    rerender(<ProbeStatusBadge state={probeStates.wrapper} {...props} />);
    expect(statusText()).toBe("Wrapper command (npx): flags for the wrapped program are not checked.");
  });

  it("ProbeStatusBadge_should_RenderNotCheckedYetCopyAndCheckButton_When_NeedsConfirm", () => {
    const props = { checkedToken: "claude", onRetry, onConfirm };
    const { rerender } = renderBadge(probeStates.needsConfirm);
    expect(statusText()).toBe(
      "Found: /usr/bin/claude. Not checked for flags yet. Check runs `claude --help` on this server.",
    );
    expect(screen.getByTestId("badge").textContent).not.toMatch(/script|Not run yet/i);
    const check = screen.getByRole("button", { name: "Check" });
    fireEvent.click(check);
    expect(onConfirm).toHaveBeenCalledTimes(1);

    // Stays mounted (same node), disabled + aria-busy while checking, relabelled after.
    rerender(<ProbeStatusBadge state={probeStates.checking} {...props} />);
    const busy = screen.getByRole("button", { name: /Check/ });
    expect(busy).toBe(check);
    expect(busy).toBeDisabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
    rerender(<ProbeStatusBadge state={probeStates.found} {...props} />);
    expect(screen.getByRole("button", { name: "Check again" })).toBe(check);
    expect(check).toBeEnabled();
  });

  it("ProbeStatusBadge_should_RenderAliasAwareNotFoundCopy_When_NotFound", () => {
    renderBadge(probeStates.notFound);
    expect(statusText()).toMatch(/PATH/);
    expect(statusText()).toMatch(/aliases and functions are not checked/);
  });

  it("ProbeStatusBadge_should_RenderCouldntCheckWithRetry_When_TransportOrBusyAndNeverNotFound", () => {
    const props = { checkedToken: "claude", onRetry, onConfirm };
    const { rerender } = renderBadge(probeStates.busyOrError);
    for (const state of [probeStates.busyOrError, probeStates.transportError]) {
      rerender(<ProbeStatusBadge state={state} {...props} />);
      expect(statusText()).toMatch(/^Couldn't check right now\./);
      expect(statusText()).not.toMatch(/not found/i);
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    }
    expect(onRetry).toHaveBeenCalledTimes(2);
    expect(statusText()).toMatch(/non-loopback/);
  });

  it("ProbeStatusBadge_should_RenderNonEmptyDistinctText_When_EachUiStateVariant", () => {
    const seen = new Set<string>();
    for (const [name, state] of Object.entries(probeStates)) {
      const { container, unmount } = renderBadge(state);
      if (name === "idle" || name === "disabled") {
        expect(container).toBeEmptyDOMElement();
      } else {
        const text = statusText();
        expect(text.length).toBeGreaterThan(0);
        if (name === "busyOrError" || name === "transportError") {
          expect(text).toMatch(/^Couldn't check right now\./);
        }
        seen.add(name === "busyOrError" || name === "transportError" ? "couldnt" : text);
      }
      unmount();
    }
    expect(seen.size).toBe(8);
  });

  it("ProbeStatusBadge_should_NotContainNotFoundText_When_TransportError", () => {
    renderBadge(probeStates.transportError);
    expect(screen.getByTestId("badge").textContent).not.toMatch(/not found/i);
  });

  it("ProbeStatusBadge_should_UseDistinctAriaHiddenIcons_When_SuccessWarningNeutral", () => {
    const ids = new Set<string>();
    for (const key of ["found", "notFound", "busyOrError"]) {
      const { container, unmount } = renderBadge(probeStates[key]);
      const icon = container.querySelector("svg[data-icon]");
      expect(icon).toHaveAttribute("aria-hidden", "true");
      ids.add(icon?.getAttribute("data-icon") ?? "");
      unmount();
    }
    expect([...ids].sort()).toEqual(["check", "info", "warning"]);
  });

  it("ProbeStatusBadge_should_AvoidBannedWords_When_WarningToneVariants", () => {
    for (const key of ["notFound", "transportError"]) {
      const { unmount } = renderBadge(probeStates[key]);
      expect(statusText()).not.toMatch(/invalid|error|failed/i);
      unmount();
    }
  });

  it("ProbeStatusBadge_should_PairColorWithIconAndText_When_AllTones", () => {
    for (const key of ["found", "noFlags", "needsConfirm", "wrapper", "notFound", "busyOrError", "transportError"]) {
      const { container, unmount } = renderBadge(probeStates[key]);
      expect(container.querySelector("svg[data-icon]")).not.toBeNull();
      expect(statusText().trim()).not.toBe("");
      unmount();
    }
  });

  it("ProbeStatusBadge_should_ShowStaticSpinnerAndCheckingText_When_ReducedMotion", () => {
    const original = window.matchMedia;
    window.matchMedia = jest.fn().mockReturnValue({ matches: true }) as unknown as typeof window.matchMedia;
    try {
      const reduced = renderBadge(probeStates.checking);
      expect(reduced.container.querySelector('[data-icon="spinner"]')).toHaveAttribute("data-static", "true");
      expect(screen.getByText("Checking...")).toBeInTheDocument();
      reduced.unmount();
      window.matchMedia = jest.fn().mockReturnValue({ matches: false }) as unknown as typeof window.matchMedia;
      const animated = renderBadge(probeStates.checking);
      expect(animated.container.querySelector('[data-icon="spinner"]')).toHaveAttribute("data-static", "false");
    } finally {
      window.matchMedia = original;
    }
  });

  it("ProbeStatusBadge_should_AnnounceCheckingOnlyAfterDelay_When_RunLastsOver300ms", () => {
    jest.useFakeTimers();
    try {
      renderBadge(probeStates.checking);
      expect(screen.getByText("Checking...")).toHaveAttribute("aria-hidden", "true");
      act(() => void jest.advanceTimersByTime(301));
      expect(screen.getByRole("status")).toHaveTextContent("Checking...");
    } finally {
      jest.useRealTimers();
    }
  });
});
