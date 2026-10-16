import { useState } from "react";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UpNextTabs, type UpNextTabsProps } from "./UpNextTabs";
import type { UpNextTab } from "@/lib/unfinished/upNextTab";

const panels = {
  prs: <p>prs body</p>,
  stuck: <p>stuck body</p>,
  worktrees: <p>worktrees body</p>,
  queue: <p>queue body</p>,
};

function Harness(props: Partial<UpNextTabsProps> & { initial?: UpNextTab }) {
  const [value, setValue] = useState<UpNextTab>(props.initial ?? "prs");
  return (
    <UpNextTabs panels={panels} {...props} value={value} onValueChange={setValue} />
  );
}

describe("UpNextTabs", () => {
  it("upNextTabs_should_RenderFourTabsInFixedOrder_When_Mounted", () => {
    render(<Harness />);
    expect(screen.getAllByRole("tab").map((t) => t.textContent)).toEqual([
      "PRs",
      "Stuck",
      "Worktrees",
      "Queue",
    ]);
    expect(screen.getByRole("tablist", { name: "Up next sections" })).toBeInTheDocument();
  });

  it("upNextTabs_should_KeepSelection_When_ParentIgnoresOnValueChange", async () => {
    const onValueChange = jest.fn();
    render(<UpNextTabs value="queue" onValueChange={onValueChange} panels={panels} />);
    expect(screen.getByRole("tab", { name: "Queue" })).toHaveAttribute("aria-selected", "true");
    await userEvent.click(screen.getByRole("tab", { name: "Worktrees" }));
    expect(onValueChange).toHaveBeenCalledWith("worktrees");
    expect(screen.getByRole("tab", { name: "Queue" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Worktrees" })).toHaveAttribute("aria-selected", "false");
  });

  it("upNextTabs_should_ShowCountInNameAndHideZeroOrUnknownBadge_When_BadgesProvided", () => {
    render(<Harness badges={{ prs: 3, stuck: 0 }} />);
    expect(screen.getByRole("tab", { name: "PRs, 3 need attention" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Stuck" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Worktrees" })).toBeInTheDocument();
    expect(screen.getByTestId("up-next-tab-stuck").querySelector("span")).toBeNull();
  });

  it("upNextTabs_should_ExposeCountInAccessibleName_When_CountGreaterThanZero", () => {
    render(<Harness badges={{ queue: 1 }} />);
    expect(screen.getByRole("tab", { name: "Queue, 1 need attention" })).toBeInTheDocument();
  });

  it("upNextTabs_should_RenderPlusSuffixAndOrMoreAccessibleName_When_BadgeDegraded", () => {
    render(<Harness badges={{ prs: 3 }} degraded={{ prs: true }} />);
    const tab = screen.getByRole("tab", { name: "PRs, 3 or more need attention" });
    const visible = screen.getByText("3+");
    expect(visible).toHaveAttribute("aria-hidden", "true");
    expect(tab).toContainElement(visible);
  });

  it("upNextTabs_should_ExplainDegradedBadgeByTooltipAndDescribedBy_When_BadgeDegraded", () => {
    const text = "At least 3 PRs need attention. Review-thread counts are not loaded yet.";
    render(<Harness badges={{ prs: 3 }} degraded={{ prs: true }} descriptions={{ prs: text }} />);
    const tab = screen.getByTestId("up-next-tab-prs");
    expect(tab).toHaveAttribute("title", text);
    const describedBy = tab.getAttribute("aria-describedby");
    expect(describedBy).toBeTruthy();
    expect(document.getElementById(describedBy as string)).toHaveTextContent(text);
  });

  it("upNextTabs_should_MoveFocusOnArrowAndSelectOnlyOnEnterOrSpace_When_ManualActivation", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const prs = screen.getByRole("tab", { name: "PRs" });
    const stuck = screen.getByRole("tab", { name: "Stuck" });
    const queue = screen.getByRole("tab", { name: "Queue" });
    const worktrees = screen.getByRole("tab", { name: "Worktrees" });

    act(() => prs.focus());
    await user.keyboard("{ArrowRight}");
    expect(stuck).toHaveFocus();
    expect(prs).toHaveAttribute("aria-selected", "true");
    expect(stuck).toHaveAttribute("aria-selected", "false");

    await user.keyboard("{End}");
    expect(queue).toHaveFocus();
    await user.keyboard("{Home}");
    expect(prs).toHaveFocus();

    await user.keyboard("{ArrowRight}{Enter}");
    expect(stuck).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{ArrowRight} ");
    expect(worktrees).toHaveAttribute("aria-selected", "true");
  });

  it("upNextTabs_should_RenderFocusablePanelWithLabelledByAndH2_When_PanelEmptyOrSkeleton", () => {
    render(<Harness />);
    const panel = screen.getByRole("tabpanel");
    expect(screen.getAllByRole("tabpanel")).toHaveLength(1);
    expect(panel).toHaveAttribute("tabindex", "0");
    const labelledBy = panel.getAttribute("aria-labelledby");
    expect(labelledBy).toBe(screen.getByRole("tab", { name: "PRs" }).id);
    const heading = screen.getByRole("heading", { level: 2, name: "Open pull requests" });
    expect(panel).toContainElement(heading);
    expect(heading).toHaveAttribute("tabindex", "-1");
    expect(screen.queryByText("stuck body")).not.toBeInTheDocument();
  });
});
