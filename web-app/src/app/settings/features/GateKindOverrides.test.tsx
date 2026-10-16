import React from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { GATE_OVERRIDE_KINDS, GateKindOverrides, modeOf } from "./GateKindOverrides";

jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({ createClient: () => ({}) }));

const noCounts = async () => ({});

// Renders and lets the first stats load settle inside act.
async function renderOverrides(ui: React.ReactElement) {
  await act(async () => {
    render(ui);
  });
}

describe("GateKindOverrides", () => {
  it("GateKindOverrides_should_OfferOnlyReachableKinds_When_Rendered", async () => {
    await renderOverrides(<GateKindOverrides onChange={jest.fn()} onReset={jest.fn()} fetchCounts={noCounts} pollMs={1e9} />);
    expect(GATE_OVERRIDE_KINDS).toEqual(["review", "diagnose", "other"]);
    expect(screen.queryByTestId("gate-override-triage")).not.toBeInTheDocument();
    expect(screen.getByTestId("gate-override-review")).toBeInTheDocument();
    expect(screen.getAllByRole("radiogroup")).toHaveLength(3);
  });

  it("GateKindOverrides_should_ShowServerReadbackAsInheritOnOff_When_ScopesGiven", async () => {
    await renderOverrides(
      <GateKindOverrides
        scopes={{ "kind:review": true, "kind:diagnose": false }}
        onChange={jest.fn()}
        onReset={jest.fn()}
        fetchCounts={noCounts}
        pollMs={1e9}
      />,
    );
    const checked = (kind: string) =>
      screen
        .getByTestId(`gate-override-${kind}`)
        .querySelector('[aria-checked="true"]')
        ?.textContent;
    expect(checked("review")).toBe("On");
    expect(checked("diagnose")).toBe("Off");
    expect(checked("other")).toBe("Inherit");
    expect(modeOf(undefined, "review")).toBe("inherit");
  });

  it("GateKindOverrides_should_ReportScopeAndMode_When_SegmentClickedAndNotWhenAlreadyActive", async () => {
    const onChange = jest.fn();
    await renderOverrides(
      <GateKindOverrides
        scopes={{ "kind:review": true }}
        onChange={onChange}
        onReset={jest.fn()}
        fetchCounts={noCounts}
        pollMs={1e9}
      />,
    );
    const row = screen.getByTestId("gate-override-review");
    fireEvent.click(row.querySelector('[aria-checked="true"]') as HTMLElement);
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getAllByRole("radio", { name: "Off" })[0]);
    expect(onChange).toHaveBeenCalledWith("kind:review", "off");
    fireEvent.click(screen.getAllByRole("radio", { name: "Inherit" })[0]);
    expect(onChange).toHaveBeenCalledWith("kind:review", "inherit");
  });

  it("GateKindOverrides_should_ShowTwentyFourHourCountsPerKind_When_StatsLoad", async () => {
    await renderOverrides(
      <GateKindOverrides
        onChange={jest.fn()}
        onReset={jest.fn()}
        fetchCounts={async () => ({ review: 12 })}
        pollMs={1e9}
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("gate-override-count-review").textContent).toBe(" (12 events in the last 24h)"),
    );
    expect(screen.getByTestId("gate-override-count-diagnose").textContent).toBe(" (0 events in the last 24h)");
  });

  it("GateKindOverrides_should_KeepOneTabStopAndMoveSelectionWithArrowKeys_When_RadioGroupFocused", async () => {
    const onChange = jest.fn();
    await renderOverrides(
      <GateKindOverrides scopes={{ "kind:review": true }} onChange={onChange} onReset={jest.fn()} fetchCounts={noCounts} pollMs={1e9} />,
    );
    const group = within(screen.getByTestId("gate-override-review"));
    const radios = group.getAllByRole("radio");
    expect(radios.map((r) => r.getAttribute("tabindex"))).toEqual(["-1", "0", "-1"]);

    fireEvent.keyDown(radios[1], { key: "ArrowRight" });
    expect(onChange).toHaveBeenCalledWith("kind:review", "off");
    expect(document.activeElement).toBe(radios[2]);

    fireEvent.keyDown(radios[1], { key: "ArrowLeft" });
    expect(onChange).toHaveBeenCalledWith("kind:review", "inherit");
  });

  it("GateKindOverrides_should_CallOnReset_When_ResetToDefaultClicked", async () => {
    const onReset = jest.fn();
    await renderOverrides(<GateKindOverrides onChange={jest.fn()} onReset={onReset} fetchCounts={noCounts} pollMs={1e9} />);
    fireEvent.click(screen.getByTestId("gate-reset-default"));
    expect(onReset).toHaveBeenCalledTimes(1);
  });
});
