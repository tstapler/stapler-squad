/**
 * Tests for StatusBadge component and getAttentionReasonInfo helper.
 *
 * Covers:
 *  - Renders with AttentionReason: shows correct label and aria-label
 *  - Renders with detectedStatus typed enum: shows correct label
 *  - Returns null when neither reason nor detectedStatus given
 *  - Icon has aria-hidden="true"
 *  - Title shows context when provided
 *  - WAITING_FOR_USER and INPUT_REQUIRED render successfully
 *  - getAttentionReasonInfo: maps all known reasons to non-empty labels
 *  - getDetectedStatusInfo: maps all known DetectedStatus values correctly
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { StatusBadge, getAttentionReasonInfo, getDetectedStatusInfo } from "../StatusBadge";
import { AttentionReason, DetectedStatus } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderBadge(props: Parameters<typeof StatusBadge>[0]) {
  return render(<StatusBadge {...props} />);
}

// ---------------------------------------------------------------------------
// AttentionReason rendering
// ---------------------------------------------------------------------------

describe("StatusBadge with AttentionReason", () => {
  it("renders Approval Pending label for APPROVAL_PENDING", () => {
    renderBadge({ reason: AttentionReason.APPROVAL_PENDING });
    expect(screen.getByText("Approval Pending")).toBeInTheDocument();
  });

  it("renders Input Required label for INPUT_REQUIRED", () => {
    renderBadge({ reason: AttentionReason.INPUT_REQUIRED });
    expect(screen.getByText("Input Required")).toBeInTheDocument();
  });

  it("renders Error label for ERROR_STATE", () => {
    renderBadge({ reason: AttentionReason.ERROR_STATE });
    expect(screen.getByText("Error")).toBeInTheDocument();
  });

  it("renders Idle label for IDLE", () => {
    renderBadge({ reason: AttentionReason.IDLE });
    expect(screen.getByText("Idle")).toBeInTheDocument();
  });

  it("renders Idle label for IDLE_TIMEOUT", () => {
    renderBadge({ reason: AttentionReason.IDLE_TIMEOUT });
    expect(screen.getByText("Idle")).toBeInTheDocument();
  });

  it("renders Complete label for TASK_COMPLETE", () => {
    renderBadge({ reason: AttentionReason.TASK_COMPLETE });
    expect(screen.getByText("Complete")).toBeInTheDocument();
  });

  it("renders Uncommitted Changes label for UNCOMMITTED_CHANGES", () => {
    renderBadge({ reason: AttentionReason.UNCOMMITTED_CHANGES });
    expect(screen.getByText("Uncommitted Changes")).toBeInTheDocument();
  });

  it("renders Stale label for STALE", () => {
    renderBadge({ reason: AttentionReason.STALE });
    expect(screen.getByText("Stale")).toBeInTheDocument();
  });

  it("renders Your Input Needed label for WAITING_FOR_USER", () => {
    renderBadge({ reason: AttentionReason.WAITING_FOR_USER });
    expect(screen.getByText("Your Input Needed")).toBeInTheDocument();
  });

  it("sets aria-label matching the reason label", () => {
    renderBadge({ reason: AttentionReason.APPROVAL_PENDING });
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Approval Pending");
  });

  it("icon span has aria-hidden", () => {
    renderBadge({ reason: AttentionReason.ERROR_STATE });
    const icons = document.querySelectorAll('[aria-hidden="true"]');
    expect(icons.length).toBeGreaterThan(0);
  });

  it("title shows context string when provided", () => {
    renderBadge({ reason: AttentionReason.ERROR_STATE, context: "Build failed" });
    expect(screen.getByRole("status")).toHaveAttribute("title", "Build failed");
  });

  it("title falls back to label when no context or title prop", () => {
    renderBadge({ reason: AttentionReason.IDLE });
    expect(screen.getByRole("status")).toHaveAttribute("title", "Idle");
  });
});

// ---------------------------------------------------------------------------
// detectedStatus typed enum rendering
// ---------------------------------------------------------------------------

describe("StatusBadge with detectedStatus", () => {
  it("renders Ready label for DetectedStatus.READY", () => {
    renderBadge({ detectedStatus: DetectedStatus.READY });
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  it("renders Tests Failing label for DetectedStatus.TESTS_FAILING", () => {
    renderBadge({ detectedStatus: DetectedStatus.TESTS_FAILING });
    expect(screen.getByText("Tests Failing")).toBeInTheDocument();
  });

  it("renders Processing label for DetectedStatus.PROCESSING", () => {
    renderBadge({ detectedStatus: DetectedStatus.PROCESSING });
    expect(screen.getByText("Processing")).toBeInTheDocument();
  });

  it("renders Executing label for DetectedStatus.EXECUTING", () => {
    renderBadge({ detectedStatus: DetectedStatus.EXECUTING });
    expect(screen.getByText("Executing")).toBeInTheDocument();
  });

  it("renders Needs Approval label for DetectedStatus.NEEDS_APPROVAL", () => {
    renderBadge({ detectedStatus: DetectedStatus.NEEDS_APPROVAL });
    expect(screen.getByText("Needs Approval")).toBeInTheDocument();
  });

  it("renders Input Required label for DetectedStatus.INPUT_REQUIRED", () => {
    renderBadge({ detectedStatus: DetectedStatus.INPUT_REQUIRED });
    expect(screen.getByText("Input Required")).toBeInTheDocument();
  });

  it("renders null (nothing) for DetectedStatus.UNKNOWN", () => {
    const { container } = renderBadge({ detectedStatus: DetectedStatus.UNKNOWN });
    expect(container).toBeEmptyDOMElement();
  });

  it("renders null (nothing) for DetectedStatus.UNSPECIFIED", () => {
    const { container } = renderBadge({ detectedStatus: DetectedStatus.UNSPECIFIED });
    expect(container).toBeEmptyDOMElement();
  });
});

// ---------------------------------------------------------------------------
// Null case
// ---------------------------------------------------------------------------

describe("StatusBadge null case", () => {
  it("renders nothing when neither reason nor detectedStatus given", () => {
    const { container } = renderBadge({});
    expect(container).toBeEmptyDOMElement();
  });
});

// ---------------------------------------------------------------------------
// getAttentionReasonInfo helper
// ---------------------------------------------------------------------------

describe("getAttentionReasonInfo", () => {
  const knownReasons = [
    AttentionReason.APPROVAL_PENDING,
    AttentionReason.INPUT_REQUIRED,
    AttentionReason.ERROR_STATE,
    AttentionReason.IDLE,
    AttentionReason.IDLE_TIMEOUT,
    AttentionReason.TASK_COMPLETE,
    AttentionReason.UNCOMMITTED_CHANGES,
    AttentionReason.STALE,
    AttentionReason.WAITING_FOR_USER,
  ];

  knownReasons.forEach((reason) => {
    it(`returns non-empty label for reason ${reason}`, () => {
      const info = getAttentionReasonInfo(reason);
      expect(info.label).toBeTruthy();
      expect(info.icon).toBeTruthy();
      expect(info.variant).toBeTruthy();
    });
  });
});

// ---------------------------------------------------------------------------
// getDetectedStatusInfo helper
// ---------------------------------------------------------------------------

describe("getDetectedStatusInfo", () => {
  const badgeStatuses = [
    { status: DetectedStatus.READY, expectedLabel: "Ready" },
    { status: DetectedStatus.PROCESSING, expectedLabel: "Processing" },
    { status: DetectedStatus.EXECUTING, expectedLabel: "Executing" },
    { status: DetectedStatus.NEEDS_APPROVAL, expectedLabel: "Needs Approval" },
    { status: DetectedStatus.INPUT_REQUIRED, expectedLabel: "Input Required" },
    { status: DetectedStatus.ERROR, expectedLabel: "Error" },
    { status: DetectedStatus.TESTS_FAILING, expectedLabel: "Tests Failing" },
    { status: DetectedStatus.IDLE, expectedLabel: "Idle" },
    { status: DetectedStatus.SUCCESS, expectedLabel: "Success" },
    { status: DetectedStatus.WAITING_FOR_AGENT, expectedLabel: "Waiting for Agent" },
  ];

  badgeStatuses.forEach(({ status, expectedLabel }) => {
    it(`returns label "${expectedLabel}" for DetectedStatus.${DetectedStatus[status]}`, () => {
      const info = getDetectedStatusInfo(status);
      expect(info).not.toBeNull();
      expect(info!.label).toBe(expectedLabel);
    });
  });

  it("returns null for DetectedStatus.UNKNOWN", () => {
    expect(getDetectedStatusInfo(DetectedStatus.UNKNOWN)).toBeNull();
  });

  it("returns null for DetectedStatus.UNSPECIFIED", () => {
    expect(getDetectedStatusInfo(DetectedStatus.UNSPECIFIED)).toBeNull();
  });
});
