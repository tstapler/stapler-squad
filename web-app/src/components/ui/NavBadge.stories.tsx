// +feature: ui-nav-badge
import type { Meta, StoryObj } from "@storybook/react";
import { NavBadge } from "./NavBadge";

const meta: Meta<typeof NavBadge> = {
  component: NavBadge,
  title: "UI/NavBadge",
};
export default meta;
type Story = StoryObj<typeof NavBadge>;

export const Span: Story = { args: { element: "span", count: 3, "aria-label": "3 unread" } };
export const Button: Story = {
  args: { element: "button", count: 7, "aria-label": "7 pending approvals" },
};
export const Inline: Story = {
  render: () => (
    <span>
      Notifications <NavBadge element="span" count={5} inline />
    </span>
  ),
};
export const CappedAt99Plus: Story = { args: { element: "span", count: 250 } };
export const ZeroShownWhenEmpty: Story = { args: { element: "span", count: 0, showWhenEmpty: true } };
// count 0 without showWhenEmpty renders null; the label keeps the canvas non-empty.
export const ZeroHidden: Story = {
  render: () => (
    <span>
      Zero count, hidden by default: <NavBadge element="span" count={0} />
    </span>
  ),
};
