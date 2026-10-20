import React from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BannerStack, type BannerSpec } from "../files/BannerStack";

const info = (n: number, kind: BannerSpec["kind"] = "informational"): BannerSpec => ({
  id: `i${n}`, kind, seq: n, body: `notice ${n}`, onDismiss: jest.fn(),
});
const rejected = (): BannerSpec => ({
  id: "rej", kind: "save_rejected", seq: 0, body: "Could not save: permission",
  actions: [{ label: "Copy my version", onClick: jest.fn() }, { label: "Retry Save", onClick: jest.fn() }],
});

describe("BannerStack", () => {
  it("bannerStack_should_ShowTwoSortedWithBlockingVisibleAndPlusNMore_When_FourBanners", async () => {
    render(<BannerStack banners={[info(1), info(2), info(3), rejected()]} />);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Could not save: permission");
    expect(within(alert).queryByRole("button", { name: "Dismiss" })).toBeNull();
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(screen.getByRole("status")).toHaveTextContent("notice 3");
    const more = screen.getByRole("button", { name: "+2 more" });
    expect(more).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(more);
    expect(screen.getByRole("button", { name: "Show fewer" })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("notice 1")).toBeInTheDocument();
    expect(screen.getByText("notice 2")).toBeInTheDocument();
  });

  it("bannerStack_should_GiveInformationalBannersADismissButton_And_NoPlusMore_When_TwoOrFewer", async () => {
    const one = info(1);
    render(<BannerStack banners={[one]} />);
    expect(screen.queryByRole("button", { name: /more/ })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(one.onDismiss).toHaveBeenCalledTimes(1);
  });

  it("bannerStack_should_KeepBlockingActionsOnOne44pxRow_When_Collapsed", async () => {
    const spec = rejected();
    render(<BannerStack banners={[info(1), info(2), spec]} collapsed />);
    const row = screen.getByTestId("banner-collapsed-row");
    expect(row).toHaveStyle({ minHeight: "44px" });
    expect(within(row).getByRole("alert")).toHaveTextContent("Could not save: permission");
    for (const name of ["Copy my version", "Retry Save"]) {
      const b = within(row).getByRole("button", { name });
      expect(b).toHaveStyle({ minWidth: "44px", minHeight: "44px" });
    }
    const chip = screen.getByRole("button", { name: "2 notices" });
    expect(chip).toHaveStyle({ minWidth: "44px", minHeight: "44px" });
    await userEvent.click(within(row).getByRole("button", { name: "Retry Save" }));
    expect(spec.actions![1].onClick).toHaveBeenCalledTimes(1);
    // tapping the text expands the full stack
    await userEvent.click(within(row).getByRole("button", { name: /Could not save/ }));
    expect(screen.queryByTestId("banner-collapsed-row")).toBeNull();
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("bannerStack_should_RenderNothing_When_NoBanners", () => {
    const { container } = render(<BannerStack banners={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
