/**
 * Tests for TriggersPanel (webhook-triggers Epic 7.1).
 *
 * Covers: type badges/filtering, last-fired rendering, and enable/disable toggle
 * wiring to updateWorkflow (AC7). TriggerFormModal and TriggerExecutionHistory are
 * mocked to keep this test focused on list behavior.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TriggersPanel } from "./TriggersPanel";
import { WorkflowProto } from "@/gen/session/v1/session_pb";

const mockRefresh = jest.fn();
const mockUpdateWorkflow = jest.fn().mockResolvedValue(undefined);
const mockCreateWorkflow = jest.fn().mockResolvedValue(undefined);

let mockWorkflows: Partial<WorkflowProto>[] = [];

jest.mock("@/lib/hooks/useWorkflows", () => ({
  useWorkflows: () => ({
    workflows: mockWorkflows,
    loading: false,
    error: null,
    createWorkflow: mockCreateWorkflow,
    updateWorkflow: mockUpdateWorkflow,
    deleteWorkflow: jest.fn(),
    archiveWorkflowSessions: jest.fn(),
    deleteWorkflowFailedSessions: jest.fn(),
    refresh: mockRefresh,
  }),
}));

jest.mock("./TriggerFormModal", () => ({
  TriggerFormModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="trigger-form-modal-stub" /> : null,
}));

jest.mock("./TriggerExecutionHistory", () => ({
  TriggerExecutionHistory: ({ workflowId }: { workflowId: string }) => (
    <div data-testid="history-stub">{workflowId}</div>
  ),
}));

function makeWorkflow(overrides: Partial<WorkflowProto> = {}): WorkflowProto {
  return {
    id: "wf-1",
    slug: "jira-ticket",
    name: "Triage tickets",
    triggerType: "webhook",
    cronEnabled: true,
    githubRepo: "",
    githubBranch: "",
    webhookSlug: "jira-ticket",
    eventFilter: "",
    labelFilter: "",
    promptTemplate: "",
    cronExpression: "",
    ...overrides,
  } as unknown as WorkflowProto;
}

describe("TriggersPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockWorkflows = [];
  });

  it("TriggersPanel_should_showEmptyState_When_noTriggersConfigured", () => {
    render(<TriggersPanel />);
    expect(screen.getByTestId("triggers-empty-state")).toBeInTheDocument();
  });

  it("TriggersPanel_should_excludeManualWorkflows_When_listingTriggers", () => {
    mockWorkflows = [
      makeWorkflow({ id: "wf-manual", triggerType: "manual", slug: "manual-one" }),
      makeWorkflow({ id: "wf-webhook", triggerType: "webhook", slug: "webhook-one" }),
    ];
    render(<TriggersPanel />);
    // Rendered once in the desktop table row and once in the mobile card row —
    // both exist simultaneously in jsdom since @media queries aren't evaluated
    // by the test environment (CSS isn't actually applied), so assert >=1 match.
    expect(screen.getAllByText("webhook-one").length).toBeGreaterThan(0);
    expect(screen.queryByText("manual-one")).not.toBeInTheDocument();
  });

  it("TriggersPanel_should_filterByType_When_tabClicked", () => {
    mockWorkflows = [
      makeWorkflow({ id: "wf-cron", triggerType: "cron", slug: "cron-one" }),
      makeWorkflow({ id: "wf-webhook", triggerType: "webhook", slug: "webhook-one" }),
    ];
    render(<TriggersPanel />);
    expect(screen.getAllByText("cron-one").length).toBeGreaterThan(0);
    expect(screen.getAllByText("webhook-one").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByTestId("trigger-tab-cron"));
    expect(screen.getAllByText("cron-one").length).toBeGreaterThan(0);
    expect(screen.queryByText("webhook-one")).not.toBeInTheDocument();
  });

  it("TriggersPanel_should_showNeverFired_When_lastFiredAtUnset", () => {
    mockWorkflows = [makeWorkflow({ lastFiredAt: undefined })];
    render(<TriggersPanel />);
    expect(screen.getAllByText("Never fired").length).toBeGreaterThan(0);
  });

  it("TriggersPanel_should_showRowDisabled_When_triggerNotEnabled", () => {
    mockWorkflows = [makeWorkflow({ cronEnabled: false })];
    render(<TriggersPanel />);
    expect(screen.getByTestId(`trigger-toggle-wf-1`)).toHaveTextContent("OFF");
  });

  it("TriggersPanel_should_callUpdateWorkflowWithInvertedCronEnabled_When_toggleClicked", async () => {
    mockWorkflows = [makeWorkflow({ id: "wf-1", cronEnabled: true })];
    render(<TriggersPanel />);

    fireEvent.click(screen.getByTestId("trigger-toggle-wf-1"));

    await waitFor(() => expect(mockUpdateWorkflow).toHaveBeenCalledTimes(1));
    expect(mockUpdateWorkflow).toHaveBeenCalledWith("wf-1", { cronEnabled: false });
  });

  it("TriggersPanel_should_showVisibleError_When_toggleFails", async () => {
    mockWorkflows = [makeWorkflow({ id: "wf-1", cronEnabled: true })];
    mockUpdateWorkflow.mockRejectedValueOnce(new Error("network error"));
    render(<TriggersPanel />);

    fireEvent.click(screen.getByTestId("trigger-toggle-wf-1"));

    await waitFor(() => expect(screen.getByTestId("trigger-toggle-error")).toBeInTheDocument());
    expect(screen.getByTestId("trigger-toggle-error")).toHaveTextContent(/Failed to disable/);
  });

  it("TriggersPanel_should_openFormModal_When_addTriggerClicked", () => {
    render(<TriggersPanel />);
    expect(screen.queryByTestId("trigger-form-modal-stub")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("add-trigger-button"));
    expect(screen.getByTestId("trigger-form-modal-stub")).toBeInTheDocument();
  });

  it("TriggersPanel_should_expandExecutionHistory_When_historyToggleClicked", () => {
    mockWorkflows = [makeWorkflow({ id: "wf-1" })];
    render(<TriggersPanel />);
    expect(screen.queryAllByTestId("history-stub").length).toBe(0);
    fireEvent.click(screen.getByTestId("trigger-history-toggle-wf-1"));
    // Rendered in both the desktop table row and the mobile card (see note above).
    expect(screen.getAllByTestId("history-stub").length).toBeGreaterThan(0);
  });
});
