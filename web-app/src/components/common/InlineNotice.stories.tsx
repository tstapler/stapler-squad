// +feature: backlog:inline-notice
import type { Meta, StoryObj } from "@storybook/react";
import { InlineNotice } from "./InlineNotice";

const meta: Meta<typeof InlineNotice> = { component: InlineNotice, title: "Common/InlineNotice" };
export default meta;
type Story = StoryObj<typeof InlineNotice>;

export const Message: Story = { args: { message: "This item changed elsewhere." } };
export const WithActions: Story = {
  args: {
    message: "This item changed elsewhere.",
    actions: [{ label: "Reload", onClick: () => {} }, { label: "Save Anyway", onClick: () => {}, variant: "primary" }],
  },
};
export const Dismissible: Story = { args: { message: "Archived elsewhere.", onDismiss: () => {} } };
