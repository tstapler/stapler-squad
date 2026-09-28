/**
 * Tests for DiagnoseOutcomeDisplay (plan.md Story 8.2.1): all 7
 * DiagnoseOutcomeKind-driven states + Pending + Stalled render distinct
 * copy/icon/testid, the flag-off banner shows proactively, DispatchFailed's
 * Retry re-invokes onDiagnose, and Stalled renders distinctly from both
 * Pending and DispatchFailed.
 */

import { render, screen, act } from "@testing-library/react";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import type { DiagnoseDispatchProto } from "@/gen/session/v1/diagnose_pb";
import { DiagnoseDispatchStatus } from "@/gen/session/v1/diagnose_pb";

const mockListDiagnoseDispatches = jest.fn();
const mockUseFeatureFlag = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    listDiagnoseDispatches: (...args: unknown[]) => mockListDiagnoseDispatches(...args),
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (...args: unknown[]) => mockUseFeatureFlag(...args),
}));

import { DiagnoseOutcomeDisplay } from "./DiagnoseOutcomeDisplay";

function makeTimestamp(secondsAgo: number): Timestamp {
  return { seconds: BigInt(Math.floor(Date.now() / 1000) - secondsAgo), nanos: 0 } as Timestamp;
}

function makeDispatch(overrides: Partial<DiagnoseDispatchProto> = {}): DiagnoseDispatchProto {
  return {
    id: `dispatch-${Math.random()}`,
    itemId: "itm_abc123",
    targetSessionUuid: "target-uuid",
    diagnosticSessionId: "headless-diagnose-itm_abc123-uuid",
    status: DiagnoseDispatchStatus.COMPLETED,
    createdAt: makeTimestamp(120),
    // Within formatTimeAgo's "just now" bucket (< 60s) -- matches
    // validation.md's Happy Path Scenario wording exactly.
    completedAt: makeTimestamp(5),
    ...overrides,
  } as DiagnoseDispatchProto;
}

describe("DiagnoseOutcomeDisplay", () => {
  beforeEach(() => {
    mockListDiagnoseDispatches.mockReset();
    mockUseFeatureFlag.mockReset();
    mockUseFeatureFlag.mockReturnValue(true);
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  // Renders and resolves via screen.findBy* (real polling, not a fixed
  // microtask count) so this never races the fetch-on-mount effect.
  function renderDisplay(dispatches: DiagnoseDispatchProto[], onDiagnose = jest.fn()) {
    mockListDiagnoseDispatches.mockResolvedValue({ dispatches });
    return render(<DiagnoseOutcomeDisplay itemId="itm_abc123" onDiagnose={onDiagnose} />);
  }

  it("renders nothing but stays mounted when there is no dispatch yet and the flag is on", async () => {
    renderDisplay([]);
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.queryByTestId("diagnose-outcome-nudging-disabled")).not.toBeInTheDocument();
  });

  it("shows the nudging-disabled banner proactively before any dispatch when the flag is off", async () => {
    mockUseFeatureFlag.mockReturnValue(false);
    renderDisplay([]);

    const banner = await screen.findByTestId("diagnose-outcome-nudging-disabled");
    expect(banner).toHaveTextContent(
      "Nudging is currently disabled — diagnosis will file a bug or post a note, but won't act on the session directly."
    );
  });

  it("shows the Diagnosing… busy state, sourced from ListDiagnoseDispatches' Pending row, not client state", async () => {
    renderDisplay([
      makeDispatch({ status: DiagnoseDispatchStatus.PENDING, outcomeKind: undefined, completedAt: undefined }),
    ]);

    const pending = await screen.findByTestId("diagnose-outcome-pending");
    expect(pending).toHaveTextContent("Diagnosing…");
    expect(pending).toHaveAttribute("aria-busy", "true");
  });

  it("renders the Nudged outcome with acted styling, a diagnosis link, and never implies resolution", async () => {
    renderDisplay([makeDispatch({ outcomeKind: "nudged" })]);

    const nudged = await screen.findByTestId("diagnose-outcome-nudged");
    expect(nudged).toHaveTextContent("Diagnosed just now — nudged the session.");
    expect(nudged).not.toHaveTextContent(/resolved|fixed|all good/i);
    expect(screen.getByRole("link", { name: "View diagnosis" })).toHaveAttribute(
      "href",
      "/?session=headless-diagnose-itm_abc123-uuid"
    );
  });

  it("names the specific SafetyGateReason for a not_idle skip, never a generic 'skipped'", async () => {
    renderDisplay([makeDispatch({ outcomeKind: "skipped_safety_gate", safetyGateReason: "not_idle" })]);

    const el = await screen.findByTestId("diagnose-outcome-skipped-safety-gate");
    expect(el).toHaveTextContent("Diagnosed just now — nudge skipped (session wasn't idle).");
  });

  it("names the specific SafetyGateReason for an identity-mismatch skip", async () => {
    renderDisplay([
      makeDispatch({ outcomeKind: "skipped_safety_gate", safetyGateReason: "identity_mismatch_instance" }),
    ]);

    const el = await screen.findByTestId("diagnose-outcome-skipped-safety-gate");
    expect(el).toHaveTextContent("nudge skipped (identity check failed)");
  });

  it("names the specific SafetyGateReason for a nudge-cap-reached skip", async () => {
    renderDisplay([makeDispatch({ outcomeKind: "skipped_safety_gate", safetyGateReason: "nudge_cap_reached" })]);

    const el = await screen.findByTestId("diagnose-outcome-skipped-safety-gate");
    expect(el).toHaveTextContent("nudge skipped (nudge cap reached)");
  });

  it("links a BugFiled outcome to the filed bug", async () => {
    renderDisplay([makeDispatch({ outcomeKind: "bug_filed", bugItemId: "itm_bug999" })]);

    const el = await screen.findByTestId("diagnose-outcome-bug-filed");
    expect(el).toHaveTextContent("Diagnosed just now — filed a bug instead of nudging.");
    expect(screen.getByRole("link", { name: "View bug" })).toHaveAttribute("href", "/backlog?item=itm_bug999");
  });

  it("links an Inconclusive outcome to the Activity Log note", async () => {
    renderDisplay([
      makeDispatch({ outcomeKind: "inconclusive_note_filed", noteText: "couldn't confirm the write landed" }),
    ]);

    const el = await screen.findByTestId("diagnose-outcome-inconclusive");
    expect(el).toHaveTextContent("Diagnosed just now — inconclusive.");
    expect(screen.getByRole("link", { name: "View diagnostic note" })).toHaveAttribute(
      "href",
      "#backlog-activity-log"
    );
  });

  it("renders DispatchFailed as its own role=alert banner with a real Retry button that re-invokes onDiagnose", async () => {
    const onDiagnose = jest.fn().mockResolvedValue(undefined);
    renderDisplay([makeDispatch({ outcomeKind: "dispatch_failed", failureReason: "MCP server unreachable" })], onDiagnose);

    const banner = await screen.findByTestId("diagnose-outcome-dispatch-failed");
    expect(banner).toHaveAttribute("role", "alert");
    expect(banner).toHaveTextContent("Couldn't start diagnosis — MCP server unreachable. Try again.");

    const retryButton = screen.getByRole("button", { name: "Retry" });
    expect(retryButton.tagName).toBe("BUTTON");

    await act(async () => {
      retryButton.click();
      await Promise.resolve();
    });

    expect(onDiagnose).toHaveBeenCalledWith("itm_abc123");
  });

  it("renders Stalled distinctly from Pending and DispatchFailed, with no role=alert and a recovery link", async () => {
    renderDisplay([makeDispatch({ status: DiagnoseDispatchStatus.STALLED, outcomeKind: undefined, completedAt: undefined })]);

    const stalled = await screen.findByTestId("diagnose-outcome-stalled");
    expect(stalled).toHaveAttribute("aria-live", "polite");
    expect(stalled).not.toHaveAttribute("role", "alert");
    expect(stalled).toHaveTextContent(
      "Diagnosis stopped without a completion signal. Open the session to see what it accomplished, then diagnose again if needed."
    );
    expect(screen.getByRole("link", { name: "View diagnosis session" })).toBeInTheDocument();

    expect(screen.queryByTestId("diagnose-outcome-pending")).not.toBeInTheDocument();
    expect(screen.queryByTestId("diagnose-outcome-dispatch-failed")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("still shows the current outcome alongside the nudging-disabled banner when the flag is off after a dispatch settled", async () => {
    mockUseFeatureFlag.mockReturnValue(false);
    renderDisplay([makeDispatch({ outcomeKind: "bug_filed", bugItemId: "itm_bug999" })]);

    await screen.findByTestId("diagnose-outcome-bug-filed");
    expect(screen.getByTestId("diagnose-outcome-nudging-disabled")).toBeInTheDocument();
  });
});
