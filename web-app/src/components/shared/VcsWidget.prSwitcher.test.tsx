import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { VcsWidget } from "./VcsWidget";
import type { GithubPrEntry, GithubSummary, VcsWidgetData } from "@/lib/vcs/types";

let mockOn = true;
jest.mock("@/lib/features/sessionMultiPr", () => ({ useSessionMultiPR: () => mockOn }));

function summary(n: number, over: Partial<GithubSummary> = {}): GithubSummary {
  return {
    owner: "acme", repo: "app", prUrl: `https://github.com/acme/app/pull/${n}`, prNumber: n, prState: "open", isDraft: false,
    checkConclusion: "success", approvedCount: 0, changesReqCount: 0, mergeable: "unknown", checks: [], reviewFeedback: [], ...over,
  };
}
const entry = (n: number, over: Partial<GithubPrEntry> & { state?: GithubSummary["prState"] } = {}): GithubPrEntry => ({
  summary: summary(n, { prState: over.state ?? "open" }), isOpen: (over.state ?? "open") === "open", isActive: false, ...over,
});

function data(entries: GithubPrEntry[]): VcsWidgetData {
  const active = entries.find((e) => e.isActive) ?? entries[0];
  return {
    kind: "live", branch: "feat/x", isClean: true, fileChanges: [], aheadOfMain: 0, behindMain: 0, branchExists: true,
    commits: [], github: active.summary, githubPrs: entries, shipped: false,
  } as VcsWidgetData;
}

const twoOpen = () => [entry(12, { isActive: true }), entry(14), entry(11, { state: "merged" })];

function setPhone(phone: boolean) {
  window.matchMedia = jest.fn().mockImplementation((query: string) => ({
    matches: phone && query.includes("max-width: 480px"), media: query, addEventListener: jest.fn(), removeEventListener: jest.fn(),
  })) as unknown as typeof window.matchMedia;
}

beforeEach(() => { mockOn = true; setPhone(false); });

describe("VcsWidget PR switcher (S4)", () => {
  it("switcher_should_defaultToActivePrAsSegmentedControl_When_TwoOpenPrsOnDesktop", () => {
    render(<VcsWidget data={data(twoOpen())} mode="full" />);
    const group = screen.getByRole("radiogroup", { name: "Pull request:" });
    expect(within(group).getByRole("radio", { name: "#12 Active" })).toBeChecked();
    expect(within(group).getByRole("radio", { name: "#14" })).not.toBeChecked();
    expect(within(group).queryByRole("radio", { name: /#11/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Pull request" })).not.toBeInTheDocument();
  });

  it("switcher_should_beANativeSelect_When_Phone", () => {
    setPhone(true);
    render(<VcsWidget data={data(twoOpen())} mode="full" />);
    const select = screen.getByRole("combobox", { name: "Pull request" });
    expect(select).toHaveValue("0");
    expect(within(select).getByRole("option", { name: "#12 acme/app (active)" })).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
  });

  it("switcher_should_beANativeSelect_When_MoreThanFourOpenPrs", () => {
    const five = [entry(1, { isActive: true }), entry(2), entry(3), entry(4), entry(5)];
    render(<VcsWidget data={data(five)} mode="full" />);
    expect(screen.getByRole("combobox", { name: "Pull request" })).toBeInTheDocument();
  });

  it("switcher_should_renderNothing_When_OneOpenPrZeroOpenPrsOrFlagOff", () => {
    const { rerender } = render(<VcsWidget data={data([entry(12, { isActive: true }), entry(11, { state: "merged" })])} mode="full" />);
    expect(screen.queryByTestId("pr-switcher")).not.toBeInTheDocument();
    rerender(<VcsWidget data={data([entry(12, { isActive: true, state: "merged" }), entry(11, { state: "closed" })])} mode="full" />);
    expect(screen.queryByTestId("pr-switcher")).not.toBeInTheDocument();
    mockOn = false;
    rerender(<VcsWidget data={data(twoOpen())} mode="full" />);
    expect(screen.queryByTestId("pr-switcher")).not.toBeInTheDocument();
  });

  it("widget_should_stateTheTargetAndShowTheChosenPr_When_AnotherSegmentIsPicked", async () => {
    const user = userEvent.setup();
    render(<VcsWidget data={data(twoOpen())} mode="full" />);
    expect(screen.getByText("Actions apply to #12 acme/app.")).toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "#14" }));
    expect(screen.getByText("Actions apply to #14 acme/app.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /#14/ })).toHaveAttribute("href", "https://github.com/acme/app/pull/14");
    expect(screen.getByTestId("pr-switcher-target")).toHaveAttribute("aria-live", "polite");
  });

  it("switcher_should_moveSelectionWithArrowKeys", async () => {
    const user = userEvent.setup();
    render(<VcsWidget data={data(twoOpen())} mode="full" />);
    screen.getByRole("radio", { name: "#12 Active" }).focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("radio", { name: "#14" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "#14" })).toHaveFocus();
    await user.keyboard("{ArrowLeft}");
    expect(screen.getByRole("radio", { name: "#12 Active" })).toBeChecked();
  });

  it("widget_should_fallBackToActiveWithANotice_When_TheChosenPrMerges", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<VcsWidget data={data(twoOpen())} mode="full" />);
    await user.click(screen.getByRole("radio", { name: "#14" }));
    const merged = [entry(12, { isActive: true }), entry(14, { state: "merged" }), entry(13), entry(11, { state: "merged" })];
    rerender(<VcsWidget data={data(merged)} mode="full" />);
    expect(screen.getByText("#14 is now merged; showing #12")).toBeInTheDocument();
    expect(screen.getByText("Actions apply to #12 acme/app.")).toBeInTheDocument();
  });
});
