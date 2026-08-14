import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ChatRefinementPanel } from "./ChatRefinementPanel";

describe("ChatRefinementPanel", () => {
  it("ChatRefinementPanel_should_CallOnSendWithTypedMessage_When_SendClicked", async () => {
    const onSend = jest.fn().mockResolvedValue(undefined);
    render(<ChatRefinementPanel clarifyingQuestions={[]} onSend={onSend} />);

    fireEvent.change(screen.getByTestId("chat-refinement-input"), { target: { value: "Add dark mode" } });
    fireEvent.click(screen.getByTestId("chat-refinement-send"));

    await waitFor(() => expect(onSend).toHaveBeenCalledWith("Add dark mode"));
  });

  it("ChatRefinementPanel_should_AppendUserAndAssistantTurns_When_MessageSent", async () => {
    const onSend = jest.fn().mockResolvedValue(undefined);
    render(<ChatRefinementPanel clarifyingQuestions={[]} onSend={onSend} />);

    fireEvent.change(screen.getByTestId("chat-refinement-input"), { target: { value: "Add dark mode" } });
    fireEvent.click(screen.getByTestId("chat-refinement-send"));

    await waitFor(() => expect(screen.getByTestId("chat-refinement-transcript")).toBeInTheDocument());
    expect(screen.getByTestId("chat-turn-user")).toHaveTextContent("Add dark mode");
    await waitFor(() => expect(screen.getByTestId("chat-turn-assistant")).toBeInTheDocument());
  });

  it("ChatRefinementPanel_should_DisableSend_When_InputIsEmpty", () => {
    const onSend = jest.fn().mockResolvedValue(undefined);
    render(<ChatRefinementPanel clarifyingQuestions={[]} onSend={onSend} />);
    expect(screen.getByTestId("chat-refinement-send")).toBeDisabled();
  });

  it("ChatRefinementPanel_should_ShowOnlyFirstClarifyingQuestion_When_MultipleQuestionsPending", () => {
    const onSend = jest.fn().mockResolvedValue(undefined);
    render(
      <ChatRefinementPanel
        clarifyingQuestions={["What repo does this target?", "Should this include tests?"]}
        onSend={onSend}
      />
    );

    expect(screen.getByTestId("chat-refinement-question")).toHaveTextContent("What repo does this target?");
    expect(screen.queryByText("Should this include tests?")).not.toBeInTheDocument();
  });

  it("ChatRefinementPanel_should_ShowNewQuestion_When_ParentReplacesClarifyingQuestionsPropAfterTriageCompletes", () => {
    // clarifyingQuestions is fully replaced (not appended to) by the parent once
    // a triage run completes via the page's live item subscription — the panel
    // must always reflect the current prop value, not a stale local index.
    const onSend = jest.fn().mockResolvedValue(undefined);
    const { rerender } = render(
      <ChatRefinementPanel clarifyingQuestions={["What repo does this target?"]} onSend={onSend} />
    );
    expect(screen.getByTestId("chat-refinement-question")).toHaveTextContent("What repo does this target?");

    rerender(<ChatRefinementPanel clarifyingQuestions={["Should this include tests?"]} onSend={onSend} />);
    expect(screen.getByTestId("chat-refinement-question")).toHaveTextContent("Should this include tests?");

    rerender(<ChatRefinementPanel clarifyingQuestions={[]} onSend={onSend} />);
    expect(screen.queryByTestId("chat-refinement-question")).not.toBeInTheDocument();
  });

  it("ChatRefinementPanel_should_ShowErrorMessage_When_OnSendRejects", async () => {
    const onSend = jest.fn().mockRejectedValue(new Error("triage already running"));
    render(<ChatRefinementPanel clarifyingQuestions={[]} onSend={onSend} />);

    fireEvent.change(screen.getByTestId("chat-refinement-input"), { target: { value: "one more thing" } });
    fireEvent.click(screen.getByTestId("chat-refinement-send"));

    await waitFor(() => expect(screen.getByText("triage already running")).toBeInTheDocument());
  });
});
