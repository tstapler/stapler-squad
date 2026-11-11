import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { WorkflowProtoSchema } from "@/gen/session/v1/session_pb";
import { WorkflowsPanel } from "./WorkflowsPanel";

const push = jest.fn();
const runWorkflow = jest.fn();

jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.memory"),
  useRouter: () => ({ push, replace: jest.fn(), back: jest.fn() }),
}));
jest.mock("@/lib/contexts/AnalyticsContext", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ runWorkflow, listSessionsByWorkflow: jest.fn().mockResolvedValue([]), archiveWorkflowSessions: jest.fn(), deleteWorkflowFailedSessions: jest.fn() }),
}));

const mockWorkflows = [
  create(WorkflowProtoSchema, { id: "wf-input", slug: "with-input", name: "With Input", command: "plan {{input}}" }),
  create(WorkflowProtoSchema, { id: "wf-plain", slug: "plain", name: "Plain", command: "do the thing" }),
];
jest.mock("@/lib/hooks/useWorkflows", () => ({
  useWorkflows: () => ({
    workflows: mockWorkflows,
    loading: false,
    error: null,
    createWorkflow: jest.fn(),
    updateWorkflow: jest.fn(),
    deleteWorkflow: jest.fn(),
    refresh: jest.fn(),
  }),
}));

function runButtonFor(name: string) {
  const row = screen.getByText(name).closest("tr") as HTMLElement;
  return row.querySelector("button") as HTMLButtonElement;
}

describe("WorkflowsPanel run", () => {
  beforeEach(() => {
    push.mockClear();
    runWorkflow.mockReset().mockResolvedValue("sess-1");
  });

  it("runs a workflow without {{input}} immediately, with no dialog", async () => {
    render(<WorkflowsPanel />);
    fireEvent.click(runButtonFor("Plain"));
    await waitFor(() => expect(runWorkflow).toHaveBeenCalledWith({ id: "wf-plain", arg: undefined, attachmentPaths: undefined }));
    expect(screen.queryByTestId("workflow-run-dialog")).not.toBeInTheDocument();
    await waitFor(() => expect(push).toHaveBeenCalledWith("/?session=sess-1"));
  });

  it("opens the Run dialog for a workflow with {{input}} and runs with the entered input", async () => {
    render(<WorkflowsPanel />);
    fireEvent.click(runButtonFor("With Input"));
    expect(runWorkflow).not.toHaveBeenCalled();
    expect(screen.getByTestId("workflow-run-dialog")).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("workflow-run-input"), { target: { value: "a shed" } });
    fireEvent.click(screen.getByTestId("workflow-run-submit"));

    await waitFor(() =>
      expect(runWorkflow).toHaveBeenCalledWith({ id: "wf-input", arg: "a shed", attachmentPaths: [] })
    );
    await waitFor(() => expect(push).toHaveBeenCalledWith("/?session=sess-1"));
    await waitFor(() => expect(screen.queryByTestId("workflow-run-dialog")).not.toBeInTheDocument());
  });

  it("keeps the dialog open when the run fails", async () => {
    runWorkflow.mockResolvedValue(null);
    render(<WorkflowsPanel />);
    fireEvent.click(runButtonFor("With Input"));
    fireEvent.click(screen.getByTestId("workflow-run-submit"));
    await waitFor(() => expect(runWorkflow).toHaveBeenCalled());
    expect(push).not.toHaveBeenCalled();
    expect(screen.getByTestId("workflow-run-dialog")).toBeInTheDocument();
  });
});
