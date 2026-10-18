/**
 * Story 3.2.3: FeaturesPage renders an optional second status-detail line
 * under a flag's description, driven by FeatureFlagMeta.statusDetail
 * (quota-aware-backlog-gating). No layout shift when statusDetail is empty.
 */

import React from "react";
import fs from "fs";
import path from "path";
import { render, screen, fireEvent, waitFor, within, act } from "@testing-library/react";
import FeaturesPage from "./page";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import type { FeatureFlagMeta } from "@/lib/contexts/FeatureFlagsContext";

jest.mock("@/lib/analytics", () => ({
  usePageView: () => {},
}));

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: jest.fn(),
}));

// Out of scope for this file (covered by StreamHubRolloutPanel.test.tsx /
// TymuxRolloutPanel.test.tsx) — stub them out so this suite doesn't also
// need to mock the RPC clients/hooks they call on mount.
jest.mock("@/components/settings/StreamHubRolloutPanel", () => ({
  StreamHubRolloutPanel: () => null,
}));
jest.mock("@/components/settings/TymuxRolloutPanel", () => ({
  TymuxRolloutPanel: () => null,
}));

// The status line has its own test (GateStatusLine.test.tsx); here only its placement matters.
jest.mock("./GateStatusLine", () => ({
  GateStatusLine: () => <div data-testid="gate-status-line" />,
}));

// The override disclosure reads GetDeliveryGateStats for its per-kind counts.
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getDeliveryGateStats: async () => ({ eventsByKind24h: {} }) }),
}));

const mockUseFeatureFlags = useFeatureFlags as jest.MockedFunction<typeof useFeatureFlags>;

function makeFlag(overrides: Partial<FeatureFlagMeta> & Pick<FeatureFlagMeta, "name">): FeatureFlagMeta {
  return {
    enabled: true,
    description: "",
    statusDetail: "",
    ...overrides,
  };
}

function mockFlags(flagList: FeatureFlagMeta[]) {
  mockUseFeatureFlags.mockReturnValue({
    flags: Object.fromEntries(flagList.map((f) => [f.name, f.enabled])),
    flagList,
    isLoading: false,
    error: null,
    loadError: null,
    reload: jest.fn(),
    setFlag: jest.fn(),
  });
}

// Same benign vanilla-extract jest-mock className warning as
// pipeline-modes/page.test.tsx — see that file's comment for details.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

describe("FeaturesPage", () => {
  it("FeaturesPage_should_RenderSecondLine_When_StatusDetailNonEmpty", () => {
    mockFlags([
      makeFlag({
        name: "backlog",
        enabled: false,
        description: "Backlog management with external sync sources and AI-driven triage",
        statusDetail: "Paused: session-quota headroom below threshold (15% remaining; threshold 20%).",
      }),
    ]);

    render(<FeaturesPage />);

    expect(
      screen.getByText("Paused: session-quota headroom below threshold (15% remaining; threshold 20%).")
    ).toBeInTheDocument();
  });

  it("FeaturesPage_should_RenderNoExtraElement_When_StatusDetailEmpty", () => {
    mockFlags([
      makeFlag({
        name: "backlog",
        enabled: true,
        description: "Backlog management with external sync sources and AI-driven triage",
        statusDetail: "",
      }),
    ]);

    const { container } = render(<FeaturesPage />);

    // Exactly one description-styled line (the flag description) — no second
    // empty line/paragraph rendered for a "" statusDetail.
    const descriptionLine = screen.getByText(
      "Backlog management with external sync sources and AI-driven triage"
    );
    // flagName + description + the (empty) row status region; no extra detail line
    expect(descriptionLine.parentElement?.children.length).toBe(3);
    expect(screen.getByTestId("feature-flag-row-status")).toBeEmptyDOMElement();
    expect(container.querySelectorAll("div").length).toBeGreaterThan(0);
  });
});

// Story 2.1.2: disabling pi-support is gated on a mandatory warning modal only
// when the pi approval extension is actually installed on disk.
describe("FeaturesPage — pi-support disable warning", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    jest.restoreAllMocks();
  });

  it("should show the mandatory warning and block persistence until acknowledged, when the extension is installed", async () => {
    const setFlag = jest.fn();
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ installed: true }),
    }) as unknown as typeof fetch;

    mockUseFeatureFlags.mockReturnValue({
      flags: { "pi-support": true },
      flagList: [makeFlag({ name: "pi-support", enabled: true, description: "pi coding agent support" })],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("switch", { name: /pi coding agent/i }));

    // Persistence must not happen until the user explicitly acknowledges.
    await waitFor(() => expect(screen.getByTestId("pi-disable-warning-overlay")).toBeInTheDocument());
    expect(setFlag).not.toHaveBeenCalled();
    expect(screen.getByTestId("pi-disable-warning-body").textContent).toContain(
      "Disabling pi-support does NOT remove the pi approval extension"
    );
    expect(screen.getByTestId("pi-disable-warning-body").textContent).toContain(
      "ssq-hooks install pi --uninstall"
    );

    fireEvent.click(screen.getByTestId("pi-disable-warning-acknowledge"));

    expect(setFlag).toHaveBeenCalledWith("pi-support", false);
    expect(screen.queryByTestId("pi-disable-warning-overlay")).not.toBeInTheDocument();
  });

  it("should show the mandatory warning (fail closed) when the status check returns a non-2xx response", async () => {
    const setFlag = jest.fn();
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 500,
      json: async () => ({}),
    }) as unknown as typeof fetch;

    mockUseFeatureFlags.mockReturnValue({
      flags: { "pi-support": true },
      flagList: [makeFlag({ name: "pi-support", enabled: true, description: "pi coding agent support" })],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("switch", { name: /pi coding agent/i }));

    await waitFor(() => expect(screen.getByTestId("pi-disable-warning-overlay")).toBeInTheDocument());
    expect(setFlag).not.toHaveBeenCalled();
  });

  it("should never persist the toggle when Cancel is clicked instead of I understand", async () => {
    const setFlag = jest.fn();
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ installed: true }),
    }) as unknown as typeof fetch;

    mockUseFeatureFlags.mockReturnValue({
      flags: { "pi-support": true },
      flagList: [makeFlag({ name: "pi-support", enabled: true, description: "pi coding agent support" })],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("switch", { name: /pi coding agent/i }));
    await waitFor(() => expect(screen.getByTestId("pi-disable-warning-overlay")).toBeInTheDocument());

    fireEvent.click(screen.getByTestId("pi-disable-warning-cancel"));

    expect(setFlag).not.toHaveBeenCalled();
    expect(screen.queryByTestId("pi-disable-warning-overlay")).not.toBeInTheDocument();
  });

  it("should persist immediately with no modal, when the extension is not installed", async () => {
    const setFlag = jest.fn();
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ installed: false }),
    }) as unknown as typeof fetch;

    mockUseFeatureFlags.mockReturnValue({
      flags: { "pi-support": true },
      flagList: [makeFlag({ name: "pi-support", enabled: true, description: "pi coding agent support" })],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("switch", { name: /pi coding agent/i }));

    await waitFor(() => expect(setFlag).toHaveBeenCalledWith("pi-support", false));
    expect(screen.queryByTestId("pi-disable-warning-overlay")).not.toBeInTheDocument();
  });

  it("should toggle other flags immediately with no extension check at all", () => {
    const setFlag = jest.fn();
    global.fetch = jest.fn();

    mockUseFeatureFlags.mockReturnValue({
      flags: { backlog: false },
      flagList: [makeFlag({ name: "backlog", enabled: false, description: "Backlog management" })],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("switch", { name: "Backlog" }));

    expect(setFlag).toHaveBeenCalledWith("backlog", true);
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("should enable pi-support immediately with no extension check, since only disabling is gated", () => {
    const setFlag = jest.fn();
    global.fetch = jest.fn();

    mockUseFeatureFlags.mockReturnValue({
      flags: { "pi-support": false },
      flagList: [makeFlag({ name: "pi-support", enabled: false, description: "pi coding agent support" })],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("switch", { name: /pi coding agent/i }));

    expect(setFlag).toHaveBeenCalledWith("pi-support", true);
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("FeaturesPage_should_RenderGateStatusLineOnlyUnderHiddenSessionGate", () => {
    mockFlags([
      makeFlag({ name: "backlog" }),
      makeFlag({ name: "hidden_session_gate", enabled: false }),
    ]);

    render(<FeaturesPage />);

    const lines = screen.getAllByTestId("gate-status-line");
    expect(lines).toHaveLength(1);
    const row = lines[0].closest('[data-testid="feature-flag-row"]');
    expect(row?.textContent).toContain("hidden-session delivery gate");
  });
});

// Story 2.11: per-kind overrides and the explicit global scope.
describe("FeaturesPage - hidden_session_gate overrides", () => {
  function setup(scopes?: Record<string, boolean>, enabled = false) {
    const setFlag = jest.fn();
    mockUseFeatureFlags.mockReturnValue({
      flags: { hidden_session_gate: enabled },
      flagList: [makeFlag({ name: "hidden_session_gate", enabled, description: "gate", scopes })],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag,
    });
    render(<FeaturesPage />);
    return setFlag;
  }

  it("features_page_should_send_the_global_literal_scope_when_the_gate_toggle_is_used", () => {
    const setFlag = setup();
    fireEvent.click(screen.getByRole("switch", { name: /hidden-session delivery gate/i }));
    expect(setFlag).toHaveBeenCalledWith("hidden_session_gate", { mutation: "set", scope: "global", enabled: true });
  });

  it("features_page_should_send_set_clear_and_reset_when_override_controls_are_used", () => {
    const setFlag = setup({ "kind:review": true });
    const review = screen.getByTestId("gate-override-review");
    fireEvent.click(review.querySelector('[role="radio"][aria-checked="false"]') as HTMLElement); // Inherit
    expect(setFlag).toHaveBeenLastCalledWith("hidden_session_gate", { mutation: "clear", scope: "kind:review" });
    fireEvent.click(screen.getAllByRole("radio", { name: "On" })[1]); // diagnose
    expect(setFlag).toHaveBeenLastCalledWith("hidden_session_gate", {
      mutation: "set",
      scope: "kind:diagnose",
      enabled: true,
    });
    fireEvent.click(screen.getByTestId("gate-reset-default"));
    expect(setFlag).toHaveBeenLastCalledWith("hidden_session_gate", { mutation: "reset" });
  });

  it("features_page_should_show_server_readback_not_optimistic_state_when_save_fails", () => {
    const setFlag = setup({ "kind:review": true });
    fireEvent.click(screen.getAllByRole("radio", { name: "Off" })[0]);
    expect(setFlag).toHaveBeenCalled();
    // setFlag is a no-op stub (a failed save): the control still shows the read-back value.
    const checked = screen.getByTestId("gate-override-review").querySelector('[aria-checked="true"]');
    expect(checked?.textContent).toBe("On");
  });

  it("features_page_should_show_no_override_controls_for_other_flags", () => {
    mockFlags([makeFlag({ name: "backlog" })]);
    render(<FeaturesPage />);
    expect(screen.queryByTestId("gate-kind-overrides")).not.toBeInTheDocument();
  });
});

// FG-4: the server's status_detail distinguishes default-off (shadow mode) from an explicit global off.
describe("FeaturesPage - hidden_session_gate status line (FG-4)", () => {
  const SHADOW = "Shadow mode: hidden sessions still notify; would-suppress counts are logged";
  const OFF = "Gate is OFF: hidden sessions deliver everything";

  function gateRow() {
    return screen.getAllByTestId("feature-flag-row")[0];
  }

  it("fg4_should_show_the_shadow_mode_line_when_the_flag_is_off_by_default", () => {
    mockFlags([makeFlag({ name: "hidden_session_gate", enabled: false, statusDetail: SHADOW })]);
    render(<FeaturesPage />);
    expect(within(gateRow()).getByText(SHADOW)).toBeInTheDocument();
    expect(within(gateRow()).queryByText(OFF)).not.toBeInTheDocument();
  });

  it("fg4_should_show_the_gate_is_off_line_instead_when_the_global_value_is_an_explicit_false", () => {
    mockFlags([makeFlag({ name: "hidden_session_gate", enabled: false, statusDetail: OFF })]);
    render(<FeaturesPage />);
    expect(within(gateRow()).getByText(OFF)).toBeInTheDocument();
    expect(within(gateRow()).queryByText(SHADOW)).not.toBeInTheDocument();
  });

  it("fg4_should_show_neither_line_when_the_gate_is_on", () => {
    mockFlags([makeFlag({ name: "hidden_session_gate", enabled: true, statusDetail: "" })]);
    render(<FeaturesPage />);
    expect(within(gateRow()).queryByText(SHADOW)).not.toBeInTheDocument();
    expect(within(gateRow()).queryByText(OFF)).not.toBeInTheDocument();
  });
});

// Story 3.10 / 2.11 UX criteria FG-1, FG-5, FG-6.
describe("FeaturesPage - notification flag rows (FG-1, FG-5, FG-6)", () => {
  it("fg1_should_show_the_gate_as_a_toggle_with_a_description_and_takes_effect_without_reload", () => {
    mockFlags([
      makeFlag({ name: "hidden_session_gate", enabled: false, description: "Quiet hidden sessions" }),
      makeFlag({ name: "backlog", enabled: true, description: "Backlog" }),
    ]);
    render(<FeaturesPage />);

    expect(screen.getByRole("switch", { name: /hidden-session delivery gate/i })).toHaveAttribute("aria-checked");
    const rows = screen.getAllByTestId("feature-flag-row");
    const noteCount = rows.filter((row) => row.querySelector('[data-testid="flag-live-note"]')).length;
    expect(noteCount).toBe(1);
    expect(screen.getAllByTestId("flag-live-note")[0]).toHaveTextContent("Takes effect without reload");
    expect(screen.getByText("Quiet hidden sessions")).toBeInTheDocument();
  });

  it("fg5_should_offer_the_session_kind_disclosure_only_under_the_gate_with_44px_labelled_controls", () => {
    mockFlags([
      makeFlag({ name: "backlog", enabled: true }),
      makeFlag({ name: "hidden_session_gate", enabled: true }),
    ]);
    render(<FeaturesPage />);

    const disclosures = screen.getAllByTestId("gate-kind-overrides");
    expect(disclosures).toHaveLength(1);
    expect(disclosures[0].closest('[data-testid="feature-flag-row"]')?.textContent).toContain("hidden-session delivery gate");
    expect(disclosures[0]).toHaveTextContent("Overrides by session kind");
    for (const group of screen.getAllByRole("radiogroup")) expect(group).toHaveAccessibleName(/Gate override for \w+ sessions/);

    const css = fs.readFileSync(path.join(__dirname, "page.css.ts"), "utf8");
    for (const name of ["overrideSegment", "resetButton"]) {
      const block = css.slice(css.indexOf(`export const ${name} = style(`));
      expect(block.slice(0, block.indexOf("});"))).toMatch(/minHeight: "2\.75rem"/);
    }
  });

  it("fg6_should_state_the_effective_value_and_its_source_and_inherit_should_clear_the_override", () => {
    const setFlag = jest.fn();
    mockUseFeatureFlags.mockReturnValue({
      flags: { hidden_session_gate: true },
      flagList: [makeFlag({ name: "hidden_session_gate", enabled: true, scopes: { "kind:review": false } })],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag,
    });
    render(<FeaturesPage />);

    expect(screen.getByTestId("gate-override-effective-review")).toHaveTextContent("Off - override");
    expect(screen.getByTestId("gate-override-effective-diagnose")).toHaveTextContent("On - from global");

    fireEvent.click(within(screen.getByTestId("gate-override-review")).getByRole("radio", { name: "Inherit" }));
    expect(setFlag).toHaveBeenLastCalledWith("hidden_session_gate", { mutation: "clear", scope: "kind:review" });
  });
});

// Story 4.1.4: legibility (UX-11, UX-12), switch semantics (UX-09, UX-10), states (UX-13, UX-14, UX-17, UX-48).
describe("FeaturesPage - Story 4.1.4", () => {
  const STALL = "autonomous_stall_nudge_enabled";
  const DIAGNOSE = "diagnose_nudge_enabled";

  function mockState(overrides: Partial<ReturnType<typeof useFeatureFlags>>) {
    mockUseFeatureFlags.mockReturnValue({
      flags: {},
      flagList: [],
      isLoading: false,
      error: null,
      loadError: null,
      reload: jest.fn(),
      setFlag: jest.fn(),
      ...overrides,
    });
  }

  it("groups autonomous flags and shows On (inactive), default source and human labels", () => {
    mockFlags([
      makeFlag({ name: "backlog" }),
      makeFlag({
        name: DIAGNOSE,
        enabled: false,
        statusDetail: "Default: Off (not set explicitly)",
      }),
      makeFlag({
        name: STALL,
        enabled: true,
        statusDetail: `inactive: requires ${DIAGNOSE}; Set explicitly: On`,
      }),
    ]);
    render(<FeaturesPage />);

    const group = screen.getByRole("region", { name: "Autonomous backlog" });
    expect(within(group).getByRole("heading", { name: "Autonomous backlog" })).toBeInTheDocument();
    // human labels, not raw keys; the stall row is nested right after the diagnose row
    const rows = within(group).getAllByTestId("feature-flag-row");
    expect(rows[0]).toHaveTextContent("Diagnose & Nudge");
    expect(rows[1]).toHaveTextContent("Autonomous stall nudge");
    expect(rows[1].className).toMatch(/nestedRow/);
    expect(screen.queryByText(DIAGNOSE)).not.toBeInTheDocument();

    expect(within(rows[1]).getByTestId("feature-flag-status")).toHaveTextContent("On (inactive)");
    expect(within(rows[1]).getByText("Has no effect until Diagnose & Nudge is On.")).toBeInTheDocument();
    expect(within(rows[1]).getByTestId("feature-flag-source")).toHaveTextContent("Set explicitly: On");
    expect(within(rows[0]).getByTestId("feature-flag-source")).toHaveTextContent("Default: Off (not set explicitly)");
    // inactive is not rendered as a free-text status line
    expect(within(rows[1]).queryByText(/requires diagnose_nudge_enabled/)).not.toBeInTheDocument();
    // an inert row's switch stays operable
    expect(within(rows[1]).getByRole("switch")).not.toHaveAttribute("aria-disabled");
  });

  it("rows should show default or explicit source rollback line and live note When rendered", () => {
    mockFlags([
      makeFlag({ name: "hidden_session_gate", enabled: false, statusDetail: "Default: Off (not set explicitly)" }),
      makeFlag({ name: "backlog", statusDetail: "Set explicitly: On" }),
    ]);
    render(<FeaturesPage />);
    const [gate, backlog] = screen.getAllByTestId("feature-flag-row");
    expect(within(gate).getByTestId("feature-flag-source")).toHaveTextContent("Default: Off (not set explicitly)");
    expect(within(gate).getByTestId("flag-live-note")).toHaveTextContent("Takes effect without reload");
    expect(within(backlog).getByTestId("feature-flag-source")).toHaveTextContent("Set explicitly: On");
  });

  it("flagExtras registry renders per-flag extras without name-branching", () => {
    mockFlags([makeFlag({ name: "hidden_session_gate" }), makeFlag({ name: "backlog" })]);
    render(<FeaturesPage />);
    const [gate, backlog] = screen.getAllByTestId("feature-flag-row");
    expect(within(gate).getByTestId("gate-status-line")).toBeInTheDocument();
    expect(within(gate).getByTestId("gate-kind-overrides")).toBeInTheDocument();
    expect(within(backlog).queryByTestId("gate-status-line")).not.toBeInTheDocument();
    expect(fs.readFileSync(path.join(__dirname, "page.tsx"), "utf8")).not.toMatch(/name === ["'A-Z_]/);
  });

  it("every flag control exposes switch semantics with a stable label, a valid description id and no aria-pressed", () => {
    const setFlag = jest.fn().mockResolvedValue({ ok: true });
    mockState({
      flagList: [makeFlag({ name: "backlog", enabled: false, description: "Backlog management" })],
      setFlag,
    });
    const { rerender } = render(<FeaturesPage />);
    const off = screen.getByRole("switch", { name: "Backlog" });
    expect(off).toHaveAttribute("aria-checked", "false");
    const describedBy = off.getAttribute("aria-describedby") ?? "";
    expect(describedBy).not.toBe("");
    expect(document.getElementById(describedBy)).toHaveTextContent("Backlog management");

    mockState({ flagList: [makeFlag({ name: "backlog", enabled: true, description: "Backlog management" })], setFlag });
    rerender(<FeaturesPage />);
    const on = screen.getByRole("switch", { name: "Backlog" });
    expect(on).toHaveAttribute("aria-checked", "true");
    expect(document.querySelector("[aria-pressed]")).toBeNull();

    const css = fs.readFileSync(path.join(__dirname, "page.css.ts"), "utf8");
    const block = css.slice(css.indexOf("export const toggle = style("));
    const toggleCss = block.slice(0, block.indexOf("export const toggleTrack"));
    expect(toggleCss).toMatch(/width: "44px"/);
    expect(toggleCss).toMatch(/height: "44px"/);
    expect(toggleCss).toMatch(/":focus-visible"/);
  });

  it("should render aria-busy skeleton and no switches When flags loading", () => {
    mockState({ isLoading: true });
    render(<FeaturesPage />);
    expect(screen.getByTestId("feature-flags-loading")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryAllByRole("switch")).toHaveLength(0);
  });

  it("shows a load-failed alert with Retry and renders no switch When the list request fails", () => {
    const reload = jest.fn();
    mockState({
      loadError: "unavailable",
      reload,
      flagList: [makeFlag({ name: "backlog", enabled: false })],
    });
    render(<FeaturesPage />);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Couldn't load feature flags: unavailable");
    expect(screen.queryAllByRole("switch")).toHaveLength(0);
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("announces the result in the row status, keeps focus on the switch, and on failure shows an alert with Retry", async () => {
    const setFlag = jest
      .fn()
      .mockResolvedValueOnce({ ok: false, error: "boom" })
      .mockResolvedValueOnce({ ok: true });
    mockState({ flagList: [makeFlag({ name: "backlog", enabled: false })], setFlag });
    render(<FeaturesPage />);
    const sw = screen.getByRole("switch", { name: "Backlog" });
    sw.focus();
    fireEvent.click(sw);

    const row = screen.getAllByTestId("feature-flag-row")[0];
    const alert = await within(row).findByRole("alert");
    expect(alert).toHaveTextContent("Couldn't change Backlog: boom");
    expect(sw).toHaveFocus();
    expect(sw).toHaveAttribute("aria-checked", "false"); // reverts to the server value

    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(within(row).getByRole("status")).toHaveTextContent("Backlog is now On."));
    expect(within(row).queryByRole("alert")).not.toBeInTheDocument();
    expect(setFlag).toHaveBeenCalledTimes(2);
    expect(sw).toHaveFocus();
  });

  it("ignores a second activation while a toggle is pending, keeps focus, and announces Saving once", async () => {
    let finish: (v: { ok: true }) => void = () => {};
    const setFlag = jest.fn().mockReturnValue(new Promise((resolve) => (finish = resolve)));
    mockState({ flagList: [makeFlag({ name: "backlog", enabled: false })], setFlag });
    render(<FeaturesPage />);
    const sw = screen.getByRole("switch", { name: "Backlog" });
    sw.focus();
    fireEvent.click(sw);
    fireEvent.click(sw);

    await waitFor(() => expect(sw).toHaveAttribute("aria-disabled", "true"));
    fireEvent.click(sw);
    expect(setFlag).toHaveBeenCalledTimes(1);
    expect(sw).toHaveFocus();
    const status = screen.getByTestId("feature-flag-row-status");
    expect(status).toHaveTextContent("Saving Backlog...");

    await act(async () => finish({ ok: true }));
    expect(status).toHaveTextContent("Backlog is now On.");
    expect(sw).not.toHaveAttribute("aria-disabled");
    expect(sw).toHaveFocus();
  });
});
