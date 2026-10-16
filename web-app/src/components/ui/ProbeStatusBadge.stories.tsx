// +feature: ui-probe-status-badge
import type { Meta, StoryObj } from "@storybook/react";
import { ProbeStatusBadge } from "./ProbeStatusBadge";

// Server-side program probe result shown under a program command input.
const meta: Meta<typeof ProbeStatusBadge> = {
  component: ProbeStatusBadge,
  title: "UI/ProbeStatusBadge",
  decorators: [(Story) => <div style={{ width: 480 }}><Story /></div>],
  args: { checkedToken: "claude", onRetry: () => {}, onConfirm: () => {} },
};
export default meta;
type Story = StoryObj<typeof ProbeStatusBadge>;

const path = "/Users/you/.asdf/installs/nodejs/22.1.0/bin/claude";

export const Checking: Story = { args: { state: { kind: "checking" } } };
export const Found: Story = { args: { state: { kind: "found", path, flagCount: 14, flags: [] } } };
export const FoundNoFlagCount: Story = { args: { state: { kind: "found", path, flagCount: 0, flags: [] } } };
export const NoFlags: Story = { args: { state: { kind: "noFlags", path } } };
export const NeedsConfirm: Story = { args: { state: { kind: "needsConfirm", path } } };
export const Timeout: Story = { args: { state: { kind: "timeout", path } } };
export const Wrapper: Story = { args: { checkedToken: "npx", state: { kind: "wrapper" } } };
export const NotFound: Story = { args: { checkedToken: "claudee", state: { kind: "notFound" } } };
export const BusyOrError: Story = { args: { state: { kind: "busyOrError" } } };
export const TransportError: Story = { args: { state: { kind: "transportError" } } };
export const LongPath: Story = {
  args: { state: { kind: "found", path: "/Users/you/" + "very-long-directory-name/".repeat(8) + "claude", flagCount: 3, flags: [] } },
};
