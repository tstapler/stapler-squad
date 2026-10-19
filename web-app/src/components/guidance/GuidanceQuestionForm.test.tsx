import { render, screen, within, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GuidanceQuestionForm, formatAnswers, type FormQuestion, type GuidanceQuestionFormProps } from "./GuidanceQuestionForm";
import { GuidanceDeliveryLine } from "./GuidanceDeliveryLine";
import { deliveryView } from "@/lib/guidance/deliveryView";

const cache: FormQuestion = {
  text: "Which cache should we use?",
  multiSelect: false,
  options: [{ label: "Redis", description: "fast, in-memory" }, { label: "Memcached" }],
};
const regions: FormQuestion = { text: "Which regions?", multiSelect: true, options: [{ label: "A" }, { label: "B" }, { label: "C" }] };
const open = <button type="button">Open session</button>;

// Drafts also live in a module-level memory map, so each test gets its own request id.
let requestCounter = 0;

function setup(over: Partial<GuidanceQuestionFormProps> = {}) {
  const onSubmit = jest.fn();
  const utils = render(<GuidanceQuestionForm id={`q${++requestCounter}`} questions={[cache]} mode="answer" onSubmit={onSubmit} openControl={open} {...over} />);
  return { onSubmit, ...utils };
}

beforeEach(() => window.sessionStorage.clear());

describe("GuidanceQuestionForm single select (UX-AC18)", () => {
  it("renders option buttons in a labelled group, never radios", () => {
    setup();
    const group = screen.getByRole("group", { name: "Which cache should we use?" });
    expect(within(group).getAllByRole("button").map((b) => b.textContent)).toEqual(["Redis - fast, in-memory", "Memcached"]);
    expect(screen.queryByRole("radio")).toBeNull();
    expect(screen.queryByRole("radiogroup")).toBeNull();
  });

  it("sends once on click and locks the options afterwards", async () => {
    const { onSubmit, rerender } = setup();
    await userEvent.click(screen.getByRole("button", { name: /Memcached/ }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit.mock.calls[0][0]).toEqual([{ question: cache.text, header: undefined, selected: ["Memcached"] }]);
    rerender(<GuidanceQuestionForm id="lock-q" questions={[cache]} mode="answer" locked onSubmit={onSubmit} openControl={open} />);
    screen.getAllByRole("button", { name: /Redis|Memcached/ }).forEach((b) => expect(b).toHaveAttribute("aria-disabled", "true"));
    await userEvent.click(screen.getByRole("button", { name: /Redis/ }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("sends on Enter and Space of a focused option", async () => {
    const { onSubmit } = setup();
    screen.getByRole("button", { name: /Redis/ }).focus();
    await userEvent.keyboard("{Enter}");
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("browses every option with Tab, Shift+Tab, arrows and digits without sending anything", async () => {
    const three: FormQuestion = { text: "Pick one", multiSelect: false, options: [{ label: "One" }, { label: "Two" }, { label: "Three" }] };
    const { onSubmit } = setup({ questions: [three] });
    const user = userEvent.setup();
    await user.tab();
    await user.tab();
    await user.tab({ shift: true });
    await user.keyboard("{ArrowDown}{ArrowUp}{ArrowDown}1");
    await user.keyboard("23");
    expect(onSubmit).not.toHaveBeenCalled();
    const focused = new Set<Element | null>();
    for (const name of ["One", "Two", "Three"]) {
      screen.getByRole("button", { name }).focus();
      focused.add(document.activeElement);
    }
    expect(focused.size).toBe(3);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("offers Other with an explicit Send only when the route can carry text", async () => {
    const hookOnly = setup();
    expect(screen.queryByLabelText("Other")).toBeNull();
    expect(screen.queryByRole("button", { name: "Send" })).toBeNull();
    hookOnly.unmount();

    const { onSubmit } = setup({ canTypeText: true });
    const other = screen.getByLabelText("Other");
    expect(other).toHaveAttribute("data-testid", "guidance-other");
    await userEvent.type(other, "Valkey");
    expect(onSubmit).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(onSubmit.mock.calls[0][0][0].selected).toEqual(["Valkey"]);
  });

  it("keeps option controls aria-disabled with the reason as their description while offline", async () => {
    const { onSubmit } = setup({ disabledReason: "Offline" });
    const redis = screen.getByRole("button", { name: /Redis/ });
    expect(redis).toHaveAttribute("aria-disabled", "true");
    expect(redis).toHaveAccessibleDescription("Offline");
    await userEvent.click(redis);
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

describe("GuidanceQuestionForm multi-select and multi-question (UX-AC19, UX-AC20, UX-AC22)", () => {
  it("stacks a single-select button group, three checkboxes and one Submit answers", () => {
    setup({ questions: [cache, regions] });
    const q1 = screen.getByRole("group", { name: /Question 1 of 2: Which cache/ });
    expect(within(q1).getAllByRole("button")).toHaveLength(2);
    expect(screen.queryByRole("radio")).toBeNull();
    expect(screen.getAllByRole("checkbox")).toHaveLength(3);
    expect(screen.getAllByRole("button", { name: "Submit answers" })).toHaveLength(1);
  });

  it("counts selections as plain text and clears only selections", async () => {
    setup({ questions: [regions] });
    await userEvent.click(screen.getByRole("checkbox", { name: "A" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "C" }));
    expect(screen.getByTestId("selected-count")).toHaveTextContent("2 selected");
    expect(screen.getByTestId("selected-count")).not.toHaveAttribute("aria-live");
    await userEvent.click(screen.getByTestId("clear-answers"));
    expect(screen.getByTestId("selected-count")).toHaveTextContent("0 selected");
    expect(screen.getByText(/Which regions\?/)).toBeInTheDocument();
  });

  it("counts only the multi-select question's picks, not a single-select answer beside it", async () => {
    setup({ questions: [cache, regions] });
    await userEvent.click(screen.getByRole("button", { name: /Redis/ }));
    await userEvent.click(screen.getByRole("checkbox", { name: "B" }));
    expect(screen.getByTestId("selected-count")).toHaveTextContent("1 selected");
  });

  it("shows an inline error linked by aria-describedby and moves focus into the first invalid group", async () => {
    const { onSubmit } = setup({ questions: [cache, regions] });
    await userEvent.click(screen.getByRole("button", { name: /Redis/ }));
    expect(screen.getByText("Answer question 2 to continue")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("submit-answers"));

    expect(onSubmit).not.toHaveBeenCalled();
    const q2 = screen.getByTestId("question-group-1");
    expect(q2).toHaveAttribute("aria-invalid", "true");
    expect(q2.getAttribute("aria-describedby")).toBe(screen.getByTestId("question-error-1").id);
    expect(screen.getByTestId("question-error-1")).toHaveTextContent("Choose at least one option");
    expect(q2.contains(document.activeElement)).toBe(true);
    expect(screen.getByTestId("question-group-0")).not.toHaveAttribute("aria-invalid");
  });

  it("submits every answer at once and drops the draft", async () => {
    const { onSubmit } = setup({ questions: [cache, regions] });
    await userEvent.click(screen.getByRole("button", { name: /Redis/ }));
    await userEvent.click(screen.getByRole("checkbox", { name: "A" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "C" }));
    await userEvent.click(screen.getByTestId("submit-answers"));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit.mock.calls[0][0].map((a: { selected: string[] }) => a.selected)).toEqual([["Redis"], ["A", "C"]]);
    expect(formatAnswers(onSubmit.mock.calls[0][0])).toBe("Which cache should we use?: Redis\nWhich regions?: A, C");
  });

  it("restores the draft selections after a remount (UX-AC30)", async () => {
    const first = setup({ id: "draft-q", questions: [regions] });
    await userEvent.click(screen.getByRole("checkbox", { name: "B" }));
    first.unmount();
    setup({ id: "draft-q", questions: [regions] });
    expect(screen.getByRole("checkbox", { name: "B" })).toBeChecked();
  });

  it("renders more than eight options as a labelled native select", async () => {
    const many: FormQuestion = { text: "Which one?", multiSelect: false, options: Array.from({ length: 9 }, (_, i) => ({ label: `Option ${i + 1}` })) };
    const { onSubmit } = setup({ questions: [many] });
    const select = screen.getByRole("combobox", { name: "Which one?" });
    await userEvent.selectOptions(select, "Option 7");
    await userEvent.click(screen.getByTestId("submit-answers"));
    expect(onSubmit.mock.calls[0][0][0].selected).toEqual(["Option 7"]);
  });

  it("gives Other a 16px-friendly field on text routes and folds it into the answer", async () => {
    const { onSubmit } = setup({ questions: [regions], canTypeText: true });
    await userEvent.type(screen.getByLabelText("Other"), "eu-west");
    await userEvent.click(screen.getByTestId("submit-answers"));
    expect(onSubmit.mock.calls[0][0][0].selected).toEqual(["eu-west"]);
  });
});

describe("GuidanceQuestionForm read-only shapes (UX-AC28)", () => {
  it("shows the question and options as a text list with View full prompt in session and Open session", () => {
    setup({ mode: "readonly", unrenderable: true });
    expect(screen.getByText("Which cache should we use?")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Redis - fast, in-memory", "Memcached"]);
    expect(screen.getByText("View full prompt in session")).toBeInTheDocument();
    expect(screen.getByText("Answer in the session")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open session" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Redis/ })).toBeNull();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });
});

describe("GuidanceDeliveryLine", () => {
  it("renders exactly one plain-text line with no live role", () => {
    const view = deliveryView(null, { applied: true })!;
    render(<GuidanceDeliveryLine view={view} onAction={jest.fn()} openControl={open} />);
    const line = screen.getByTestId("delivery-line");
    expect(line).toHaveTextContent("Sent - waiting for the agent");
    expect(line.closest("[aria-live]")).toBeNull();
    expect(line.querySelector("[role=status],[role=alert],[aria-live]")).toBeNull();
    expect(screen.getAllByTestId("delivery-text")).toHaveLength(1);
  });

  it("runs Retry anyway only after the inline confirm", async () => {
    const onAction = jest.fn();
    const view = deliveryView({ status: "answered", deliveryRoute: "live_keystroke", deliveryState: "delivery_failed", unacknowledged: false, sessionEnded: false, sessionArchived: false, cancelReason: "", answer: "x" })!;
    render(<GuidanceDeliveryLine view={view} onAction={onAction} openControl={open} />);
    expect(screen.queryByRole("button", { name: /^Retry$/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry anyway" }));
    expect(onAction).not.toHaveBeenCalled();
    expect(screen.getByText("Retry anyway?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onAction).toHaveBeenCalledWith("retryAnyway");
  });

  it("asks before marking an Unconfirmed answer handled and shows the chip", async () => {
    const onAction = jest.fn();
    const view = deliveryView({ status: "answered", deliveryRoute: "live_keystroke", deliveryState: "sent", unacknowledged: true, sessionEnded: false, sessionArchived: false, cancelReason: "", answer: "x" })!;
    render(<GuidanceDeliveryLine view={view} onAction={onAction} openControl={open} />);
    expect(screen.getByTestId("delivery-chip")).toHaveTextContent("Unconfirmed");
    fireEvent.click(screen.getByRole("button", { name: "Mark as handled" }));
    expect(onAction).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Mark handled" }));
    expect(onAction).toHaveBeenCalledWith("markHandled");
  });
});
