// +feature: ui-autocomplete-input
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { AutocompleteInput } from "./AutocompleteInput";

// Text input with a filtered suggestion listbox. Suggestions open on typing or focus.
const meta: Meta<typeof AutocompleteInput> = {
  component: AutocompleteInput,
  title: "UI/AutocompleteInput",
  decorators: [(Story) => <div style={{ width: 360, minHeight: 200 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof AutocompleteInput>;

const BRANCHES = ["main", "develop", "feature/login", "feature/logout", "fix/typo", "release/1.0"];

function Controlled(props: Partial<React.ComponentProps<typeof AutocompleteInput>>) {
  const [v, setV] = useState(props.value ?? "");
  return (
    <AutocompleteInput
      id="story-autocomplete"
      placeholder="Branch name"
      suggestions={BRANCHES}
      {...props}
      value={v}
      onChange={setV}
    />
  );
}

export const Empty: Story = { render: () => <Controlled /> };
export const Filled: Story = { render: () => <Controlled value="feature/login" /> };
export const NoMatches: Story = { render: () => <Controlled value="zzz" /> };
export const NoSuggestions: Story = { render: () => <Controlled suggestions={[]} /> };
export const Loading: Story = { render: () => <Controlled isLoading /> };
export const WithError: Story = { render: () => <Controlled value="bad" error /> };
export const Disabled: Story = { render: () => <Controlled value="main" disabled /> };
export const CustomLabels: Story = {
  render: () => <Controlled getLabel={(v) => v.toUpperCase()} />,
};
export const LongSuggestions: Story = {
  render: () => (
    <Controlled
      suggestions={[
        "feature/an-extremely-long-branch-name-that-overflows-the-input-width-in-narrow-layouts",
        ...BRANCHES,
      ]}
    />
  ),
};
