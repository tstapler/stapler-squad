import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { CollapsibleGroup, CollapsibleSection } from "./Collapsible";

describe("CollapsibleSection", () => {
  it("CollapsibleSection_should_RenderAriaExpandedTrueAndMountBody_When_HeaderButtonClicked", () => {
    render(
      <CollapsibleSection sectionKey="plan-artifacts" title="Plan artifacts" defaultExpanded={false}>
        <p>Body content</p>
      </CollapsibleSection>
    );

    const header = screen.getByTestId("collapsible-header-plan-artifacts");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Body content")).not.toBeInTheDocument();

    fireEvent.click(header);

    expect(header).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Body content")).toBeInTheDocument();
  });

  it("renders a real <button> header, never a <div onClick>", () => {
    render(
      <CollapsibleSection sectionKey="plan-artifacts" title="Plan artifacts">
        <p>Body content</p>
      </CollapsibleSection>
    );
    expect(screen.getByTestId("collapsible-header-plan-artifacts").tagName).toBe("BUTTON");
  });

  it("starts expanded and mounts its body when defaultExpanded is true", () => {
    render(
      <CollapsibleSection sectionKey="version-control" title="Version control" defaultExpanded={true}>
        <p>VCS body</p>
      </CollapsibleSection>
    );

    expect(screen.getByTestId("collapsible-header-version-control")).toHaveAttribute(
      "aria-expanded",
      "true"
    );
    expect(screen.getByText("VCS body")).toBeInTheDocument();
  });

  it("removes collapsed content from the DOM again after re-collapsing, not just visually", () => {
    render(
      <CollapsibleSection sectionKey="notes" title="Notes" defaultExpanded={true}>
        <p>Note body</p>
      </CollapsibleSection>
    );

    const header = screen.getByTestId("collapsible-header-notes");
    expect(screen.getByText("Note body")).toBeInTheDocument();

    fireEvent.click(header);

    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Note body")).not.toBeInTheDocument();
  });

  it("calls onExpandedChange with the new open state when toggled standalone", () => {
    const onExpandedChange = jest.fn();
    render(
      <CollapsibleSection
        sectionKey="actions"
        title="Actions"
        defaultExpanded={false}
        onExpandedChange={onExpandedChange}
      >
        <p>Actions body</p>
      </CollapsibleSection>
    );

    fireEvent.click(screen.getByTestId("collapsible-header-actions"));
    expect(onExpandedChange).toHaveBeenCalledWith(true);
  });
});

describe("CollapsibleGroup", () => {
  it("CollapsibleGroup_should_MoveFocusToNextHeader_When_ArrowDownPressedOnSiblingSection", () => {
    render(
      <CollapsibleGroup>
        <CollapsibleSection sectionKey="plan-artifacts" title="Plan artifacts">
          <p>Plan body</p>
        </CollapsibleSection>
        <CollapsibleSection sectionKey="version-control" title="Version control">
          <p>VCS body</p>
        </CollapsibleSection>
      </CollapsibleGroup>
    );

    const firstHeader = screen.getByTestId("collapsible-header-plan-artifacts");
    const secondHeader = screen.getByTestId("collapsible-header-version-control");

    firstHeader.focus();
    expect(firstHeader).toHaveFocus();

    fireEvent.keyDown(firstHeader, { key: "ArrowDown", code: "ArrowDown" });

    expect(secondHeader).toHaveFocus();
  });

  it("allows each sibling section to open independently (type=multiple, no forced exclusivity)", () => {
    render(
      <CollapsibleGroup>
        <CollapsibleSection sectionKey="plan-artifacts" title="Plan artifacts">
          <p>Plan body</p>
        </CollapsibleSection>
        <CollapsibleSection sectionKey="version-control" title="Version control">
          <p>VCS body</p>
        </CollapsibleSection>
      </CollapsibleGroup>
    );

    fireEvent.click(screen.getByTestId("collapsible-header-plan-artifacts"));
    fireEvent.click(screen.getByTestId("collapsible-header-version-control"));

    expect(screen.getByText("Plan body")).toBeInTheDocument();
    expect(screen.getByText("VCS body")).toBeInTheDocument();
  });

  it("CollapsibleGroup_should_WarnAndIgnoreDefaultExpanded_When_NestedSectionSetsDefaultExpandedTrue", () => {
    const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});

    render(
      <CollapsibleGroup defaultValue={["version-control"]}>
        <CollapsibleSection sectionKey="plan-artifacts" title="Plan artifacts" defaultExpanded={true}>
          <p>Plan body</p>
        </CollapsibleSection>
        <CollapsibleSection sectionKey="version-control" title="Version control">
          <p>VCS body</p>
        </CollapsibleSection>
      </CollapsibleGroup>
    );

    // Warned once for the section that set defaultExpanded inside a group.
    expect(warnSpy).toHaveBeenCalledTimes(1);
    expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining("plan-artifacts"));

    // The ignored prop does NOT control initial open state — the group's
    // defaultValue does. "plan-artifacts" set defaultExpanded={true} but is
    // NOT in the group's defaultValue, so it stays collapsed.
    expect(
      screen.getByTestId("collapsible-header-plan-artifacts")
    ).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Plan body")).not.toBeInTheDocument();

    // "version-control" never set defaultExpanded but IS in the group's
    // defaultValue, so it starts open.
    expect(
      screen.getByTestId("collapsible-header-version-control")
    ).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("VCS body")).toBeInTheDocument();

    warnSpy.mockRestore();
  });

  it("CollapsibleGroup_should_NotWarn_When_StandaloneSectionSetsDefaultExpanded", () => {
    const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});

    render(
      <CollapsibleSection sectionKey="standalone" title="Standalone" defaultExpanded={true}>
        <p>Standalone body</p>
      </CollapsibleSection>
    );

    expect(warnSpy).not.toHaveBeenCalled();

    warnSpy.mockRestore();
  });
});
