import React from "react";
import { render, screen } from "@testing-library/react";
import { VcsWidgetBlockingReasons } from "./VcsWidgetBlockingReasons";
import type { BlockingReason } from "@/lib/vcs/mergeability";

const REASONS: BlockingReason[] = [
  { key: "draft", label: "Draft" },
  { key: "changes_requested", label: "Changes requested (1)" },
  { key: "ci_failing", label: "Checks failing" },
];

describe("VcsWidgetBlockingReasons", () => {
  it("VcsWidgetBlockingReasons_should_RenderNothing_When_ReasonsEmpty", () => {
    const { container } = render(<VcsWidgetBlockingReasons reasons={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("VcsWidgetBlockingReasons_should_RenderNothing_When_ReasonsEmptyEvenWithStaleLastCheckedAt", () => {
    const staleTimestamp = new Date(Date.now() - 10 * 60 * 1000);
    const { container } = render(
      <VcsWidgetBlockingReasons reasons={[]} lastCheckedAt={staleTimestamp} />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("VcsWidgetBlockingReasons_should_RenderOneLiPerReasonInOrder_When_MultipleReasonsProvided", () => {
    render(<VcsWidgetBlockingReasons reasons={REASONS} />);

    const items = screen.getAllByRole("listitem").map((el) => el.textContent);
    expect(items).toEqual(["Draft", "Changes requested (1)", "Checks failing"]);
  });

  it("VcsWidgetBlockingReasons_should_ShowStaleNotice_When_LastCheckedAtOlderThanThreshold", () => {
    const staleTimestamp = new Date(Date.now() - 10 * 60 * 1000); // 10m ago > 3m threshold
    render(<VcsWidgetBlockingReasons reasons={REASONS} lastCheckedAt={staleTimestamp} />);

    expect(screen.getByTestId("blocking-reasons-stale")).toHaveTextContent(
      "These reasons may be out of date"
    );
  });

  it("VcsWidgetBlockingReasons_should_OmitStaleNotice_When_LastCheckedAtRecent", () => {
    const recentTimestamp = new Date(Date.now() - 30 * 1000); // 30s ago < 3m threshold
    render(<VcsWidgetBlockingReasons reasons={REASONS} lastCheckedAt={recentTimestamp} />);

    expect(screen.queryByTestId("blocking-reasons-stale")).not.toBeInTheDocument();
  });

  it("VcsWidgetBlockingReasons_should_OmitStaleNotice_When_LastCheckedAtUndefined", () => {
    render(<VcsWidgetBlockingReasons reasons={REASONS} />);

    expect(screen.queryByTestId("blocking-reasons-stale")).not.toBeInTheDocument();
  });
});
