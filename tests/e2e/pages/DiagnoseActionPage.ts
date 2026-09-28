import { APIRequestContext, Locator, Page, Route } from "@playwright/test";
import { StuckItemsPage, seedStuckItem } from "./StuckItemsPage";

export const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

// config/config.go's DiagnoseNudgeExecutionFeatureFlag key -- gates the nudge
// write call itself (default off), mirrored client-side by
// DiagnoseOutcomeDisplay.tsx's DIAGNOSE_NUDGE_EXECUTION_FLAG constant.
const DIAGNOSE_NUDGE_EXECUTION_FLAG = "diagnose_nudge_execution";

export async function setDiagnoseNudgeExecutionFlag(request: APIRequestContext, enabled: boolean): Promise<void> {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name: DIAGNOSE_NUDGE_EXECUTION_FLAG, enabled },
  });
}

/**
 * Polls ListStuckBacklogItems until the `backlog` feature flag's RPC gate
 * has actually taken effect server-side -- shared by all three diagnose e2e
 * spec files (diagnose-action/-history/-accessibility.spec.ts) to avoid the
 * jscpd cross-file duplication this repo's `make ready` gate flags on a
 * copy-pasted helper.
 */
export async function waitForBacklogRPCsEnabled(request: APIRequestContext): Promise<void> {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListStuckBacklogItems`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

export function uniqueDiagnoseTitle(prefix: string, label: string): string {
  return `fix: ${prefix} e2e ${label} ${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
}

/** Seeds a stuck item (reason "rework_cap", validation.md's Happy Path Scenario) and expands its card, revealing the Diagnose button/outcome/history. */
export async function openStuckItemDetail(page: Page, request: APIRequestContext, title: string): Promise<StuckItemsPage> {
  await seedStuckItem(request, { itemId: `e2e-diag-${Date.now()}`, title, reason: "rework_cap" });
  const stuckPage = new StuckItemsPage(page);
  await stuckPage.goto();
  await stuckPage.cardByTitle(title).click();
  return stuckPage;
}

export type DiagnoseDispatchStatusWire =
  | "DIAGNOSE_DISPATCH_STATUS_PENDING"
  | "DIAGNOSE_DISPATCH_STATUS_COMPLETED"
  | "DIAGNOSE_DISPATCH_STATUS_STALLED";

/**
 * JSON shape of `session.v1.DiagnoseDispatchProto` as it appears over the
 * wire (ConnectRPC JSON codec) -- `google.protobuf.Timestamp` fields
 * serialize as RFC3339 strings and `status` serializes as its full
 * `DIAGNOSE_DISPATCH_STATUS_*` name, per this repo's established
 * HandoffSummaryPage.ts / idle-session-chip.spec.ts precedent for enum wire
 * encoding. Field names/types read directly from
 * web-app/src/gen/session/v1/diagnose_pb.ts, not from validation.md's/
 * design/ux.md's prose.
 */
export interface DiagnoseDispatchFixture {
  id?: string;
  itemId?: string;
  targetSessionUuid?: string;
  diagnosticSessionId?: string;
  status?: DiagnoseDispatchStatusWire;
  outcomeKind?: "nudged" | "skipped_safety_gate" | "bug_filed" | "inconclusive_note_filed" | "dispatch_failed";
  safetyGateReason?: string;
  bugItemId?: string;
  noteText?: string;
  failureReason?: string;
  createdAt?: string;
  completedAt?: string;
}

export function diagnoseDispatchFixture(overrides: DiagnoseDispatchFixture = {}): Record<string, unknown> {
  const now = new Date().toISOString();
  const status = overrides.status ?? "DIAGNOSE_DISPATCH_STATUS_COMPLETED";
  return {
    id: overrides.id ?? `dispatch-${Math.random().toString(36).slice(2)}`,
    itemId: overrides.itemId ?? "",
    targetSessionUuid: overrides.targetSessionUuid ?? "target-uuid",
    diagnosticSessionId: overrides.diagnosticSessionId ?? "headless-diagnose-e2e-item-uuid",
    status,
    outcomeKind: overrides.outcomeKind,
    safetyGateReason: overrides.safetyGateReason,
    bugItemId: overrides.bugItemId,
    noteText: overrides.noteText,
    failureReason: overrides.failureReason,
    createdAt: overrides.createdAt ?? now,
    completedAt: status === "DIAGNOSE_DISPATCH_STATUS_PENDING" ? undefined : (overrides.completedAt ?? now),
  };
}

/**
 * Page object for the "Diagnose" action (design/ux.md Surfaces 1-16):
 * StuckItemDetail's and BacklogItemDetail's Diagnose button, the shared
 * DiagnoseOutcomeDisplay outcome states, and DiagnoseHistoryList.
 *
 * All locators use `data-testid`/ARIA roles only (e2e-test-conventions
 * skill), read directly from the current implementation
 * (DiagnoseOutcomeDisplay.tsx, DiagnoseHistoryList.tsx, StuckItemDetail.tsx,
 * BacklogItemDetail.tsx) rather than assumed from validation.md's/
 * design/ux.md's wireframe prose. Only one of StuckItemDetail/
 * BacklogItemDetail is ever expanded per test, so these testids are looked
 * up page-wide rather than re-scoped per itemId.
 */
export class DiagnoseActionPage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  // -- Diagnose button (Surfaces 1-3) --------------------------------------

  diagnoseButton(surface: "stuck" | "backlog" = "stuck"): Locator {
    return this.page.getByTestId(surface === "stuck" ? "stuck-item-diagnose" : "backlog-detail-diagnose");
  }

  diagnoseError(surface: "stuck" | "backlog" = "stuck"): Locator {
    return this.page.getByTestId(surface === "stuck" ? "stuck-item-diagnose-error" : "backlog-detail-diagnose-error");
  }

  // -- DiagnoseOutcomeDisplay (Surfaces 4-10, 15) --------------------------

  nudgingDisabledBanner(): Locator {
    return this.page.getByTestId("diagnose-outcome-nudging-disabled");
  }

  pendingOutcome(): Locator {
    return this.page.getByTestId("diagnose-outcome-pending");
  }

  stalledOutcome(): Locator {
    return this.page.getByTestId("diagnose-outcome-stalled");
  }

  dispatchFailedOutcome(): Locator {
    return this.page.getByTestId("diagnose-outcome-dispatch-failed");
  }

  outcomeRetryButton(): Locator {
    return this.page.getByTestId("diagnose-outcome-retry-button");
  }

  nudgedOutcome(): Locator {
    return this.page.getByTestId("diagnose-outcome-nudged");
  }

  skippedSafetyGateOutcome(): Locator {
    return this.page.getByTestId("diagnose-outcome-skipped-safety-gate");
  }

  bugFiledOutcome(): Locator {
    return this.page.getByTestId("diagnose-outcome-bug-filed");
  }

  inconclusiveOutcome(): Locator {
    return this.page.getByTestId("diagnose-outcome-inconclusive");
  }

  /** Any of the 7 DiagnoseOutcomeDisplay outcome-state containers, for a "some outcome rendered" wait. */
  anyOutcome(): Locator {
    return this.page.locator(
      '[data-testid^="diagnose-outcome-"]:not([data-testid="diagnose-outcome-nudging-disabled"])'
    );
  }

  // -- DiagnoseHistoryList (Surfaces 11-12) --------------------------------

  historyList(): Locator {
    return this.page.getByTestId("diagnose-history-list");
  }

  historyEmpty(): Locator {
    return this.page.getByTestId("diagnose-history-empty");
  }

  historyError(): Locator {
    return this.page.getByTestId("diagnose-history-error");
  }

  historyItems(): Locator {
    return this.historyList().getByRole("listitem");
  }

  historyRetryButton(): Locator {
    return this.historyError().getByRole("button", { name: "Retry" });
  }

  // -- Route interception ---------------------------------------------------

  /**
   * Intercepts `ListDiagnoseDispatches` with a mutable fixture list --
   * mirrors HandoffSummaryPage.mockHandoffSummary's controller pattern, the
   * established precedent in this repo for injecting hard-to-produce
   * backend states (no test-mode hook exists to force a real diagnostic
   * session to a given outcome without a real LLM call).
   */
  async mockListDiagnoseDispatches(
    initial: Record<string, unknown>[]
  ): Promise<{ set: (list: Record<string, unknown>[]) => void }> {
    let current = initial;
    await this.page.route("**/api/session.v1.DiagnoseService/ListDiagnoseDispatches", async (route: Route) => {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ dispatches: current }) });
    });
    return { set: (list) => { current = list; } };
  }

  /** Forces `ListDiagnoseDispatches` to reject with a ConnectRPC error envelope (Surface 11's fetch-error case). */
  async mockListDiagnoseDispatchesError(message = "backend unavailable"): Promise<void> {
    await this.page.route("**/api/session.v1.DiagnoseService/ListDiagnoseDispatches", async (route: Route) => {
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ code: "unavailable", message }),
      });
    });
  }

  /**
   * Mocks `DiagnoseBacklogItem` to resolve immediately with the given
   * dispatch id/session id (default success shape).
   */
  async mockDiagnoseBacklogItemSuccess(dispatchId = "d1", diagnosticSessionId = "headless-diagnose-e2e-item-uuid") {
    await this.page.route("**/api/session.v1.DiagnoseService/DiagnoseBacklogItem", async (route: Route) => {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ dispatchId, diagnosticSessionId }),
      });
    });
  }

  /** Forces `DiagnoseBacklogItem` to reject with a ConnectRPC error envelope (Surface 9 / dispatch-failed). */
  async mockDiagnoseBacklogItemError(code: string, message: string, status = 400): Promise<void> {
    await this.page.route("**/api/session.v1.DiagnoseService/DiagnoseBacklogItem", async (route: Route) => {
      await route.fulfill({ status, contentType: "application/json", body: JSON.stringify({ code, message }) });
    });
  }

  /**
   * Holds `DiagnoseBacklogItem` pending until `resolve()`/`reject()` is
   * called -- for asserting the in-flight busy state (Surface 3) before the
   * dispatch settles, without a fixed sleep.
   */
  async mockDiagnoseBacklogItemDeferred(): Promise<{
    resolve: (response?: Record<string, unknown>) => void;
    reject: (code: string, message: string, status?: number) => void;
  }> {
    let settleGate!: (fn: (route: Route) => Promise<void>) => void;
    const gate = new Promise<(route: Route) => Promise<void>>((res) => {
      settleGate = res;
    });
    await this.page.route("**/api/session.v1.DiagnoseService/DiagnoseBacklogItem", async (route: Route) => {
      const fulfill = await gate;
      await fulfill(route);
    });
    return {
      resolve: (response = { dispatchId: "d1", diagnosticSessionId: "headless-diagnose-e2e-item-uuid" }) => {
        settleGate(async (route) => {
          await route.fulfill({ contentType: "application/json", body: JSON.stringify(response) });
        });
      },
      reject: (code, message, status = 400) => {
        settleGate(async (route) => {
          await route.fulfill({ status, contentType: "application/json", body: JSON.stringify({ code, message }) });
        });
      },
    };
  }
}
