/**
 * Tests for GateChecklist component (Story 2.10.1, Task 2.10.1e).
 *
 * Covers:
 *  1. Multi-gate row rendering: two independent role="status" rows, only the
 *     human-approval row gets Approve/Reject buttons.
 *  2. Approve action: clicking Approve calls onApprove(gateId) and the row
 *     flips to satisfied.
 *  3. Reject action: clicking Reject calls onReject(gateId).
 *  4. Config-error rendering: exact "Configuration error — ..." copy plus the
 *     "Fix in Stages settings" link.
 *  5. Empty gates list renders nothing.
 *  6. A failed Approve shows an inline error scoped to that row only.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GateChecklist, type GateChecklistItem, type GateChecklistProps } from "./GateChecklist";

function makeProps(overrides: Partial<GateChecklistProps> = {}): GateChecklistProps {
  return {
    gates: [],
    transitionLabel: "Review to Design Review",
    onApprove: jest.fn().mockResolvedValue(undefined),
    onReject: jest.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

const humanApprovalGate: GateChecklistItem = {
  gateId: "gate-human-1",
  kind: "human_approval",
  satisfied: false,
};

const structuralGate: GateChecklistItem = {
  gateId: "gate-structural-1",
  kind: "structural",
  satisfied: false,
  description: "2 of 5 acceptance criteria incomplete",
};

// ---------------------------------------------------------------------------
// Test: 1 — multi-gate row rendering
// ---------------------------------------------------------------------------

describe("GateChecklist — multi-gate row rendering", () => {
  it("renders two independent status rows, Approve/Reject only on the human-approval row", () => {
    render(<GateChecklist {...makeProps({ gates: [humanApprovalGate, structuralGate] })} />);

    const rows = screen.getAllByRole("status");
    expect(rows).toHaveLength(2);

    expect(screen.getByText("Human approval")).toBeInTheDocument();
    expect(screen.getByText("Structural check")).toBeInTheDocument();
    expect(screen.getByText("2 of 5 acceptance criteria incomplete")).toBeInTheDocument();

    expect(screen.getByRole("button", { name: /Approve human-approval gate/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Reject human-approval gate/i })).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 2 — Approve action
// ---------------------------------------------------------------------------

describe("GateChecklist — Approve action", () => {
  it("calls onApprove with the gate id and flips the row to satisfied", async () => {
    const user = userEvent.setup();
    const onApprove = jest.fn().mockResolvedValue(undefined);
    render(<GateChecklist {...makeProps({ gates: [humanApprovalGate], onApprove })} />);

    await user.click(screen.getByRole("button", { name: /Approve human-approval gate/i }));

    expect(onApprove).toHaveBeenCalledWith("gate-human-1");
    expect(await screen.findByText("Satisfied")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Approve human-approval gate/i })).not.toBeInTheDocument();
  });

  it("shows a row-scoped inline error and re-enables the buttons when the RPC fails", async () => {
    const user = userEvent.setup();
    const onApprove = jest.fn().mockRejectedValue(new Error("network error"));
    render(<GateChecklist {...makeProps({ gates: [humanApprovalGate], onApprove })} />);

    await user.click(screen.getByRole("button", { name: /Approve human-approval gate/i }));

    expect(await screen.findByText(/network error/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Approve human-approval gate/i })).not.toBeDisabled();
  });
});

// ---------------------------------------------------------------------------
// Test: 3 — Reject action
// ---------------------------------------------------------------------------

describe("GateChecklist — Reject action", () => {
  it("calls onReject with the gate id", async () => {
    const user = userEvent.setup();
    const onReject = jest.fn().mockResolvedValue(undefined);
    render(<GateChecklist {...makeProps({ gates: [humanApprovalGate], onReject })} />);

    await user.click(screen.getByRole("button", { name: /Reject human-approval gate/i }));

    expect(onReject).toHaveBeenCalledWith("gate-human-1");
  });
});

// ---------------------------------------------------------------------------
// Test: 4 — config-error rendering
// ---------------------------------------------------------------------------

describe("GateChecklist — config-error rendering", () => {
  it("renders the exact configuration-error copy and a Fix-in-Stages-settings link", () => {
    const configErrorGate: GateChecklistItem = {
      gateId: "gate-custom-1",
      kind: "automated_review",
      satisfied: false,
      configError: "referenced pipeline mode not found",
    };

    render(
      <GateChecklist
        {...makeProps({
          gates: [configErrorGate],
          stagesSettingsHref: "/settings/backlog-stages?stage=design-review",
        })}
      />,
    );

    expect(
      screen.getByText(
        "Configuration error — this gate can't be evaluated (referenced pipeline mode not found)",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Fix in Stages settings/i })).toHaveAttribute(
      "href",
      "/settings/backlog-stages?stage=design-review",
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 5 — empty gates list
// ---------------------------------------------------------------------------

describe("GateChecklist — no pending gates", () => {
  it("renders nothing when the gates list is empty", () => {
    const { container } = render(<GateChecklist {...makeProps({ gates: [] })} />);
    expect(container).toBeEmptyDOMElement();
  });
});
