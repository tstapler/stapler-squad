import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError } from "@connectrpc/connect";
import { act, render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { FlowSummarySchema, type FlowSummary } from "@/gen/session/v1/backlog_flow_pb";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import { MemoryNavProvider } from "@/lib/nav/impl.memory";
import { usePathname, useRouter, useSearchParams } from "@/lib/nav/impl.memory";

/** Local wall-clock 14:02:11 on a fixed day, so formatting does not depend on the machine's zone. */
export const AS_OF = new Date(2026, 10, 6, 14, 2, 11);

const OK = { status: "ok" };
type SummaryInit = MessageInitShape<typeof FlowSummarySchema>;

export function makeSummary(overrides: SummaryInit = {}): FlowSummary {
  const base: SummaryInit = {
    meta: { asOf: timestampFromDate(AS_OF) },
    tiles: { meta: OK, merged: { value: 9 } },
    funnel: { meta: OK },
    causes: { meta: OK },
    review: { meta: OK },
    pr: { meta: OK },
    stuck: { meta: OK, open: 8, reasons: [{ reason: "x", duration: { n: 30 } }, { reason: "y", duration: { n: 16 } }] },
    anomalies: { meta: OK, thrashItems: { value: 35 } },
    notes: { meta: OK },
    trends: makeTrends(),
  };
  return create(FlowSummarySchema, { ...base, ...overrides } as SummaryInit);
}

const WEEK_MS = 7 * 86_400_000;
const FIRST_WEEK = Date.UTC(2026, 9, 5);
const weekStart = (i: number) => timestampFromDate(new Date(FIRST_WEEK + i * WEEK_MS));
/** Mirrors session/flow/drill.go: `<series>@<RFC3339 week start>`. */
export const weekKey = (series: string, i: number) => `${series}@${new Date(FIRST_WEEK + i * WEEK_MS).toISOString().replace(".000Z", "Z")}`;

interface WeeklyMix {
  pass: number;
  fail: number;
  partial: number;
  unverifiable: number;
}

/** Five weekly buckets: FAIL share 41% in week 1, 28% in week 5, and an empty week 3. */
export const VERDICT_WEEKS: readonly WeeklyMix[] = [
  { pass: 50, fail: 41, partial: 4, unverifiable: 5 },
  { pass: 55, fail: 35, partial: 4, unverifiable: 6 },
  { pass: 0, fail: 0, partial: 0, unverifiable: 0 },
  { pass: 58, fail: 30, partial: 5, unverifiable: 7 },
  { pass: 60, fail: 28, partial: 5, unverifiable: 7 },
];

export function makeTrends(): SummaryInit["trends"] {
  const throughput = [4, 6, 0, 9, 9];
  const reviewFail = [30, 25, 0, 20, 12];
  return {
    meta: OK,
    throughput: throughput.map((n, i) => ({ weekStart: weekStart(i), n, drillKey: weekKey("merged", i) })),
    throughputTotal: 28,
    verdictMix: VERDICT_WEEKS.map((w, i) => ({
      weekStart: weekStart(i),
      n: w.pass + w.fail + w.partial + w.unverifiable,
      cells: [
        { class: "judgment", outcome: "PASS", n: w.pass },
        { class: "judgment", outcome: "FAIL", n: w.fail },
        { class: "judgment", outcome: "PARTIAL", n: w.partial },
        { class: "judgment", outcome: "UNVERIFIABLE", n: w.unverifiable },
      ].filter((cell) => cell.n > 0),
    })),
    verdictMixTotal: 500,
    rework: reviewFail.map((n, i) => ({
      weekStart: weekStart(i),
      n,
      drillKey: weekKey("rework", i),
      byCause: [
        { name: "review_fail", n, drillKey: weekKey("rework.review_fail", i) },
        { name: "pr_fix", n: i, drillKey: weekKey("rework.pr_fix", i) },
        { name: "bounce", n: 0, drillKey: weekKey("rework.bounce", i) },
        { name: "other", n: 0, drillKey: weekKey("rework.other", i) },
      ],
    })),
    reworkTotal: 120,
  };
}

export const resourceExhausted = (message: string) => new ConnectError(message, Code.ResourceExhausted);

export function LocationProbe() {
  const search = useSearchParams().toString();
  const router = useRouter();
  return (
    <>
      <span data-testid="loc">{`${usePathname()}${search ? `?${search}` : ""}`}</span>
      <button type="button" data-testid="history-back" onClick={router.back} />
    </>
  );
}

export const location = () => screen.getByTestId("loc").textContent;

/** Renders under in-memory nav at `entry`, with the announcer spied. */
export async function renderAt(entry: string, ui: ReactElement) {
  const announce = jest.fn();
  const view = render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <MemoryNavProvider initialEntries={[entry]}>
        {ui}
        <LocationProbe />
      </MemoryNavProvider>
    </AnnouncerContext.Provider>,
  );
  await act(async () => {});
  return { ...view, announce };
}
