/**
 * UX-AC76: the operator vocabulary is enforced by a test, not by review. Scope is UI
 * strings only; internal identifiers (wake_trigger, CANCEL_WAKE, agent_wake_triggers) keep their names.
 */
import fs from "fs";
import path from "path";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import * as copy from "./notificationCopy";
import { BANNER_COPY, NO_RESPONSE_COPY, STILL_SENDING_COPY } from "@/components/ui/OfflineBanner";
import { flagOffMessage } from "@/lib/hooks/useFeatureFlagValue";
import { makeApprovalDescriptors, makeDecisionDescriptors, makeDescriptor, makeQuestionDescriptors } from "@/components/ui/__fixtures__/notificationFixtures";
import { invokeMock, renderRow, respondWith } from "@/components/ui/__fixtures__/actionRowHarness";
import { ActionKind } from "@/gen/session/v1/notification_action_pb";

jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

const WAKE = /\bwakes?\b/i;
const WAITING_ON = /waiting on/i;
const BARE_CANCEL = /^\s*cancel\s*$/i;

function allCopyStrings(): string[] {
  const out: string[] = [];
  const visit = (v: unknown): void => {
    if (typeof v === "string") out.push(v);
    else if (typeof v === "function") visit((v as (...a: unknown[]) => unknown)("sample", 3));
    else if (v && typeof v === "object") Object.values(v).forEach(visit);
  };
  Object.values(copy).forEach(visit);
  [...Object.values(BANNER_COPY), NO_RESPONSE_COPY, STILL_SENDING_COPY].forEach(visit);
  return out;
}

function expectClean(text: string) {
  expect(text).not.toMatch(WAKE);
  expect(text).not.toMatch(WAITING_ON);
  expect(text).not.toMatch(BARE_CANCEL);
}

// The notification surfaces: tray, items, toasts, banners and the action row. Other dialogs keep their own labels.
const SURFACE = /^(Notification|Tray|OfflineBanner|Background|Alerts)/;

const sourceFiles = (dir: string, onlySurface = false): string[] =>
  fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) return e.name === "__tests__" || e.name === "__fixtures__" ? [] : sourceFiles(p, onlySurface);
    const isSource = /\.tsx?$/.test(e.name) && !/\.(test|stories)\.tsx?$/.test(e.name);
    return isSource && (!onlySurface || SURFACE.test(e.name)) ? [p] : [];
  });

describe("notification copy vocabulary", () => {
  it("exports at least the strings this guard is meant to cover", () => {
    expect(allCopyStrings().length).toBeGreaterThan(40);
  });

  it.each(allCopyStrings())("copy string %j avoids wake, Waiting on and a bare Cancel", (text) => {
    expectClean(text);
  });

  it("uses the exact FLAG_OFF string", () => {
    expect(copy.COPY.flagOff).toBe("This action is turned off. Open session.");
    expect(flagOffMessage).toBe(copy.COPY.flagOff);
  });

  it("keeps bare Cancel and Waiting on out of the notification surfaces' source", () => {
    const root = path.resolve(__dirname, "../..");
    const files = [
      ...sourceFiles(path.join(root, "components/ui/notification-actions")),
      ...sourceFiles(path.join(root, "components/ui"), true),
      ...sourceFiles(path.join(root, "lib/copy")),
    ];
    const hits = files.flatMap((f) => {
      const text = fs.readFileSync(f, "utf8");
      return [/["'`]Cancel["'`]/, />\s*Cancel\s*</, WAITING_ON].filter((re) => re.test(text)).map((re) => `${path.basename(f)} ${re}`);
    });
    expect(hits).toEqual([]);
  });
});

describe("rendered action row states", () => {
  beforeEach(() => invokeMock.mockReset());

  const bodyText = () => `${document.body.textContent} ${Array.from(document.querySelectorAll("[aria-label]")).map((e) => e.getAttribute("aria-label")).join(" ")}`;

  it.each([
    ActionOutcome.APPLIED,
    ActionOutcome.ALREADY_RESOLVED,
    ActionOutcome.STALE,
    ActionOutcome.NOT_SENT,
    ActionOutcome.SEND_INDETERMINATE,
    ActionOutcome.FORBIDDEN,
    ActionOutcome.FLAG_OFF,
    ActionOutcome.INVALID,
  ])("outcome %s renders clean copy", async (outcome) => {
    respondWith(outcome, { staleReason: "dialog_replaced", detail: "desktop_only" });
    renderRow({ descriptors: makeApprovalDescriptors() });
    await userEvent.click(screen.getByTestId("action-approve"));
    await screen.findByTestId("action-status");
    expectClean(bodyText());
  });

  it("renders clean copy in the idle, Deny, Merge and Cancel-prompt sub-steps", async () => {
    const cancel = makeDescriptor({ kind: ActionKind.CANCEL_WAKE, label: "Cancel prompt", targetKey: "w1" });
    renderRow({ descriptors: [...makeApprovalDescriptors(), ...makeDecisionDescriptors(), ...makeQuestionDescriptors(), cancel], approval: { text: "ls" } });
    expectClean(bodyText());
    await userEvent.click(screen.getByTestId("action-deny"));
    expectClean(bodyText());
    await userEvent.click(screen.getByTestId("action-back"));
    await userEvent.click(screen.getByTestId("action-backlog_decision-merge"));
    expectClean(bodyText());
    await userEvent.click(screen.getByTestId("action-back"));
    await userEvent.click(screen.getByTestId("action-cancel_wake"));
    expectClean(bodyText());
    expect(screen.getByText("Keep")).toBeInTheDocument();
  });

  it("renders clean copy for the rate-limit countdown", async () => {
    jest.useFakeTimers();
    respondWith(ActionOutcome.RATE_LIMITED, { retryAfterSeconds: 2 });
    renderRow({ descriptors: makeApprovalDescriptors() });
    screen.getByTestId("action-approve").click();
    await act(async () => {});
    expectClean(bodyText());
    jest.useRealTimers();
  });
});
