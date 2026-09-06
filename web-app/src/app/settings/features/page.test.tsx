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
// need to mock the RPC clients they call on mount.
jest.mock("@/components/settings/StreamHubRolloutPanel", () => ({
  StreamHubRolloutPanel: () => null,
}));
jest.mock("@/components/settings/TymuxRolloutPanel", () => ({
  TymuxRolloutPanel: () => null,
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
});
