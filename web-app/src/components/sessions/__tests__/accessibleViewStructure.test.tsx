/**
 * Story 0.1.6 / Task 1.5.4a: structure of the production Accessible view (grown from the Task 0.1.6a
 * prototype): pages of exactly 500 numbered lines, pager label, 44 px controls.
 */
import { render, screen, within, fireEvent } from "@testing-library/react";
import { AccessibleView } from "../files/AccessibleView";
import { textPageSource } from "@/lib/files/pageSource";

const doc = Array.from({ length: 3000 }, (_, i) => `L${i + 1} text`).join("\n");
const view = () => render(<AccessibleView path="src/a.go" source={textPageSource(doc)} />);

describe("Accessible view structure (Story 0.1.6)", () => {
  it("accessibleView_should_Render500NumberedLinesPerPageWithPagerLabel_When_3000LineDoc", () => {
    view();
    const rows = screen.getAllByTestId("accessible-view-line");
    expect(rows).toHaveLength(500);
    expect(rows[0].textContent).toBe("1 L1 text");
    expect(rows[499].textContent).toBe("500 L500 text");
    expect(screen.getByTestId("accessible-view-label").textContent).toBe("Lines 1-500 of 3000");

    const pre = screen.getByTestId("accessible-view-page");
    expect(pre.tagName).toBe("PRE");
    expect(pre.getAttribute("tabindex")).toBe("0");
    expect(document.querySelectorAll("pre")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(screen.getByTestId("accessible-view-label").textContent).toBe("Lines 501-1000 of 3000");
    expect(screen.getAllByTestId("accessible-view-line")[0].textContent).toBe("501 L501 text");
    expect(screen.getByTestId("accessible-view-announce").textContent).toBe("Lines 501-1000 of 3000");
    expect(document.activeElement).toBe(screen.getByTestId("accessible-view-page"));
  });

  it("accessibleView_should_KeepPagerButtonsAtLeast44px_And_GoToLineLandOnItsPage", () => {
    view();
    const nav = screen.getByRole("navigation", { name: "Pager" });
    for (const name of ["Previous page", "Next page", "Go"]) {
      const btn = within(nav).getByRole("button", { name });
      expect(btn).toHaveStyle({ minHeight: "44px", minWidth: "44px" });
    }
    expect(screen.getByLabelText("Go to line")).toHaveStyle({ minHeight: "44px" });
    fireEvent.change(screen.getByLabelText("Go to line"), { target: { value: "1234" } });
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    expect(screen.getByTestId("accessible-view-label").textContent).toBe("Lines 1001-1500 of 3000");
    expect(screen.getByTestId("accessible-view-announce").textContent).toBe("Line 1234");
  });
});
