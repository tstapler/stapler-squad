// +feature: ui-background-activity
import type { Meta, StoryObj } from "@storybook/react";
import type { BackgroundRow } from "@/lib/utils/backgroundActivity";
import { BackgroundActivity } from "./BackgroundActivity";

const row = (over: Partial<BackgroundRow> = {}): BackgroundRow => ({
  key: "bg:review:ee1b4be0",
  sessionId: "review:ee1b4be0",
  sessionTitle: "review:ee1b4be0",
  kind: "failure",
  statusLabel: "FAILED",
  title: "review - PR #912 fix",
  message: "3 tests failing",
  timestampMs: Date.now() - 4 * 60_000,
  recordIds: ["n1"],
  primaryRecordId: "n1",
  sessionAvailable: true,
  pendingQuestion: false,
  ...over,
});

const meta: Meta<typeof BackgroundActivity> = {
  component: BackgroundActivity,
  title: "UI/BackgroundActivity",
  decorators: [(Story) => <div style={{ width: 400 }}><Story /></div>],
  args: {
    view: { rows: [], completedOkToday: 0, running: 0 },
    hasHiddenSessions: true,
    loading: false,
    failed: false,
    offline: false,
    lastUpdatedAt: Date.now() - 30_000,
    onRefresh: () => {},
    onOpenRow: () => {},
  },
};
export default meta;
type Story = StoryObj<typeof BackgroundActivity>;

export const WithRows: Story = {
  args: {
    view: {
      rows: [
        row(),
        row({ key: "bg:triage", sessionId: "triage:355", sessionTitle: "triage:355", kind: "needs_human", statusLabel: "NEEDS INPUT", title: "triage - issue #355", message: "Claude is waiting for your input", pendingQuestion: true }),
      ],
      completedOkToday: 12,
      running: 3,
    },
  },
};
export const SessionDeleted: Story = {
  args: { view: { rows: [row({ sessionAvailable: false })], completedOkToday: 0, running: 0 } },
};
export const EmptyHealthy: Story = { args: { view: { rows: [], completedOkToday: 12, running: 0 } } };
export const EmptyNoSessions: Story = { args: { hasHiddenSessions: false } };
export const Loading: Story = { args: { loading: true, lastUpdatedAt: null } };
export const ErrorWithStaleData: Story = {
  args: { failed: true, view: { rows: [row()], completedOkToday: 0, running: 0 }, lastUpdatedAt: Date.now() - 120_000 },
};
export const Offline: Story = {
  args: { offline: true, view: { rows: [row()], completedOkToday: 0, running: 0 }, lastUpdatedAt: Date.now() - 300_000 },
};
