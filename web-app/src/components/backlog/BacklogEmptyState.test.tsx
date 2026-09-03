/**
 * Tests for BacklogEmptyState, FilterZeroState, and FooterNudge components.
 *
 * BacklogEmptyState is button-only — the inline create-form this file originally tested
 * was removed in favor of a page-level form (see web-app/src/app/backlog/page.tsx); the
 * button here only triggers onCreateItem.
 *
 * Covers:
 *  1. First-run state: shows "+ Create First Item" button
 *  2. First-run state: lifecycle diagram is aria-hidden
 *  3. Clicking the button calls onCreateItem once
 *  4. onCreateItem returning a rejected promise doesn't crash the component
 *  5. FilterZeroState renders "No items match" and calls onClearFilters on button click
 *  6. FooterNudge renders "No items are currently in progress" message
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BacklogEmptyState, FilterZeroState, FooterNudge } from "./BacklogEmptyState";

// ---------------------------------------------------------------------------
// Test: 1 — First-run state shows "+ Create First Item"
// ---------------------------------------------------------------------------

describe("BacklogEmptyState — first-run state", () => {
  it("renders the Create First Item button initially", () => {
    render(<BacklogEmptyState onCreateItem={jest.fn()} />);

    expect(
      screen.getByRole("button", { name: /\+ Create First Item/i })
    ).toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // Test: 2 — Lifecycle diagram is aria-hidden
  // -------------------------------------------------------------------------

  it("lifecycle diagram element is aria-hidden", () => {
    const { container } = render(<BacklogEmptyState onCreateItem={jest.fn()} />);

    const diagram = container.querySelector('[aria-hidden="true"]');
    expect(diagram).not.toBeNull();
  });

  // -------------------------------------------------------------------------
  // Test: 3 — Clicking the button calls onCreateItem once
  // -------------------------------------------------------------------------

  it("clicking Create First Item calls onCreateItem once", async () => {
    const onCreateItem = jest.fn();
    render(<BacklogEmptyState onCreateItem={onCreateItem} />);

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));

    await waitFor(() => expect(onCreateItem).toHaveBeenCalledTimes(1));
  });

  // -------------------------------------------------------------------------
  // Test: 4 — onCreateItem rejection does not crash the component
  // -------------------------------------------------------------------------

  it("component stays rendered when onCreateItem returns a rejected promise", async () => {
    const onCreateItem = jest
      .fn()
      .mockRejectedValue(new Error("Server error"));

    render(<BacklogEmptyState onCreateItem={onCreateItem} />);

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));

    await waitFor(() => expect(onCreateItem).toHaveBeenCalledTimes(1));
    expect(
      screen.getByRole("button", { name: /\+ Create First Item/i })
    ).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 5 — FilterZeroState
// ---------------------------------------------------------------------------

describe("FilterZeroState", () => {
  it("renders 'No items match' text and a Clear filters button", () => {
    render(<FilterZeroState onClearFilters={jest.fn()} />);

    expect(screen.getByText(/No items match your filters/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Clear filters/i })).toBeInTheDocument();
  });

  it("calls onClearFilters when the Clear filters button is clicked", () => {
    const onClearFilters = jest.fn();
    render(<FilterZeroState onClearFilters={onClearFilters} />);

    fireEvent.click(screen.getByRole("button", { name: /Clear filters/i }));

    expect(onClearFilters).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Test: 6 — FooterNudge
// ---------------------------------------------------------------------------

describe("FooterNudge", () => {
  it("renders the 'No items are currently in progress' message", () => {
    render(<FooterNudge />);

    expect(
      screen.getByText(/No items are currently in progress/i)
    ).toBeInTheDocument();
  });
});
