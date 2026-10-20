import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { makeApprovalDescriptors } from "../__fixtures__/notificationFixtures";
import { invokeMock, renderRow } from "../__fixtures__/actionRowHarness";

jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

const LONG = Array.from({ length: 14 }, (_, i) => `echo line-${i}-${"x".repeat(80)}`).join("\n").slice(0, 1204);
const descriptors = () => makeApprovalDescriptors();

beforeEach(() => invokeMock.mockReset());

describe("approval command display (UX-AC64)", () => {
  it("shows a long command in full in a labelled scroll region with a count and Copy, never ellipsized", () => {
    expect(LONG.length).toBe(1204);
    renderRow({ descriptors: descriptors(), approval: { tool: "Bash", text: LONG } });
    const region = screen.getByRole("region", { name: "Command, scrollable" });
    expect(region).toHaveAttribute("tabindex", "0");
    expect(region.textContent).toBe(LONG);
    expect(screen.getByText("1,204 characters")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy" })).toBeInTheDocument();
    expect(region.style.textOverflow).toBe("");
    expect(screen.getByTestId("action-approve")).toBeInTheDocument();
  });

  it("renders a short command without the scroll region, count or Copy", () => {
    renderRow({ descriptors: descriptors(), approval: { tool: "Bash", text: "git status" } });
    expect(screen.getByText("git status")).toBeInTheDocument();
    expect(screen.queryByRole("region")).toBeNull();
    expect(screen.queryByRole("button", { name: "Copy" })).toBeNull();
  });

  it("fails closed for an input that cannot be shown: no Approve, Open present", () => {
    renderRow({ descriptors: descriptors(), approval: { tool: "Bash", unrenderable: true } });
    expect(screen.getByText(/Can't show this command. Open session to review it./)).toBeInTheDocument();
    expect(screen.queryByTestId("action-approve")).toBeNull();
    expect(screen.getByTestId("action-open")).toBeInTheDocument();
  });
});

describe("More menu and Create rule (UX-AC65)", () => {
  const withRule = { onCreateRule: jest.fn(), approval: { tool: "Bash", text: "git push", ruleScope: "Bash git" } };

  it("orders Approve, Deny, More, Open and keeps Create rule out of the primary row", () => {
    renderRow({ descriptors: descriptors(), ...withRule });
    expect(screen.getAllByRole("button").map((b) => b.textContent)).toEqual(["Approve", "Deny", "More", "Open session"]);
    expect(screen.queryByText("Create rule...")).toBeNull();
  });

  it("creates nothing until the inline confirm is accepted", async () => {
    const onCreateRule = jest.fn();
    renderRow({ descriptors: descriptors(), ...withRule, onCreateRule });
    await userEvent.click(screen.getByTestId("action-more"));
    await userEvent.click(screen.getByRole("menuitem", { name: "Create rule..." }));
    expect(screen.getByText("Create a rule that auto-approves Bash git?")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(onCreateRule).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Create rule" }));
    expect(onCreateRule).toHaveBeenCalledTimes(1);
  });

  it("Back from the confirm returns focus to More", async () => {
    renderRow({ descriptors: descriptors(), ...withRule });
    await userEvent.click(screen.getByTestId("action-more"));
    await userEvent.click(screen.getByRole("menuitem"));
    await userEvent.click(screen.getByTestId("action-back"));
    await waitFor(() => expect(screen.getByTestId("action-more")).toHaveFocus());
  });

  it("is not rendered on a phone", () => {
    renderRow({ descriptors: descriptors(), ...withRule }, { mobile: true });
    expect(screen.queryByTestId("action-more")).toBeNull();
  });

  it("is not rendered on the remote listener", () => {
    renderRow({ descriptors: descriptors(), ...withRule, localListener: false });
    expect(screen.queryByTestId("action-more")).toBeNull();
  });
});
