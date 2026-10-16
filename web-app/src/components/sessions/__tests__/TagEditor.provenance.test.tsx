/**
 * Story 6.2.1 (ux.md Surface 2): Unclassified has no remove control in
 * TagEditor and shows an explanatory caption instead; a plain tag is
 * unaffected.
 */

import { render, screen } from "@testing-library/react";
import { TagEditor } from "../TagEditor";

describe("TagEditor — Unclassified styling (Story 6.2.1)", () => {
  it("TagEditor_should_HideRemoveButtonAndShowCaption_When_UnclassifiedTagPresent", () => {
    render(
      <TagEditor
        tags={["Unclassified", "Bugfix"]}
        tagProvenance={{ Bugfix: "seed-bugfix" }}
        onSave={jest.fn()}
        onCancel={jest.fn()}
        sessionTitle="My Session"
      />
    );

    expect(screen.queryByRole("button", { name: "Remove tag Unclassified" })).toBeNull();
    expect(screen.getByText("Removed automatically once classification succeeds")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Remove tag Bugfix" })).not.toBeNull();
  });
});
