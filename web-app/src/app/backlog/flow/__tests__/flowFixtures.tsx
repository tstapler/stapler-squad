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
  };
  return create(FlowSummarySchema, { ...base, ...overrides } as SummaryInit);
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
