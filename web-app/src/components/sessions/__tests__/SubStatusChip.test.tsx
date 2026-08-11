/**
 * Tests for SubStatusChip component.
 *
 * Covers:
 *  - Renders the correct label/aria-label/title for every SubStatus value
 *  - Returns null (renders nothing) for SubStatus.UNSPECIFIED
 *  - Returns null (renders nothing) for undefined/null subStatus (defensive guard,
 *    e.g. when the session proto field is not set or a caller passes a
 *    partially-typed fixture)
 *  - PROCESSING chip's spinner element is aria-hidden
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { SubStatusChip } from "../SubStatusChip";
import { SubStatus } from "@/gen/session/v1/types_pb";

function renderChip(subStatus: SubStatus | undefined | null) {
  return render(<SubStatusChip subStatus={subStatus as SubStatus} />);
}

describe("SubStatusChip", () => {
  it("renders Waiting for Agents chip for WAITING_FOR_AGENT", () => {
    renderChip(SubStatus.WAITING_FOR_AGENT);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Waiting for agents");
    expect(screen.getByText(/Waiting for Agents/)).toBeInTheDocument();
  });

  it("renders processing chip for PROCESSING with an aria-hidden spinner", () => {
    renderChip(SubStatus.PROCESSING);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Session is processing");
    expect(screen.getByText(/Thinking/)).toBeInTheDocument();
    const hidden = document.querySelectorAll('[aria-hidden="true"]');
    expect(hidden.length).toBeGreaterThan(0);
  });

  it("renders Approve Tool Use chip for NEEDS_APPROVAL", () => {
    renderChip(SubStatus.NEEDS_APPROVAL);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Needs approval");
  });

  it("renders Your Input Needed chip for INPUT_REQUIRED", () => {
    renderChip(SubStatus.INPUT_REQUIRED);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Input needed");
  });

  it("renders Error chip for ERROR", () => {
    renderChip(SubStatus.ERROR);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Error");
    expect(screen.getByText(/Error/)).toBeInTheDocument();
  });

  it("renders Tests Failing chip for TESTS_FAILING", () => {
    renderChip(SubStatus.TESTS_FAILING);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Tests failing");
  });

  it("renders Rate Limited chip for RATE_LIMITED", () => {
    renderChip(SubStatus.RATE_LIMITED);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Rate limited");
  });

  it("renders Idle chip for IDLE", () => {
    renderChip(SubStatus.IDLE);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Session is idle");
  });

  it("renders Ready chip for READY", () => {
    renderChip(SubStatus.READY);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Ready for your next instruction");
  });

  it("renders Done chip for SUCCESS", () => {
    renderChip(SubStatus.SUCCESS);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Task complete");
  });

  it("renders nothing for UNSPECIFIED", () => {
    const { container } = renderChip(SubStatus.UNSPECIFIED);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing for undefined subStatus", () => {
    const { container } = renderChip(undefined);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing for null subStatus", () => {
    const { container } = renderChip(null);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing instead of throwing for an unrecognized SubStatus value", () => {
    // Simulates a newer server sending a SubStatus value this client bundle
    // doesn't know about yet — proto enums are forward-compatible, so this
    // must degrade gracefully rather than crash the sessions UI.
    const { container } = renderChip(999 as unknown as SubStatus);
    expect(container).toBeEmptyDOMElement();
  });
});
