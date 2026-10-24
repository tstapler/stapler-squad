/**
 * EnforceDialog (Story 1.3.3a/c): evidence region states, gating copy, focus handoffs.
 * Page-level flows (the write, the row confirmation) live in page.test.tsx.
 */

import { createRef } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import {
  ClassReadinessSchema,
  GetWorkflowEngineStatusResponseSchema,
  WorkflowEngineSoakVerdict,
} from "@/gen/session/v1/backlog_pb";
import { EnforceDialog } from "./EnforceReadinessEvidence";

const mockGetStatus = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getWorkflowEngineStatus: (...a: unknown[]) => mockGetStatus(...a) }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const V = WorkflowEngineSoakVerdict;

function status(divergence: bigint, verdict: WorkflowEngineSoakVerdict, stranded = 0n) {
  const r = create(ClassReadinessSchema, { divergence24h: divergence, soakVerdict: verdict, strandedItemCount: stranded });
  return create(GetWorkflowEngineStatusResponseSchema, { classReadiness: { operator: r } });
}

function renderDialog(props: Partial<React.ComponentProps<typeof EnforceDialog>> = {}) {
  const onConfirm = jest.fn();
  const onCancel = jest.fn();
  render(
    <EnforceDialog
      cls="operator"
      flagLabel="Enforce for operators (this UI)"
      returnFocusRef={createRef<HTMLElement>()}
      onConfirm={onConfirm}
      onCancel={onCancel}
      {...props}
    />
  );
  return { onConfirm, onCancel };
}

beforeEach(() => {
  mockGetStatus.mockReset();
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => jest.restoreAllMocks());

describe("EnforceDialog", () => {
  it("shows a busy evidence region, no acknowledgement and a waiting hint while loading", async () => {
    mockGetStatus.mockReturnValue(new Promise(() => {}));
    renderDialog();

    const region = screen.getByTestId("feature-flag-enforce-evidence");
    expect(region).toHaveAttribute("role", "status");
    expect(region).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByTestId("feature-flag-enforce-ack")).toBeNull();
    const enforce = screen.getByTestId("alert-confirm-confirm");
    expect(enforce).toHaveAttribute("aria-disabled", "true");
    expect(enforce).toHaveAttribute("aria-describedby", screen.getByTestId("feature-flag-enforce-hint").id);
    expect(screen.getByTestId("feature-flag-enforce-hint")).toHaveTextContent("Waiting for readiness evidence.");
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByTestId("alert-confirm-cancel")).toHaveFocus();
  });

  it("announces clean evidence once, drops aria-busy, leaves focus on Cancel and shows the checklist pointer", async () => {
    mockGetStatus.mockResolvedValue(status(0n, V.READY));
    renderDialog();

    const region = screen.getByTestId("feature-flag-enforce-evidence");
    await waitFor(() => expect(region).not.toHaveAttribute("aria-busy"));
    expect(region).toHaveTextContent("Readiness evidence: 0 shadow differences, soak verdict READY, 0 items would lose an exit.");
    expect(region).not.toHaveTextContent("Acknowledgement required.");
    expect(screen.getByTestId("alert-confirm-cancel")).toHaveFocus();
    expect(screen.getByTestId("alert-confirm-confirm")).not.toHaveAttribute("aria-disabled");
    expect(screen.queryByTestId("feature-flag-enforce-hint")).toBeNull();
    expect(screen.getByRole("alertdialog")).toHaveTextContent(
      "Rollout checklist: docs/how-to/flip-workflow-engine-enforcement.md"
    );
  });

  it("labels the stranded count as a preview shared by every class, not a per-class number", async () => {
    mockGetStatus.mockResolvedValue(status(0n, V.READY, 4n));
    renderDialog();
    await screen.findByText(/Items that would lose an exit: 4/);
    expect(screen.getByTestId("feature-flag-enforce-evidence-lines")).toHaveTextContent("the same for every caller class");
    expect(screen.getByTestId("feature-flag-enforce-ack")).toBeInTheDocument();
  });

  it("states a floor, not a count, past 200 differences", async () => {
    mockGetStatus.mockResolvedValue(status(200n, V.NOT_READY));
    renderDialog();
    await screen.findByText(/Shadow differences in the last 24h: at least 200/);
  });

  it.each([
    [V.NOT_READY, "NOT READY"],
    [V.NEEDS_OWNER_ACK, "needs your acknowledgement"],
    [V.UNAVAILABLE, "unavailable"],
  ])("never presents a %s verdict as clean", async (verdict, label) => {
    mockGetStatus.mockResolvedValue(status(0n, verdict));
    renderDialog();
    await screen.findByText(new RegExp(`Soak verdict: ${label}`));
    expect(screen.getByTestId("feature-flag-enforce-evidence")).toHaveTextContent("Acknowledgement required.");
    expect(screen.getByTestId("alert-confirm-confirm")).toHaveAttribute("aria-disabled", "true");
  });

  it("makes the whole acknowledgement label the checkbox's hit area", async () => {
    mockGetStatus.mockResolvedValue(status(1n, V.READY));
    renderDialog();
    const ack = await screen.findByTestId("feature-flag-enforce-ack");
    expect(ack.closest("label")).toHaveTextContent("I have reviewed this evidence and the rollout checklist");
    fireEvent.click(screen.getByText("I have reviewed this evidence and the rollout checklist"));
    expect(ack).toBeChecked();
  });

  it("keeps focus on Retry after a repeat failure and moves it to Cancel after a success", async () => {
    mockGetStatus.mockRejectedValue(new Error("down"));
    renderDialog();
    const retry = await screen.findByTestId("feature-flag-enforce-evidence-retry");
    retry.focus();

    fireEvent.click(retry);
    await waitFor(() => expect(mockGetStatus).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(retry).not.toHaveAttribute("aria-disabled"));
    expect(screen.getByTestId("feature-flag-enforce-evidence-retry")).toHaveFocus();

    mockGetStatus.mockResolvedValue(status(0n, V.READY));
    fireEvent.click(screen.getByTestId("feature-flag-enforce-evidence-retry"));
    await waitFor(() => expect(screen.queryByTestId("feature-flag-enforce-evidence-retry")).toBeNull());
    expect(screen.getByTestId("alert-confirm-cancel")).toHaveFocus();
  });

  it("focuses the acknowledgement when a retry loads evidence that is not clean", async () => {
    mockGetStatus.mockRejectedValueOnce(new Error("down"));
    renderDialog();
    const retry = await screen.findByTestId("feature-flag-enforce-evidence-retry");
    mockGetStatus.mockResolvedValue(status(3n, V.NOT_READY));
    fireEvent.click(retry);
    await waitFor(() => expect(screen.getByTestId("feature-flag-enforce-ack")).toHaveFocus());
  });

  it("calls onConfirm only after the re-check passes, and never on a disabled Enforce", async () => {
    mockGetStatus.mockResolvedValue(status(1n, V.READY));
    const { onConfirm } = renderDialog();
    const ack = await screen.findByTestId("feature-flag-enforce-ack");
    fireEvent.click(screen.getByTestId("alert-confirm-confirm"));
    expect(onConfirm).not.toHaveBeenCalled();

    fireEvent.click(ack);
    fireEvent.click(screen.getByTestId("alert-confirm-confirm"));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  });

  it("shows the order advisory without requiring acknowledgement", async () => {
    mockGetStatus.mockResolvedValue(status(0n, V.READY));
    renderDialog({ orderAdvisory: "Operators are not enforced yet; the recommended order is operators first." });
    expect(screen.getByTestId("feature-flag-enforce-order-advisory")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("alert-confirm-confirm")).not.toHaveAttribute("aria-disabled"));
    expect(screen.queryByTestId("feature-flag-enforce-ack")).toBeNull();
  });
});
