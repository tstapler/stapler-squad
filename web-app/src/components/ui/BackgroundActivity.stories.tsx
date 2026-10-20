// +feature: ui-background-activity
import type { Meta, StoryObj } from "@storybook/react";
import { buildActivityRows, buildRollup, type ActivityRow } from "@/lib/utils/backgroundActivity";
import { BackgroundActivity } from "./BackgroundActivity";
import { makeBackgroundRow, makeElevenStopped } from "./__fixtures__/notificationFixtures";

const exception = (over: Parameters<typeof makeBackgroundRow>[0] = {}): ActivityRow => {
  const ex = makeBackgroundRow({ title: "review - PR #912 fix", ...over });
  return {
    key: ex.key,
    sessionId: ex.sessionId,
    sessionTitle: ex.sessionTitle,
    role: "review",
    state: ex.kind === "failure" ? "exception_failure" : "exception_needs_human",
    timestampMs: ex.timestampMs,
    exception: ex,
  };
};

const withRollup = (rows: ActivityRow[]) => ({ rows, rollup: buildRollup(rows) });

const meta: Meta<typeof BackgroundActivity> = {
  component: BackgroundActivity,
  title: "UI/BackgroundActivity",
  decorators: [(Story) => <div style={{ width: 400 }}><Story /></div>],
  args: {
    ...withRollup([]),
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
  args: withRollup([
    exception(),
    exception({ key: "bg:triage", sessionId: "triage:355", sessionTitle: "triage:355", kind: "needs_human", statusLabel: "NEEDS INPUT", title: "triage - issue #355", message: "Claude is waiting for your input", pendingQuestion: true }),
    ...buildActivityRows([], makeElevenStopped("completed")),
  ]),
};
export const SessionDeleted: Story = { args: withRollup([exception({ sessionAvailable: false })]) };
export const NoFailuresReported: Story = { args: withRollup(buildActivityRows([], makeElevenStopped("unknown"))) };
export const EmptyNoSessions: Story = {};
export const Loading: Story = { args: { loading: true, lastUpdatedAt: null } };
export const ErrorWithStaleData: Story = {
  args: { failed: true, ...withRollup([exception()]), lastUpdatedAt: Date.now() - 120_000 },
};
export const Offline: Story = {
  args: { offline: true, ...withRollup([exception()]), lastUpdatedAt: Date.now() - 300_000 },
};
