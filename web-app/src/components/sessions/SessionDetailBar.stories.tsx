// +feature: session-detail-bar
import type { Meta, StoryObj } from "@storybook/react";
import { SessionDetailBar } from "./SessionDetailBar";

const meta: Meta<typeof SessionDetailBar> = {
  component: SessionDetailBar,
  title: "Sessions/SessionDetailBar",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof SessionDetailBar>;

export const Default: Story = {
  args: {
    branch: "feat/ux-revamp",
    path: "~/projects/stapler-squad",
  },
};

export const WithBack: Story = {
  args: {
    branch: "main",
    path: "~/projects/stapler-squad",
    onBack: () => alert("back"),
  },
};

export const RunningSession: Story = {
  args: {
    branch: "feat/streaming",
    path: "~/work/stapler-squad",
    statusBadge: (
      <span
        style={{
          fontSize: 11,
          padding: "2px 8px",
          borderRadius: 4,
          background: "rgba(0,255,65,0.15)",
          border: "1px solid currentColor",
          opacity: 0.85,
        }}
      >
        running
      </span>
    ),
  },
};

export const NoPath: Story = {
  args: {
    branch: "fix/modal-focus",
  },
};
