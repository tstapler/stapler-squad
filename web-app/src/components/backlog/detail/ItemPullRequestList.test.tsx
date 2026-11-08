import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { makePr } from "@/lib/pr/__testUtils__/pullRequests";
import { ItemPullRequestList } from "./ItemPullRequestList";

// The list form of SessionPullRequest omits `url` (K11): links must be derived from the ref.
const listFormPr = (over: Parameters<typeof makePr>[0]) => makePr({ url: "", ...over });

describe("ItemPullRequestList with list-form PRs (no url)", () => {
  it("row_should_linkToTheDerivedPullRequestUrl", () => {
    render(<ItemPullRequestList pullRequests={[listFormPr({ number: 23 }), listFormPr({ number: 7, host: "ghe.example.net" })]} holds={[]} />);
    expect(screen.getByTestId("item-pr-row-23").querySelector("a")).toHaveAttribute("href", "https://github.com/acme/app/pull/23");
    expect(screen.getByTestId("item-pr-row-7").querySelector("a")).toHaveAttribute("href", "https://ghe.example.net/acme/app/pull/7");
  });

  it("stopTracking_undo_should_retrackTheDerivedUrl", async () => {
    const user = userEvent.setup();
    const actions = { stopTracking: jest.fn(async () => ({ ok: true as const })), trackByUrl: jest.fn(async () => ({ ok: true as const })) };
    render(<ItemPullRequestList pullRequests={[listFormPr({ number: 23, itemDirect: true })]} holds={[]} actions={actions} />);
    await user.click(screen.getByTestId("item-pr-stop-23"));
    await user.click(within(screen.getByTestId("item-pr-stop-dialog")).getByRole("button", { name: "Stop tracking" }));
    await user.click(within(await screen.findByTestId("pr-undo-toast")).getByRole("button", { name: "Undo" }));
    expect(actions.trackByUrl).toHaveBeenCalledWith("https://github.com/acme/app/pull/23");
  });
});
