import { fireEvent, render, screen, within } from "@testing-library/react";
import { FlowChart, type FlowChartKind, type FlowChartProps } from "./FlowChart";

const WEEKS = ["Oct 12", "Oct 19", "Oct 26"];
const SERIES = [
  { name: "FAIL", values: [4, 3, 2] },
  { name: "PASS", values: [6, 7, 9] },
  { name: "PARTIAL", values: [1, 1, 0] },
];

function renderChart(overrides: Partial<FlowChartProps> = {}) {
  return render(
    <FlowChart caption="Verdict mix per week" takeaway="FAIL share fell from 36% to 15% over 3 weeks" categories={WEEKS} series={SERIES} kind="stacked-bar" {...overrides} />,
  );
}

beforeAll(() => {
  // The jest style mock hands React proxy functions for class names; a failed chart also logs by design.
  jest.spyOn(console, "error").mockImplementation(() => {});
});

describe("FlowChart", () => {
  it.each<FlowChartKind>(["line", "stacked-bar"])("FlowChart_should_RenderFigureCaptionImgLabelAndTable_When_DataPresent_%s", (kind) => {
    renderChart({ kind });

    const figure = screen.getByRole("figure");
    expect(within(figure).getByText("Verdict mix per week", { selector: "figcaption" })).toBeInTheDocument();
    expect(within(figure).getByRole("img", { name: "FAIL share fell from 36% to 15% over 3 weeks" })).toBeInTheDocument();

    const table = within(figure).getByTestId("flow-chart-table");
    expect(within(table).getAllByRole("columnheader").map((h) => h.textContent)).toEqual(["Period", "FAIL", "PASS", "PARTIAL"]);
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows.map((r) => within(r).getAllByRole("cell").map((c) => c.textContent))).toEqual([
      ["4", "6", "1"],
      ["3", "7", "1"],
      ["2", "9", "0"],
    ]);
    expect(within(rows[0]).getByRole("rowheader")).toHaveTextContent("Oct 12");
  });

  it.each<FlowChartKind>(["line", "stacked-bar"])("FlowChart_should_TellSeriesApartWithoutColor_When_Rendered_%s", (kind) => {
    renderChart({ kind });

    const encodings = [...screen.getByRole("figure").querySelectorAll("[data-encoding]")].map((el) => el.getAttribute("data-encoding"));
    expect(encodings).toHaveLength(3);
    expect(new Set(encodings).size).toBe(3);
    expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["FAIL", "PASS", "PARTIAL"]);
  });

  it("FlowChart_should_ShowEmptyTextErrorFallbackAndTableToggle_When_EmptyThrowingOrToggled", () => {
    const { unmount } = renderChart({ series: [], categories: [] });
    expect(screen.getByText("No data in this window")).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
    unmount();

    // An unknown kind makes drawing throw; the table must still carry the values.
    const failed = renderChart({ kind: "bogus" as FlowChartKind });
    expect(screen.getByText("Chart unavailable, showing data")).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.getByTestId("flow-chart-table")).toHaveTextContent("Oct 26");
    failed.unmount();

    renderChart();
    const toggle = screen.getByRole("button", { name: "Show data table" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByTestId("flow-chart-table").className).toMatch(/tableHidden/);
    fireEvent.click(toggle);
    expect(screen.getByRole("button", { name: "Hide data table" })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByTestId("flow-chart-table").className).not.toMatch(/tableHidden/);
  });

  it("FlowChart_should_KeepTableInAccessibilityTree_When_ToggleClosed", () => {
    renderChart();

    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show data table" })).toHaveAttribute("aria-controls", screen.getByTestId("flow-chart-table").parentElement?.id);
  });
});
