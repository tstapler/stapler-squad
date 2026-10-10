/**
 * Story 3.2.3: FeaturesPage renders an optional second status-detail line
 * under a flag's description, driven by FeatureFlagMeta.statusDetail
 * (quota-aware-backlog-gating). No layout shift when statusDetail is empty.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
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
    expect(descriptionLine.parentElement?.children.length).toBe(2); // flagName + description only
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
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("button", { name: /disable pi coding agent/i }));

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
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("button", { name: /disable pi coding agent/i }));

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
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("button", { name: /disable pi coding agent/i }));
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
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("button", { name: /disable pi coding agent/i }));

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
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("button", { name: /enable backlog/i }));

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
      setFlag,
    });

    render(<FeaturesPage />);

    fireEvent.click(screen.getByRole("button", { name: /enable pi coding agent/i }));

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
      setFlag,
    });
    render(<FeaturesPage />);
    return setFlag;
  }

  it("features_page_should_send_the_global_literal_scope_when_the_gate_toggle_is_used", () => {
    const setFlag = setup();
    fireEvent.click(screen.getByRole("button", { name: /enable notifications: hidden-session delivery gate/i }));
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
