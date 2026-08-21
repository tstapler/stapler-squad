import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { NotesSection } from "./NotesSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Item",
    status: "in_progress",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

describe("NotesSection", () => {
  it("defaults collapsed when defaultExpanded=false", () => {
    render(
      <NotesSection
        item={makeItem()}
        actionLoading={null}
        editingNotes={false}
        notesValue=""
        defaultExpanded={false}
        onNotesValueChange={jest.fn()}
        onStartEditing={jest.fn()}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    expect(screen.getByTestId("collapsible-header-notes")).toHaveAttribute("aria-expanded", "false");
  });

  it("calls onStartEditing when the display text is clicked", () => {
    const onStartEditing = jest.fn();
    render(
      <NotesSection
        item={makeItem({ notes: "hello" })}
        actionLoading={null}
        editingNotes={false}
        notesValue=""
        defaultExpanded={true}
        onNotesValueChange={jest.fn()}
        onStartEditing={onStartEditing}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    fireEvent.click(screen.getByTestId("backlog-notes-display"));
    expect(onStartEditing).toHaveBeenCalled();
  });

  it("renders the editing textarea and calls onSave/onCancel", () => {
    const onSave = jest.fn();
    const onCancel = jest.fn();
    render(
      <NotesSection
        item={makeItem()}
        actionLoading={null}
        editingNotes={true}
        notesValue="draft text"
        defaultExpanded={true}
        onNotesValueChange={jest.fn()}
        onStartEditing={jest.fn()}
        onSave={onSave}
        onCancel={onCancel}
      />
    );

    expect(screen.getByTestId("backlog-notes-textarea")).toHaveValue("draft text");
    fireEvent.click(screen.getByTestId("backlog-notes-save"));
    expect(onSave).toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("backlog-notes-cancel"));
    expect(onCancel).toHaveBeenCalled();
  });
});
