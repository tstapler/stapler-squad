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

  it("ChatRefinementPanel_should_ShowOneClarifyingQuestionAtATime_When_MultipleQuestionsPending", async () => {
    const onSend = jest.fn().mockResolvedValue(undefined);
    render(
      <ChatRefinementPanel
        clarifyingQuestions={["What repo does this target?", "Should this include tests?"]}
        onSend={onSend}
      />
    );

    expect(screen.getByTestId("chat-refinement-question")).toHaveTextContent("What repo does this target?");
    expect(screen.queryByText("Should this include tests?")).not.toBeInTheDocument();

    fireEvent.change(screen.getByTestId("chat-refinement-input"), { target: { value: "stapler-squad" } });
    fireEvent.click(screen.getByTestId("chat-refinement-send"));

    await waitFor(() =>
      expect(screen.getByTestId("chat-refinement-question")).toHaveTextContent("Should this include tests?")
    );
  });

  it("ChatRefinementPanel_should_ShowErrorMessage_When_OnSendRejects", async () => {
    const onSend = jest.fn().mockRejectedValue(new Error("triage already running"));
    render(<ChatRefinementPanel clarifyingQuestions={[]} onSend={onSend} />);

    fireEvent.change(screen.getByTestId("chat-refinement-input"), { target: { value: "one more thing" } });
    fireEvent.click(screen.getByTestId("chat-refinement-send"));

    await waitFor(() => expect(screen.getByText("triage already running")).toBeInTheDocument());
  });
});
