import fs from "fs";
import path from "path";
import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
import { ActionsDisabledReasonContext } from "../OfflineBanner";
import { makeApprovalDescriptors, makeDecisionDescriptors, makeQuestionDescriptors } from "../__fixtures__/notificationFixtures";
import { invokeMock, renderRow, respondWith } from "../__fixtures__/actionRowHarness";
import { ActionRow, TOUCH_TARGET_PX } from "./ActionRow";

jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

const LIVE = "[aria-live],[role=status],[role=alert]";
const css = fs.readFileSync(path.join(__dirname, "ActionRow.css.ts"), "utf8");

beforeEach(() => invokeMock.mockReset());

describe("ActionRow semantics", () => {
  it("never renders a modal, dialog or sheet in any sub-step", async () => {
    const more = { onCreateRule: jest.fn(), approval: { text: "ls" } };
    renderRow({ descriptors: [...makeApprovalDescriptors(), ...makeDecisionDescriptors()], ...more });
    const none = () => expect(document.querySelectorAll("[role=dialog],[aria-modal=true]")).toHaveLength(0);
    none();
    await userEvent.click(screen.getByTestId("action-deny"));
    none();
    await userEvent.click(screen.getByTestId("action-back"));
    await userEvent.click(screen.getByTestId("action-backlog_decision-merge"));
    none();
    await userEvent.click(screen.getByTestId("action-back"));
    await userEvent.click(screen.getByTestId("action-more"));
    await userEvent.click(screen.getByRole("menuitem"));
    none();
  });

  it("adds no live region of its own, even with a countdown and a delivery line", async () => {
    jest.useFakeTimers();
    respondWith(ActionOutcome.RATE_LIMITED, { retryAfterSeconds: 3 });
    const { container } = render(
      <DeckViewportContext.Provider value={{ isInnerScreen: true, isVirtualKeyboardOpen: false }}>
        <ActionRow rowId="live-1" descriptors={makeApprovalDescriptors()} />
      </DeckViewportContext.Provider>,
    );
    fireEvent.click(screen.getByTestId("action-approve"));
    await act(async () => {});
    expect(screen.getByTestId("action-status")).toBeInTheDocument();
    expect(container.querySelectorAll(LIVE)).toHaveLength(0);
    jest.useRealTimers();
    respondWith(ActionOutcome.APPLIED);
  });

  it("uses aria-disabled, not disabled, so a blocked action keeps its place in the tab order", () => {
    renderRow({ descriptors: makeApprovalDescriptors(), disabledReason: "Offline" });
    for (const b of screen.getAllByRole("button")) expect(b).not.toBeDisabled();
    expect(screen.getByTestId("action-approve")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByTestId("action-open")).not.toHaveAttribute("aria-disabled");
  });

  it("takes the shared connectivity reason as the disabled description", () => {
    const { container } = render(
      <ActionsDisabledReasonContext.Provider value="Reconnecting">
        <ActionRow rowId="ctx-1" descriptors={makeApprovalDescriptors()} />
      </ActionsDisabledReasonContext.Provider>,
    );
    const approve = screen.getByTestId("action-approve");
    expect(approve).toHaveAttribute("aria-disabled", "true");
    expect(container.querySelector(`#${CSS.escape(approve.getAttribute("aria-describedby") as string)}`)).toHaveTextContent("Reconnecting");
  });

  it("labels each option group and names every control", () => {
    renderRow({ descriptors: [...makeDecisionDescriptors(), ...makeQuestionDescriptors()] });
    expect(screen.getByRole("group", { name: "Decide" })).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Answer" })).toBeInTheDocument();
    for (const b of screen.getAllByRole("button")) expect(b).toHaveAccessibleName();
  });

  it("completes every action from click events alone (tap-only operation)", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: makeQuestionDescriptors() }, { mobile: true });
    fireEvent.click(screen.getByTestId("action-answer_question-0"));
    expect(await screen.findByTestId("action-status")).toHaveTextContent("Sent - waiting for the agent");
  });
});

describe("ActionRow phone layout", () => {
  it("gives every option button a 44px minimum height and stacks them", () => {
    renderRow({ descriptors: [...makeQuestionDescriptors(), ...makeDecisionDescriptors()] }, { mobile: true });
    for (const b of screen.getAllByRole("button")) expect(b.style.minHeight).toBe(`${TOUCH_TARGET_PX}px`);
    expect(css).toMatch(/flexDirection: "column"/);
  });

  it("keeps the reason field at 16px and never fixes a height (text spacing, no zoom)", () => {
    expect(css).toMatch(/fontSize: "16px"/);
    expect(css).not.toMatch(/^\s*height:/m);
    expect(css).toMatch(/overflowWrap: "anywhere"/);
  });

  it("has no unconditional animation or transition (reduced motion) and styles forced colors", () => {
    expect(css).not.toMatch(/transition|animation/);
    expect(css).toMatch(/forced-colors: active/);
  });
});
