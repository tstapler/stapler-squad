import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { FlowAnomaliesSectionSchema, type FlowAnomaliesSection } from "@/gen/session/v1/backlog_flow_pb";
import { FlowAnomalies } from "./FlowAnomalies";
import { ThrashBanner } from "./ThrashBanner";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));

const NOW = new Date(2026, 10, 6, 14, 2, 11).getTime();
const DAY = 86_400_000;
const onDrill = jest.fn();

const thrashRow = (id: string, episodes: number, events: number, lastAgoMs: number, spanDays: number) => ({
  itemId: id,
  title: `Title ${id}`,
  episodes,
  events,
  pair: "pr_pending <-> in_progress",
  firstAt: timestampFromDate(new Date(NOW - lastAgoMs - spanDays * DAY)),
  lastAt: timestampFromDate(new Date(NOW - lastAgoMs)),
});

function anomalies(items: object[], thrashCount = items.length, meta: object = { status: "ok" }): FlowAnomaliesSection {
  return create(FlowAnomaliesSectionSchema, {
    meta,
    thrashItems: { value: thrashCount, drillKey: "thrash.episode" },
    items: { items, totalCount: thrashCount, returnedCount: items.length },
  } as never);
}

const banner = (section: FlowAnomaliesSection | undefined, archived = new Set<string>()) => {
  const onView = jest.fn();
  const onReload = jest.fn();
  render(<ThrashBanner anomalies={section} archivedIds={archived} onView={onView} onReload={onReload} />);
  return { onView, onReload };
};

beforeEach(() => onDrill.mockReset());

describe("ThrashBanner", () => {
  it("ThrashBanner_should_CountItemsNotEpisodesAndOpenAnomalies_When_TwoItemsThrash", () => {
    const { onView } = banner(anomalies([thrashRow("a1", 2, 40, 3_600_000, 3), thrashRow("b2", 5, 90, 7_200_000, 9)]));

    expect(screen.getByRole("status")).toHaveTextContent("2 items thrashing; excluded from averages");
    fireEvent.click(screen.getByRole("button", { name: "View" }));
    expect(onView).toHaveBeenCalledTimes(1);
  });

  it("ThrashBanner_should_LabelArchivedAndUseSingularCopy_When_AllTimeView", () => {
    banner(anomalies([thrashRow("a1", 2, 40, 1000, 3)], 35), new Set(["a1", "zz"]));
    expect(screen.getByRole("status")).toHaveTextContent("35 items thrashing (2 archived); excluded from averages");
  });

  it("ThrashBanner_should_UseSingularCopy_When_OneItem", () => {
    banner(anomalies([thrashRow("a1", 2, 40, 1000, 3)]));
    expect(screen.getByRole("status")).toHaveTextContent("1 item thrashing; excluded from averages");
  });

  it("FlowPage_should_ReplaceBannerWithUnavailableReload_When_AnomalyDataFailed", () => {
    const { onReload } = banner(anomalies([], 0, { status: "failed", error: "boom" }));

    expect(screen.getByRole("status")).toHaveTextContent("Anomaly data unavailable");
    fireEvent.click(screen.getByRole("button", { name: "Reload" }));
    expect(onReload).toHaveBeenCalledTimes(1);
  });

  it("ThrashBanner_should_BeAbsent_When_NoThrashAndDataOk", () => {
    banner(anomalies([], 0, { status: "empty" }));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});

describe("ThrashBanner server archived count", () => {
  it("ThrashBanner_should_UseServerArchivedCount_When_ClientKnowsNoArchivedIds", () => {
    banner(create(FlowAnomaliesSectionSchema, { meta: { status: "ok" }, thrashItems: { value: 35, drillKey: "thrash.episode" }, archivedItems: 2 } as never));

    expect(screen.getByRole("status")).toHaveTextContent("35 items thrashing (2 archived); excluded from averages");
  });
});

describe("FlowAnomalies", () => {
  const view = (section: FlowAnomaliesSection, archived = new Set<string>()) =>
    render(<FlowAnomalies anomalies={section} archivedIds={archived} now={NOW} onDrill={onDrill} />);

  it("FlowAnomalies_should_ChipArchivedRow_When_ServerFlagsItArchived", () => {
    view(anomalies([{ ...thrashRow("a1", 2, 40, 1000, 3), archived: true }, thrashRow("b2", 5, 90, 1000, 9)]));

    const rows = within(screen.getByRole("table")).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("archived");
    expect(rows[1]).not.toHaveTextContent("archived");
  });

  it("FlowAnomalies_should_ListOneRowPerItemWithChipAndDrill_When_ItemsThrash", () => {
    view(anomalies([thrashRow("a1", 2, 87_400, 30_000, 48), thrashRow("b2", 5, 8_700, 4 * DAY, 28)]), new Set(["b2"]));

    const rows = within(screen.getByRole("table")).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("pr_pending <-> in_progress");
    expect(rows[0]).toHaveTextContent("48d");
    expect(rows[0]).toHaveTextContent("30s ago");
    expect(rows[0]).toHaveTextContent("Still active");
    expect(rows[0]).not.toHaveTextContent("archived");
    expect(rows[1]).toHaveTextContent("archived");
    expect(rows[1]).toHaveTextContent("4d ago");
    expect(rows[1]).not.toHaveTextContent("Still active");
    fireEvent.click(within(rows[0]).getByRole("button", { name: /87,400/ }));
    expect(onDrill).toHaveBeenCalledWith("thrash.episode");
    expect(within(rows[0]).getByRole("link", { name: /Open item/ })).toHaveAttribute("href", "/backlog?item=a1");
  });

  it("FlowAnomalies_should_SayNoThrash_When_Empty", () => {
    view(anomalies([], 0, { status: "empty" }));
    expect(screen.getByText("No thrash episodes in this window")).toBeInTheDocument();
  });

  it("FlowAnomalies_should_StateExclusionAndTruncation_When_MoreItemsThanReturned", () => {
    view(anomalies([thrashRow("a1", 2, 40, 1000, 3)], 35));

    expect(screen.getByText(/Excluded from aggregates/)).toBeInTheDocument();
    expect(screen.getByText("Showing 1 of 35")).toBeInTheDocument();
  });
});
