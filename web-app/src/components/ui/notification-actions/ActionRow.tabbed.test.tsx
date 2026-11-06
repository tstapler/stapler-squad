import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { makeDescriptor, makeGuidanceRow, makeQuestionDescriptors, makeTabbedGuidanceRow } from "../__fixtures__/notificationFixtures";
import { invokeMock, renderRow, respondWith } from "../__fixtures__/actionRowHarness";

jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

const REDIS_AND_AB = '[["Redis"],["A","B"]]';
const line = () => screen.getByTestId("delivery-text");
const descriptors = () => makeQuestionDescriptors({ targetKey: "row1", options: [{ label: "Redis" }, { label: "A" }] });

async function pickRedisAB() {
  await userEvent.click(screen.getByRole("button", { name: "Redis" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "A" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "B" }));
}

beforeEach(() => invokeMock.mockReset());

describe("ActionRow question card: measured tabbed multi-question dialog", () => {
  it("submits one answer carrying every pick as a JSON list of label lists", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: descriptors(), guidance: makeTabbedGuidanceRow() });
    expect(screen.queryByLabelText("Other")).toBeNull(); // keys cannot carry typed text

    await pickRedisAB();
    await userEvent.click(screen.getByTestId("submit-answers"));

    expect(invokeMock).toHaveBeenCalledTimes(1);
    expect(invokeMock.mock.calls[0][0]).toMatchObject({ targetKey: "row1", params: { answer: REDIS_AND_AB } });
  });

  it("shows the stored sequence as Answered: Redis; A, B once acknowledged", () => {
    renderRow({
      descriptors: [makeDescriptor()],
      guidance: makeTabbedGuidanceRow({ status: "answered", answer: REDIS_AND_AB, deliveryState: "acknowledged" }),
    });
    expect(line()).toHaveTextContent("Answered: Redis; A, B");
  });

  it("keeps an unmeasured multi-question shape read-only", () => {
    const multiFirst = makeTabbedGuidanceRow({ questions: [...makeTabbedGuidanceRow().questions!].reverse() });
    renderRow({ descriptors: descriptors(), guidance: multiFirst });
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.getByText("Answer in the session")).toBeInTheDocument();
  });

  it("keeps a lone multi-select hook question read-only", () => {
    renderRow({ descriptors: descriptors(), guidance: makeGuidanceRow({ questions: [makeTabbedGuidanceRow().questions![1]] }) });
    expect(screen.queryByRole("checkbox")).toBeNull();
  });
});

describe("ActionRow question card: aborted multi-question answer (row 14)", () => {
  const abortedLine = "Part of your answer was sent, then the prompt changed. Open session to finish.";

  it("disables Submit and offers only Open session and a confirmed Dismiss after the server reports it", async () => {
    const announce = jest.fn();
    respondWith(ActionOutcome.STALE, { staleReason: "answer_aborted_partial", detail: "3 of 9 keys sent" });
    renderRow({ descriptors: descriptors(), guidance: makeTabbedGuidanceRow() }, { announce });
    await pickRedisAB();
    await userEvent.click(screen.getByTestId("submit-answers"));

    expect(line()).toHaveTextContent(abortedLine);
    expect(screen.getByTestId("submit-answers")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByTestId("action-open")).toBeInTheDocument();
    expect(screen.queryByTestId("action-delivery-retry")).toBeNull();
    expect(screen.queryByTestId("action-delivery-retryAnyway")).toBeNull();
    expect(announce.mock.calls.map((c) => c[0])).toContain("Answer partly sent");
    await userEvent.click(screen.getByTestId("action-delivery-dismissConfirm"));
    expect(screen.getByText("Dismiss without finishing?")).toBeInTheDocument();
    expect(invokeMock).toHaveBeenCalledTimes(1);
  });

  it("shows the same line from the durable row after a reload", () => {
    renderRow({ descriptors: descriptors(), guidance: makeTabbedGuidanceRow({ deliveryState: "aborted_partial" }) });
    expect(line()).toHaveTextContent(abortedLine);
    expect(screen.getByTestId("submit-answers")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByTestId("delivery-line")).toHaveAttribute("data-delivery-row", "14");
  });
});
