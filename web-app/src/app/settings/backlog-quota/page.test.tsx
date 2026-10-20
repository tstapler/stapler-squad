import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import BacklogQuotaPage from "./page";
import { QuotaSource } from "@/gen/session/v1/quota_pb";

const getQuotaConfig = jest.fn();
const updateQuotaConfig = jest.fn();
let flagList: Array<{ name: string; statusDetail: string }> = [];

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({
    getQuotaConfig: (...a: unknown[]) => getQuotaConfig(...a),
    updateQuotaConfig: (...a: unknown[]) => updateQuotaConfig(...a),
  }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@/lib/analytics/usePageView", () => ({ usePageView: () => undefined }));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flagList }),
}));
jest.mock("next/navigation", () => ({ useRouter: () => ({ push: jest.fn(), replace: jest.fn() }) }));

interface Overrides {
  enabled?: boolean;
  budget?: bigint;
  pause?: number;
  resume?: number;
  usd?: number;
  source?: QuotaSource;
}

function response(o: Overrides = {}) {
  return {
    settings: {
      enabled: o.enabled ?? false,
      assumedWindowTokenBudget: o.budget ?? BigInt(0),
      pauseBelowHeadroomPct: o.pause ?? 20,
      resumeMarginPct: o.resume ?? 15,
      ...(o.usd === undefined ? {} : { defaultCostBudgetThresholdUsd: o.usd }),
    },
    quotaSource: o.source ?? QuotaSource.CONFIGURED,
  };
}

const field = (id: string) => screen.getByTestId(`backlog-quota-${id}`) as HTMLInputElement;
const type = (id: string, value: string) => fireEvent.change(field(id), { target: { value } });
const save = () => fireEvent.click(screen.getByTestId("backlog-quota-save"));

async function renderLoaded(o: Overrides = {}) {
  getQuotaConfig.mockResolvedValue(response(o));
  render(<BacklogQuotaPage />);
  await screen.findByTestId("backlog-quota-form");
}

beforeEach(() => {
  getQuotaConfig.mockReset();
  updateQuotaConfig.mockReset();
  flagList = [];
});

describe("loading and load failure", () => {
  it("is aria-busy with no fields while loading", async () => {
    getQuotaConfig.mockReturnValue(new Promise(() => undefined));
    render(<BacklogQuotaPage />);
    expect(screen.getByTestId("backlog-quota-load")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByTestId("backlog-quota-form")).toBeNull();
  });

  it("shows an alert with Retry and hides the form when the RPC fails", async () => {
    getQuotaConfig.mockRejectedValueOnce(new Error("boom"));
    render(<BacklogQuotaPage />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't load quota settings: boom.");
    expect(screen.queryByTestId("backlog-quota-form")).toBeNull();
    getQuotaConfig.mockResolvedValue(response());
    fireEvent.click(screen.getByTestId("backlog-quota-retry"));
    expect(await screen.findByTestId("backlog-quota-form")).toBeInTheDocument();
  });

  it("treats a response without quota_source as load-failed", async () => {
    getQuotaConfig.mockResolvedValue(response({ source: QuotaSource.UNSPECIFIED }));
    render(<BacklogQuotaPage />);
    expect(await screen.findByTestId("backlog-quota-load-error")).toBeInTheDocument();
    expect(screen.queryByTestId("backlog-quota-form")).toBeNull();
  });
});

describe("not-configured state", () => {
  it("shows the defaults sentence only for NOT_CONFIGURED", async () => {
    await renderLoaded({ source: QuotaSource.NOT_CONFIGURED });
    expect(screen.getByTestId("backlog-quota-not-configured")).toHaveTextContent(
      "Quota is not configured. Defaults apply: gate Off, no budget.",
    );
  });

  it("omits the sentence for CONFIGURED with default-equal values", async () => {
    await renderLoaded();
    expect(screen.queryByTestId("backlog-quota-not-configured")).toBeNull();
    expect(screen.getByTestId("backlog-quota-effective")).toHaveTextContent("Effective now: gate Off, no budget");
  });
});

describe("static warning and helper text", () => {
  it("shows the warning with no role and links it from the budget input when gate is on and budget 0", async () => {
    await renderLoaded({ enabled: true });
    const warning = screen.getByTestId("backlog-quota-warning");
    expect(warning).not.toHaveAttribute("role");
    expect(warning).toHaveTextContent("Gate is on but no budget is set, so the proactive signal never fires");
    expect(field("budget").getAttribute("aria-describedby")).toContain("backlog-quota-warning");
    expect(screen.getByText("0 = proactive headroom signal disabled")).toBeVisible();
  });

  it("announces the warning once through the status region when an edit causes it", async () => {
    await renderLoaded({ budget: BigInt(1000) });
    fireEvent.click(screen.getByTestId("backlog-quota-enabled"));
    expect(screen.queryByTestId("backlog-quota-warning")).toBeNull();
    type("budget", "0");
    expect(screen.getByTestId("backlog-quota-warning")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Gate is on but no budget is set");
  });
});

describe("input attributes", () => {
  it("uses numeric mode with pattern for tokens and percentages and decimal for USD", async () => {
    await renderLoaded();
    for (const id of ["budget", "pause", "resume"]) {
      expect(field(id)).toHaveAttribute("inputmode", "numeric");
      expect(field(id)).toHaveAttribute("pattern", "[0-9]*");
    }
    expect(field("usd")).toHaveAttribute("inputmode", "decimal");
  });

  it("sets enterKeyHint next on all but the last field, done on the last, and Enter moves forward", async () => {
    await renderLoaded();
    expect(["budget", "pause", "resume"].map((id) => field(id).getAttribute("enterkeyhint"))).toEqual(["next", "next", "next"]);
    expect(field("usd").getAttribute("enterkeyhint")).toBe("done");
    fireEvent.keyDown(field("budget"), { key: "Enter" });
    expect(field("pause")).toHaveFocus();
  });

  it("scrolls a focused field into view only when the on-screen keyboard shrinks the viewport", async () => {
    await renderLoaded();
    const scroll = jest.fn();
    field("pause").scrollIntoView = scroll;
    Object.defineProperty(window, "visualViewport", { configurable: true, value: { height: window.innerHeight } });
    fireEvent.focus(field("pause"));
    expect(scroll).not.toHaveBeenCalled();
    Object.defineProperty(window, "visualViewport", { configurable: true, value: { height: window.innerHeight - 300 } });
    fireEvent.focus(field("pause"));
    expect(scroll).toHaveBeenCalledWith({ block: "center" });
    Object.defineProperty(window, "visualViewport", { configurable: true, value: undefined });
  });
});

describe("validation", () => {
  it("rejects 0 for pause and resume with the default-replacement explanation", async () => {
    await renderLoaded();
    type("pause", "0");
    type("resume", "0");
    save();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/between 1 and 100; 0 would be replaced by the default \(20\)/);
    expect(alert).toHaveTextContent(/between 1 and 100; 0 would be replaced by the default \(15\)/);
    expect(updateQuotaConfig).not.toHaveBeenCalled();
    expect(screen.getAllByText("Range 1-100")).toHaveLength(2);
  });

  it("rejects pause+resume over 100, marks both invalid, keeps values and focus on a field", async () => {
    await renderLoaded();
    type("pause", "60");
    type("resume", "50");
    save();
    const msg = "Pause threshold plus resume margin must be at most 100";
    expect(await screen.findAllByText(msg)).not.toHaveLength(0);
    for (const id of ["pause", "resume"]) {
      expect(field(id)).toHaveAttribute("aria-invalid", "true");
      expect(field(id).getAttribute("aria-describedby")).toContain("backlog-quota-sum-error");
    }
    expect(field("pause").value).toBe("60");
    expect(field("resume").value).toBe("50");
    expect(field("pause")).toHaveFocus();
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });
});

describe("save, read-back and live region", () => {
  it("sends the entered values, shows Saving... then Applied, and has one status element", async () => {
    await renderLoaded();
    let resolve!: (v: unknown) => void;
    updateQuotaConfig.mockReturnValue(new Promise((r) => (resolve = r)));
    fireEvent.click(screen.getByTestId("backlog-quota-enabled"));
    type("budget", "5000000");
    type("usd", "5.5");
    expect(screen.getByRole("status")).toHaveTextContent("Unsaved changes");
    save();
    expect(screen.getByRole("status")).toHaveTextContent("Saving...");
    expect(screen.getByTestId("backlog-quota-save")).toHaveAttribute("aria-disabled", "true");
    const sent = updateQuotaConfig.mock.calls[0][0].settings;
    expect(sent.enabled).toBe(true);
    expect(sent.assumedWindowTokenBudget).toBe(BigInt(5000000));
    expect(sent.defaultCostBudgetThresholdUsd).toBe(5.5);
    await act(async () => resolve(response({ enabled: true, budget: BigInt(5000000), usd: 5.5 })));
    expect(screen.getByRole("status")).toHaveTextContent("Applied");
    expect(screen.getByTestId("backlog-quota-effective")).toHaveTextContent(
      "gate On, budget 5,000,000 tokens, warn at $5.50",
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("omits the USD field from the request when empty (off)", async () => {
    await renderLoaded({ usd: 3 });
    updateQuotaConfig.mockResolvedValue(response());
    type("usd", "");
    save();
    await waitFor(() => expect(updateQuotaConfig).toHaveBeenCalled());
    expect(updateQuotaConfig.mock.calls[0][0].settings.defaultCostBudgetThresholdUsd).toBeUndefined();
  });

  it("shows Backlog resumed when disabling a gate that had paused the backlog", async () => {
    flagList = [{ name: "backlog", statusDetail: "Paused: session-quota headroom below threshold." }];
    await renderLoaded({ enabled: true, budget: BigInt(10) });
    updateQuotaConfig.mockResolvedValue(response({ enabled: false, budget: BigInt(10) }));
    fireEvent.click(screen.getByTestId("backlog-quota-enabled"));
    save();
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Backlog resumed"));
  });

  it("shows a save-failed alert with Retry and keeps entered values", async () => {
    await renderLoaded();
    updateQuotaConfig.mockRejectedValueOnce(new Error("disk full"));
    type("budget", "42");
    save();
    expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't save: disk full");
    expect(field("budget").value).toBe("42");
    updateQuotaConfig.mockResolvedValue(response({ budget: BigInt(42) }));
    fireEvent.click(screen.getByText("Retry"));
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Applied"));
  });
});

describe("read-back mismatch", () => {
  async function mismatchAfterSave() {
    await renderLoaded();
    updateQuotaConfig.mockResolvedValue(response({ budget: BigInt(7) }));
    type("budget", "42");
    save();
    return screen.findByTestId("backlog-quota-mismatch");
  }

  it("replaces Applied with a focused alert listing entered vs effective", async () => {
    const mismatch = await mismatchAfterSave();
    expect(mismatch).toHaveFocus();
    expect(mismatch).toHaveTextContent("Entered 42, effective now 7");
    expect(screen.getByRole("status")).not.toHaveTextContent("Applied");
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });

  it("Reload effective values restores the read-back and focuses the first differing field", async () => {
    await mismatchAfterSave();
    fireEvent.click(screen.getByTestId("backlog-quota-mismatch-reload"));
    expect(field("budget").value).toBe("7");
    expect(field("budget")).toHaveFocus();
    expect(screen.getByRole("status")).toHaveTextContent("Reloaded effective values");
    expect(screen.queryByTestId("backlog-quota-mismatch")).toBeNull();
  });

  it("Save again re-submits the entered values", async () => {
    await mismatchAfterSave();
    fireEvent.click(screen.getByTestId("backlog-quota-mismatch-save-again"));
    await waitFor(() => expect(updateQuotaConfig).toHaveBeenCalledTimes(2));
    expect(updateQuotaConfig.mock.calls[1][0].settings.assumedWindowTokenBudget).toBe(BigInt(42));
  });
});

describe("unsaved changes", () => {
  it("shows Unsaved changes, enables Revert, and Revert restores the stored values", async () => {
    await renderLoaded();
    expect(screen.getByTestId("backlog-quota-revert")).toBeDisabled();
    type("budget", "9");
    expect(screen.getByRole("status")).toHaveTextContent("Unsaved changes");
    expect(screen.getByTestId("backlog-quota-revert")).toBeEnabled();
    fireEvent.click(screen.getByTestId("backlog-quota-revert"));
    expect(field("budget").value).toBe("0");
    expect(screen.getByRole("status")).toHaveTextContent("");
  });
});
