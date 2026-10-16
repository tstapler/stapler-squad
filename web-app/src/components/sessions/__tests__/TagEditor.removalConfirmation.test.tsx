/**
 * Story 6.2.2 (ux.md Surface 3): removing a rule-provenance tag requires a
 * second confirming click and names the rule; a plain user tag is removed
 * immediately with no confirmation.
 */

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TagEditor } from "../TagEditor";

describe("TagEditor — removal confirmation (Story 6.2.2)", () => {
  it("TagEditor_should_RequireTwoActions_When_RemovingRuleProvenanceTag", async () => {
    const onSave = jest.fn();
    render(
      <TagEditor
        tags={["Bugfix"]}
        tagProvenance={{ Bugfix: "seed-bugfix" }}
        tagRuleNames={{ "seed-bugfix": "Bugfix branch" }}
        onSave={onSave}
        onCancel={jest.fn()}
        sessionTitle="My Session"
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Remove tag Bugfix" }));

    expect(screen.getByText('Applied by rule "Bugfix branch" — may reappear.')).not.toBeNull();
    expect(screen.getByText("Bugfix")).not.toBeNull();

    // Not removed yet after just one click.
    fireEvent.click(screen.getByRole("button", { name: "Save Tags" }));
    expect(onSave).toHaveBeenCalledWith(["Bugfix"]);

    fireEvent.click(screen.getByRole("button", { name: "Remove anyway" }));
    fireEvent.click(screen.getByRole("button", { name: "Save Tags" }));
    expect(onSave).toHaveBeenLastCalledWith([]);
  });

  it("TagEditor_should_MoveFocusToKeepButton_When_ConfirmationExpands", async () => {
    render(
      <TagEditor
        tags={["Bugfix"]}
        tagProvenance={{ Bugfix: "seed-bugfix" }}
        tagRuleNames={{ "seed-bugfix": "Bugfix branch" }}
        onSave={jest.fn()}
        onCancel={jest.fn()}
        sessionTitle="My Session"
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Remove tag Bugfix" }));
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("button", { name: "Keep" })));
  });

  it("TagEditor_should_CancelWithNoChange_When_KeepClicked", () => {
    render(
      <TagEditor
        tags={["Bugfix"]}
        tagProvenance={{ Bugfix: "seed-bugfix" }}
        tagRuleNames={{ "seed-bugfix": "Bugfix branch" }}
        onSave={jest.fn()}
        onCancel={jest.fn()}
        sessionTitle="My Session"
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Remove tag Bugfix" }));
    fireEvent.click(screen.getByRole("button", { name: "Keep" }));

    expect(screen.getByRole("button", { name: "Remove tag Bugfix" })).not.toBeNull();
    expect(screen.queryByText(/may reappear/)).toBeNull();
  });

  it("TagEditor_should_RemoveImmediatelyWithNoConfirmation_When_RemovingPlainUserTag", () => {
    const onSave = jest.fn();
    render(
      <TagEditor
        tags={["MyTag"]}
        tagProvenance={{}}
        onSave={onSave}
        onCancel={jest.fn()}
        sessionTitle="My Session"
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Remove tag MyTag" }));
    expect(screen.queryByText(/may reappear/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save Tags" }));
    expect(onSave).toHaveBeenCalledWith([]);
  });

  it("TagEditor_should_UseGenericFallbackText_When_ProvenanceRuleIdNoLongerResolves", () => {
    render(
      <TagEditor
        tags={["Bugfix"]}
        tagProvenance={{ Bugfix: "deleted-rule-id" }}
        tagRuleNames={{}}
        onSave={jest.fn()}
        onCancel={jest.fn()}
        sessionTitle="My Session"
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Remove tag Bugfix" }));
    expect(
      screen.getByText("This tag was applied automatically and may reappear — remove anyway?")
    ).not.toBeNull();
  });
});
