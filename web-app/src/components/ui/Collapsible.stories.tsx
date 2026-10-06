// +feature: ui-collapsible
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { CollapsibleGroup, CollapsibleSection } from "./Collapsible";

// Progressive-disclosure sections: standalone CollapsibleSection, or several in a CollapsibleGroup
// that shares one Radix accordion root (cross-header keyboard nav).
const meta: Meta<typeof CollapsibleSection> = {
  component: CollapsibleSection,
  title: "UI/Collapsible",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof CollapsibleSection>;

export const Collapsed: Story = {
  render: () => <CollapsibleSection sectionKey="a" title="Advanced options">Hidden body content</CollapsibleSection>,
};
export const Expanded: Story = {
  render: () => (
    <CollapsibleSection sectionKey="a" title="Advanced options" defaultExpanded>
      Visible body content
    </CollapsibleSection>
  ),
};
export const LongTitleAndContent: Story = {
  render: () => (
    <CollapsibleSection
      sectionKey="long"
      title="An extremely long section title that should wrap rather than overflow its container"
      defaultExpanded
    >
      {"Lots of body content. ".repeat(40)}
    </CollapsibleSection>
  ),
};
export const EmptyBody: Story = {
  render: () => <CollapsibleSection sectionKey="empty" title="Empty section" defaultExpanded>{null}</CollapsibleSection>,
};
export const Group: Story = {
  render: () => (
    <CollapsibleGroup defaultValue={["one"]}>
      <CollapsibleSection sectionKey="one" title="Section one">First body</CollapsibleSection>
      <CollapsibleSection sectionKey="two" title="Section two">Second body</CollapsibleSection>
      <CollapsibleSection sectionKey="three" title="Section three">Third body</CollapsibleSection>
    </CollapsibleGroup>
  ),
};

function ControlledGroup() {
  const [open, setOpen] = useState<string[]>(["two"]);
  return (
    <div>
      <CollapsibleGroup value={open} onValueChange={setOpen}>
        <CollapsibleSection sectionKey="one" title="Section one">First body</CollapsibleSection>
        <CollapsibleSection sectionKey="two" title="Section two">Second body</CollapsibleSection>
      </CollapsibleGroup>
      <p>Open: {open.join(", ") || "none"}</p>
    </div>
  );
}
export const ControlledGroupStory: Story = { render: () => <ControlledGroup /> };
