import React from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import { legalBoardTransitions } from "@/lib/board/transitions";
import { MoveToMenu } from "./MoveToMenu";

function makeSession(overrides: Partial<Session> & { id: string; title: string }): Session {
  return {
    status: SessionStatus.ACTIVE,
    subStatus: SubStatus.UNSPECIFIED,
    tags: [],
    category: "",
    path: "/tmp/session",
    branch: "",
    program: "claude",
    ...overrides,
  } as unknown as Session;
}

describe("MoveToMenu", () => {
  it("MoveToMenu_should_ListOnlyLegalTargetColumns_When_OpenedForRunningColumnCard", async () => {
    const user = userEvent.setup();
    const session = makeSession({ id: "s1", title: "Fix login bug" });
    const onMove = jest.fn();

    render(<MoveToMenu session={session} currentColumn="running" onMove={onMove} />);

    await user.click(screen.getByTestId("move-to-menu-trigger"));

    const menu = screen.getByTestId("move-to-menu");
    const items = within(menu).getAllByRole("menuitem").map((el) => el.textContent);
    // legalBoardTransitions["running"] === ["paused", "complete"]
    expect(items).toEqual(["Paused", "Complete"]);
    expect(legalBoardTransitions.running).toEqual(["paused", "complete"]);
  });

  it("MoveToMenu_should_ListRunningAsSpecialCase_When_OpenedForNeedsReviewColumnCard", async () => {
    const user = userEvent.setup();
    const session = makeSession({ id: "s2", title: "Review this" });
    const onMove = jest.fn();

    render(<MoveToMenu session={session} currentColumn="needs_review" onMove={onMove} />);

    await user.click(screen.getByTestId("move-to-menu-trigger"));

    const menu = screen.getByTestId("move-to-menu");
    const items = within(menu).getAllByRole("menuitem").map((el) => el.textContent);
    // needs_review -> running is a special case (ApprovalResolution), not a raw entry in
    // legalBoardTransitions itself (which is [] for needs_review).
    expect(items).toEqual(["Running"]);
  });

  it("MoveToMenu_should_ShowNoMovesAvailableMessage_When_CardIsInCompleteColumn", async () => {
    const user = userEvent.setup();
    const session = makeSession({ id: "s3", title: "Done session", status: SessionStatus.STOPPED });
    const onMove = jest.fn();

    render(<MoveToMenu session={session} currentColumn="complete" onMove={onMove} />);

    await user.click(screen.getByTestId("move-to-menu-trigger"));

    expect(screen.getByTestId("move-to-menu-empty")).toHaveTextContent("No moves available");
    expect(screen.queryAllByRole("menuitem")).toHaveLength(0);
  });

  it("MoveToMenu_should_CallOnMoveWithSelectedColumn_When_MenuItemClicked", async () => {
    const user = userEvent.setup();
    const session = makeSession({ id: "s4", title: "Fix login bug" });
    const onMove = jest.fn();

    render(<MoveToMenu session={session} currentColumn="running" onMove={onMove} />);

    await user.click(screen.getByTestId("move-to-menu-trigger"));
    await user.click(screen.getByRole("menuitem", { name: "Paused" }));

    expect(onMove).toHaveBeenCalledTimes(1);
    expect(onMove).toHaveBeenCalledWith("paused");
    // Selecting an item closes the menu.
    expect(screen.queryByTestId("move-to-menu")).not.toBeInTheDocument();
  });

  it("MoveToMenu_should_ExposeAccessibleTriggerAttributes_When_Rendered", () => {
    const session = makeSession({ id: "s5", title: "Fix login bug" });
    const onMove = jest.fn();

    render(<MoveToMenu session={session} currentColumn="running" onMove={onMove} />);

    const trigger = screen.getByTestId("move-to-menu-trigger");
    expect(trigger).toHaveAttribute("aria-haspopup", "menu");
    expect(trigger).toHaveAttribute("aria-label", "Move Fix login bug to another column");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });

  it("MoveToMenu_should_CloseOnEscape_When_MenuIsOpen", async () => {
    const user = userEvent.setup();
    const session = makeSession({ id: "s6", title: "Fix login bug" });
    const onMove = jest.fn();

    render(<MoveToMenu session={session} currentColumn="running" onMove={onMove} />);

    await user.click(screen.getByTestId("move-to-menu-trigger"));
    expect(screen.getByTestId("move-to-menu")).toBeInTheDocument();

    await user.keyboard("{Escape}");

    expect(screen.queryByTestId("move-to-menu")).not.toBeInTheDocument();
  });
});
