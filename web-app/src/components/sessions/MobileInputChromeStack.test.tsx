import { render, screen } from "@testing-library/react";
import { MobileInputChromeStack } from "./MobileInputChromeStack";
import { chromeBudget } from "@/lib/terminal/mobileInput/chromeBudget";

describe("MobileInputChromeStack", () => {
  it("stack_should_RenderTerminalFirstThenChromeInOrderAndExposeDensity_When_LandscapeBudget", () => {
    const budget = chromeBudget({ viewportHeightPx: 170, cellHeightPx: 17, toolbarPx: 44, composeOpen: true, chipVisible: true });
    render(<MobileInputChromeStack budget={budget} terminal={<div>term</div>} chrome={[<div key="t">toolbar</div>, <div key="c">compose</div>]} />);
    const root = screen.getByTestId("mobile-input-chrome-stack");
    expect(root).toHaveAttribute("data-density", "compact");
    expect(root.textContent).toBe("termtoolbarcompose");
  });
});
