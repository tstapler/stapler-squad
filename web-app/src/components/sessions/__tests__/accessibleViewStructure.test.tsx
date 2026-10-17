/**
 * Story 0.1.6 / Task 0.1.6a: structure of the non-virtual Accessible view prototype
 * (seed of Task 1.5.4a): pages of exactly 500 numbered lines, pager label, 44 px controls.
 */
import { render, screen, within, fireEvent } from "@testing-library/react";
import { AccessibleViewSpike } from "@/app/spike-files/AccessibleViewSpike";
import { genDoc } from "@/app/spike-files/cm";

const lines = genDoc(3000).split("\n");

describe("Accessible view structure (Story 0.1.6)", () => {
  it("accessibleView_should_Render500NumberedLinesPerPageWithPagerLabel_When_3000LineDoc", () => {
    render(<AccessibleViewSpike lines={lines} />);
    const rows = screen.getAllByTestId("spike-accessible-line");
    expect(rows).toHaveLength(500);
    expect(rows[0].textContent).toBe("1 L1 text");
    expect(rows[499].textContent).toBe("500 L500 text");
    expect(screen.getByTestId("spike-page-label").textContent).toBe("Lines 1-500 of 3000");

    const pre = screen.getByTestId("spike-accessible-page");
    expect(pre.tagName).toBe("PRE");
    expect(pre.getAttribute("tabindex")).toBe("0");
    expect(document.querySelectorAll("pre")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(screen.getByTestId("spike-page-label").textContent).toBe("Lines 501-1000 of 3000");
    expect(screen.getAllByTestId("spike-accessible-line")[0].textContent).toBe("501 L501 text");
  });

  it("accessibleView_should_KeepPagerButtonsAtLeast44px_And_GoToLineLandOnItsPage", () => {
    render(<AccessibleViewSpike lines={lines} />);
    const nav = screen.getByRole("navigation", { name: "Pager" });
    for (const name of ["Previous page", "Next page", "Go"]) {
      const btn = within(nav).getByRole("button", { name });
      expect(btn.style.minHeight).toBe("44px");
      expect(btn.style.minWidth).toBe("44px");
    }
    fireEvent.change(screen.getByLabelText("Go to line"), { target: { value: "1234" } });
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    expect(screen.getByTestId("spike-page-label").textContent).toBe("Lines 1001-1500 of 3000");
    expect(screen.getByTestId("spike-announce").textContent).toBe("Line 1234");
  });
});
