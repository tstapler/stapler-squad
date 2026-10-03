// +feature: ui-system-banner
import type { Meta, StoryObj } from "@storybook/react";
import { AnalyticsContextProvider } from "@/lib/analytics";
import type { AnalyticsProvider } from "@/lib/analytics";
import { SystemBanner, SystemBannerStack, type SystemBannerProps } from "./SystemBanner";

// Page-level banner in info/warning/error severities, with actions, dismiss and paging.
const noopProvider: AnalyticsProvider = { metadata: { name: "storybook" }, track: () => {} };

const meta: Meta<typeof SystemBanner> = {
  component: SystemBanner,
  title: "UI/SystemBanner",
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <AnalyticsContextProvider provider={noopProvider}>
        <Story />
      </AnalyticsContextProvider>
    ),
  ],
  args: { id: "story-banner", severity: "info", message: "A new version is available." },
};
export default meta;
type Story = StoryObj<typeof SystemBanner>;

const reload = { label: "Reload", onClick: () => {} };

export const Info: Story = { args: { severity: "info", icon: "ℹ", actions: [reload], onDismiss: () => {} } };
export const Warning: Story = {
  args: { severity: "warning", icon: "⚠", message: "Notifications are blocked by your browser.", actions: [{ label: "Fix", onClick: () => {} }], onDismiss: () => {} },
};
export const ErrorSeverity: Story = {
  args: {
    severity: "error",
    icon: "✕",
    message: "Lost connection to the server.",
    actions: [{ label: "Retry", onClick: () => {} }, { label: "Restart", onClick: () => {}, variant: "danger" }],
  },
};
export const DisabledAction: Story = { args: { actions: [{ label: "Reconnecting…", onClick: () => {}, disabled: true }] } };
export const MessageOnly: Story = { args: { onDismiss: undefined, actions: undefined } };
export const LongContent: Story = {
  args: {
    severity: "warning",
    message: "This is a very long banner message that is intended to overflow a single line so the expand and collapse behaviour can be reviewed. ".repeat(4),
    actions: [reload],
    onDismiss: () => {},
  },
};

const stackItems: SystemBannerProps[] = [
  { id: "a", severity: "error", message: "Server unreachable.", actions: [reload] },
  { id: "b", severity: "warning", message: "Notifications blocked." },
  { id: "c", severity: "info", message: "Update available." },
];
export const Stack: Story = { render: () => <SystemBannerStack items={stackItems} /> };
