import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { makeDescriptor, makeGuidanceDescriptors, makeGuidanceRow, makeQuestionDescriptors } from "../__fixtures__/notificationFixtures";
import { invokeMock, respondWith } from "../__fixtures__/actionRowHarness";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
import { ActionRow, type ActionRowProps } from "./ActionRow";

jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

const sentRow = (over = {}) => makeGuidanceRow({ status: "answered", answer: "Redis", deliveryState: "sent", ...over });
const line = () => screen.getByTestId("delivery-text");

beforeEach(() => invokeMock.mockReset());

describe("ActionRow question card: hook-mirrored single select", () => {
  it("sends the chosen option once and walks sent -> acknowledged without a second control", async () => {
    respondWith(ActionOutcome.APPLIED);
    const props = { descriptors: makeQuestionDescriptors(), guidance: makeGuidanceRow() };
    const { rerender } = renderRowWith(props);
    expect(screen.queryByLabelText("Other")).toBeNull();
    expect(screen.queryByRole("button", { name: "Send" })).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Redis" }));
    expect(invokeMock).toHaveBeenCalledTimes(1);
    expect(invokeMock.mock.calls[0][0]).toMatchObject({ params: { answer: "Redis" }, targetKey: "q1" });
    expect(line()).toHaveTextContent("Sent - waiting for the agent");

    rerender({ ...props, guidance: sentRow() });
    expect(line()).toHaveTextContent("Sent - waiting for the agent");
    rerender({ ...props, guidance: sentRow({ deliveryState: "acknowledged" }) });
    expect(line()).toHaveTextContent("Answered: Redis");
    expect(screen.getAllByTestId("delivery-line")).toHaveLength(1);
  });

  it("shows NOT_SENT with Retry and Open session, and the human_active wording", async () => {
    respondWith(ActionOutcome.NOT_SENT, { detail: "human_active" });
    renderRowWith({ descriptors: makeQuestionDescriptors(), guidance: makeGuidanceRow() });
    await userEvent.click(screen.getByRole("button", { name: "Redis" }));
    expect(line()).toHaveTextContent("Not sent: someone is typing in that session. Retry when they are done.");
    expect(screen.getByTestId("action-delivery-retry")).toBeInTheDocument();
    expect(screen.getByTestId("action-open")).toBeInTheDocument();
  });

  it("shows SEND_INDETERMINATE as row 6 with Unconfirmed, Open session and Retry anyway behind a confirm", async () => {
    respondWith(ActionOutcome.SEND_INDETERMINATE);
    renderRowWith({ descriptors: makeQuestionDescriptors(), guidance: makeGuidanceRow() });
    await userEvent.click(screen.getByRole("button", { name: "Redis" }));
    expect(line()).toHaveTextContent("Sent, but we couldn't confirm it arrived. Check the session before retrying.");
    expect(screen.getByTestId("delivery-chip")).toHaveTextContent("Unconfirmed");
    expect(screen.queryByTestId("action-delivery-retry")).toBeNull();
    await userEvent.click(screen.getByTestId("action-delivery-retryAnyway"));
    expect(screen.getByText("Retry anyway?")).toBeInTheDocument();
  });

  it("marks an Unconfirmed answer handled only after the inline confirm", async () => {
    const onMarkHandled = jest.fn();
    renderRowWith({ descriptors: [makeDescriptor()], guidance: sentRow({ unacknowledged: true }), onMarkHandled });
    expect(screen.getByTestId("delivery-chip")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("action-delivery-markHandled"));
    expect(onMarkHandled).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Mark handled" }));
    expect(onMarkHandled).toHaveBeenCalledTimes(1);
  });

  it("announces one result per distinct delivery transition and none for repeats or a late acknowledgement", () => {
    const announce = jest.fn();
    const props = { descriptors: [makeDescriptor()], guidance: sentRow() };
    const { rerender } = renderRowWith(props, announce);
    rerender({ ...props, guidance: sentRow({ unacknowledged: true }) });
    rerender({ ...props, guidance: sentRow({ unacknowledged: true }) });
    rerender({ ...props, guidance: sentRow({ deliveryState: "acknowledged" }) });
    const said = announce.mock.calls.map((c) => c[0]);
    expect(said).toEqual(["Answer not confirmed"]);
  });

  it("flips an open card to Already handled on a terminal answer: one polite announcement, focus unchanged (UX-AC26)", async () => {
    const announce = jest.fn();
    const props = { descriptors: makeQuestionDescriptors(), guidance: makeGuidanceRow() };
    const { rerender } = renderRowWith(props, announce);
    const redis = screen.getByRole("button", { name: "Redis" });
    redis.focus();
    expect(document.activeElement).toBe(redis);
    announce.mockClear();

    rerender({ descriptors: [makeDescriptor()], guidance: makeGuidanceRow({ status: "cancelled", cancelReason: "answered_in_terminal" }) });

    expect(line()).toHaveTextContent("Already handled in the session. No action needed.");
    expect(document.activeElement).toBe(redis);
    expect(redis).toHaveAttribute("aria-disabled", "true");
    expect(announce.mock.calls.map((c) => c[0])).toEqual(["Already handled in the session. No action needed."]);
    expect(announce.mock.calls[0][1]).toBe("polite");
  });

  it("flips to The session is now asking something else when the dialog was replaced", () => {
    const props = { descriptors: makeQuestionDescriptors(), guidance: makeGuidanceRow() };
    const { rerender } = renderRowWith(props);
    rerender({ descriptors: [makeDescriptor()], guidance: makeGuidanceRow({ status: "cancelled", cancelReason: "dialog_replaced" }) });
    expect(line()).toHaveTextContent("The session is now asking something else. Open session to answer it.");
  });

  it("shows row 17, never the Session ended before this was answered string, when a sent answer's session ended", () => {
    renderRowWith({ descriptors: [makeDescriptor()], guidance: sentRow({ sessionEnded: true }) });
    expect(line()).toHaveTextContent("Sent, but the session ended before confirming it.");
    expect(line()).not.toHaveTextContent("before this was answered");
  });

  it("degrades to a text list with Answer in the session when the canary withdrew the control", () => {
    const [d, open] = makeQuestionDescriptors({ available: false, unavailableReason: "read_only" });
    renderRowWith({ descriptors: [d, open], guidance: makeGuidanceRow() });
    expect(screen.queryByRole("button", { name: "Redis" })).toBeNull();
    expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Redis", "Memcached"]);
    expect(screen.getByText("Answer in the session")).toBeInTheDocument();
  });

  it("reads Answer from desktop on the remote listener while keeping the options visible but disabled", () => {
    const { container } = renderRowWith({ descriptors: makeQuestionDescriptors({ available: false, unavailableReason: "desktop_only" }), guidance: makeGuidanceRow() });
    expect(within(container).getByRole("button", { name: "Redis" })).toHaveAttribute("aria-disabled", "true");
    expect(invokeMock).not.toHaveBeenCalled();
  });

  it("renders a multi-select hook question as read-only text because a keystroke cannot answer it", () => {
    const multi = makeGuidanceRow({ questions: [{ question: "Which regions?", header: "R", multiSelect: true, options: [{ label: "A", description: "" }, { label: "B", description: "" }] }] });
    renderRowWith({ descriptors: makeQuestionDescriptors(), guidance: multi });
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.getByText("Answer in the session")).toBeInTheDocument();
  });
});

describe("ActionRow question card: agent-created durable-record row", () => {
  const agent = (over = {}) => makeGuidanceRow({ source: "agent_mcp", deliveryRoute: "", ...over });

  it("offers Other with an explicit Send and sends the typed text", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRowWith({ descriptors: makeGuidanceDescriptors(), guidance: agent() });
    await userEvent.type(screen.getByLabelText("Other"), "Valkey");
    expect(invokeMock).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(invokeMock.mock.calls[0][0]).toMatchObject({ params: { answer: "Valkey" }, targetKey: "g1" });
  });

  it("submits a multi-select and a second question together as one formatted answer", async () => {
    respondWith(ActionOutcome.APPLIED);
    const two = agent({
      questions: [
        { question: "Cache?", header: "Cache", multiSelect: false, options: [{ label: "Redis", description: "" }, { label: "Memcached", description: "" }] },
        { question: "Regions?", header: "Regions", multiSelect: true, options: [{ label: "A", description: "" }, { label: "B", description: "" }, { label: "C", description: "" }] },
      ],
    });
    renderRowWith({ descriptors: makeGuidanceDescriptors(), guidance: two });
    await userEvent.click(screen.getByRole("button", { name: "Redis" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "A" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "C" }));
    await userEvent.click(screen.getByTestId("submit-answers"));
    expect(invokeMock.mock.calls[0][0].params.answer).toBe("Cache: Redis\nRegions: A, C");
  });

  it("shows the archived copy before the operator acts and still sends", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRowWith({ descriptors: makeGuidanceDescriptors(), guidance: agent({ sessionArchived: true }) });
    expect(line()).toHaveTextContent("This session is archived. Your answer will be saved but not delivered unless you resume it.");
    await act(async () => {});
    await userEvent.click(screen.getByRole("button", { name: "Redis" }));
    expect(invokeMock).toHaveBeenCalledTimes(1);
  });
});

type RowProps = Omit<ActionRowProps, "rowId">;

/** Renders one row with a stable id and a rerender that swaps props, so a snapshot can change under it. */
function renderRowWith(props: RowProps, announce: jest.Mock = jest.fn()) {
  const tree = (p: RowProps) => (
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <DeckViewportContext.Provider value={{ isInnerScreen: true, isVirtualKeyboardOpen: false }}>
        <ActionRow rowId="guidance-row" onOpen={() => {}} {...p} />
      </DeckViewportContext.Provider>
    </AnnouncerContext.Provider>
  );
  const view = render(tree(props));
  return { ...view, rerender: (next: RowProps) => view.rerender(tree(next)) };
}
