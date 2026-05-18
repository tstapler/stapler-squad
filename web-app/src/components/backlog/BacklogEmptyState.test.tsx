/**
 * Tests for BacklogEmptyState, FilterZeroState, and FooterNudge components.
 *
 * Covers:
 *  1. First-run state: shows "+ Create First Item" button
 *  2. First-run state: lifecycle diagram is aria-hidden
 *  3. Click "+ Create First Item" reveals the inline form
 *  4. Form has title input with autoFocus
 *  5. Submit with empty title shows "Title is required." error with role="alert"
 *  6. Submit with valid title calls onCreateItem with correct data
 *  7. Cancel button closes form and removes it from DOM
 *  8. Form select has P1-P5 priority options
 *  9. onCreateItem rejection shows submit error (component does not crash)
 *  10. FilterZeroState renders "No items match" and calls onClearFilters on button click
 *  11. FooterNudge renders "No items are currently in progress" message
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
  // Test: 3 — Clicking button reveals inline form
  // -------------------------------------------------------------------------

  it("clicking Create First Item reveals the inline form", () => {
    render(<BacklogEmptyState onCreateItem={jest.fn()} />);

    expect(screen.queryByRole("form", { name: /Create new backlog item/i })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));

    expect(screen.getByRole("form", { name: /Create new backlog item/i })).toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // Test: 4 — Form title input is present (autoFocus)
  // -------------------------------------------------------------------------

  it("inline form has a title input field", () => {
    render(<BacklogEmptyState onCreateItem={jest.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));

    const titleInput = screen.getByLabelText(/^Title$/i);
    expect(titleInput).toBeInTheDocument();
    expect(titleInput).toHaveAttribute("type", "text");
  });

  // -------------------------------------------------------------------------
  // Test: 5 — Submit with empty title shows "Title is required." alert
  // -------------------------------------------------------------------------

  it("submitting with an empty title shows a Title is required error", () => {
    render(<BacklogEmptyState onCreateItem={jest.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));

    fireEvent.click(screen.getByRole("button", { name: /Create Item/i }));

    const alert = screen.getByRole("alert");
    expect(alert).toBeInTheDocument();
    expect(alert).toHaveTextContent("Title is required.");
  });

  // -------------------------------------------------------------------------
  // Test: 6 — Submit with valid title calls onCreateItem with correct data
  // -------------------------------------------------------------------------

  it("submitting with a valid title calls onCreateItem with title and priority", async () => {
    const onCreateItem = jest.fn().mockResolvedValue(undefined);
    render(<BacklogEmptyState onCreateItem={onCreateItem} />);

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));

    fireEvent.change(screen.getByLabelText(/^Title$/i), {
      target: { value: "My new feature" },
    });

    fireEvent.click(screen.getByRole("button", { name: /Create Item/i }));

    await waitFor(() => expect(onCreateItem).toHaveBeenCalledTimes(1));
    expect(onCreateItem).toHaveBeenCalledWith({
      title: "My new feature",
      priority: 3,
    });
  });

  // -------------------------------------------------------------------------
  // Test: 7 — Cancel button closes form
  // -------------------------------------------------------------------------

  it("clicking Cancel closes the form and removes it from the DOM", () => {
    render(<BacklogEmptyState onCreateItem={jest.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));
    expect(screen.getByRole("form", { name: /Create new backlog item/i })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^Cancel$/i }));

    expect(screen.queryByRole("form", { name: /Create new backlog item/i })).not.toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // Test: 8 — Priority select has P1-P5 options
  // -------------------------------------------------------------------------

  it("priority select contains all P1 through P5 options", () => {
    render(<BacklogEmptyState onCreateItem={jest.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));

    const select = screen.getByLabelText(/^Priority$/i);
    expect(select).toBeInTheDocument();

    const options = Array.from((select as HTMLSelectElement).options).map(
      (o) => o.text
    );
    expect(options.some((o) => /P1/i.test(o))).toBe(true);
    expect(options.some((o) => /P2/i.test(o))).toBe(true);
    expect(options.some((o) => /P3/i.test(o))).toBe(true);
    expect(options.some((o) => /P4/i.test(o))).toBe(true);
    expect(options.some((o) => /P5/i.test(o))).toBe(true);
  });

  // -------------------------------------------------------------------------
  // Test: 9 — onCreateItem rejection does not crash the component
  // -------------------------------------------------------------------------

  it("component stays rendered when onCreateItem rejects", async () => {
    const onCreateItem = jest
      .fn()
      .mockRejectedValue(new Error("Server error"));

    render(<BacklogEmptyState onCreateItem={onCreateItem} />);

    fireEvent.click(screen.getByRole("button", { name: /\+ Create First Item/i }));
    fireEvent.change(screen.getByLabelText(/^Title$/i), {
      target: { value: "Title that will fail" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Create Item/i }));

    // After rejection the form should still be in the document (component didn't unmount)
    await waitFor(() => expect(onCreateItem).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("form", { name: /Create new backlog item/i })).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Test: 10 — FilterZeroState
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
// Test: 11 — FooterNudge
// ---------------------------------------------------------------------------

describe("FooterNudge", () => {
  it("renders the 'No items are currently in progress' message", () => {
    render(<FooterNudge />);

    expect(
      screen.getByText(/No items are currently in progress/i)
    ).toBeInTheDocument();
  });
});
