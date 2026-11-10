import { screen } from "@testing-library/react";
import { renderWithNav } from "@/lib/nav/testing";
import { BacklogViewTabs, type BacklogView } from "./BacklogViewTabs";

describe("BacklogViewTabs", () => {
  it.each<[BacklogView, string]>([
    ["list", "List"],
    ["board", "Board"],
    ["flow", "Flow"],
  ])("BacklogViewTabs_should_MarkOnlyActiveTabCurrent_When_ViewIs_%s", (view, label) => {
    renderWithNav(<BacklogViewTabs active={view} />);
    const nav = screen.getByRole("navigation", { name: "Backlog views" });
    const links = nav.querySelectorAll("a");
    expect([...links].map((a) => a.textContent)).toEqual(["List", "Board", "Flow"]);
    expect([...links].map((a) => a.getAttribute("href"))).toEqual(["/backlog", "/backlog/board", "/backlog/flow"]);
    const current = nav.querySelectorAll('[aria-current="page"]');
    expect(current).toHaveLength(1);
    expect(current[0].textContent).toBe(label);
  });
});
