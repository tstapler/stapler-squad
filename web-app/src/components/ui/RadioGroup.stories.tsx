// +feature: ui-radio-group
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { RadioGroup } from "./RadioGroup";

const options = [
  { value: "a", label: "Option A", description: "First choice" },
  { value: "b", label: "Option B" },
] as const;

const meta: Meta = { title: "UI/RadioGroup" };
export default meta;
type Story = StoryObj;

export const Default: Story = {
  render: () => {
    const [v, setV] = useState<"a" | "b">("a");
    return <RadioGroup options={options} value={v} onChange={setV} groupLabel="Pick one" />;
  },
};
