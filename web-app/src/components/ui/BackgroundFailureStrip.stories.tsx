// +feature: notification-background-activity
import type { Meta, StoryObj } from "@storybook/react";
import { buildActivityRows } from "@/lib/utils/backgroundActivity";
import { BackgroundFailureStrip } from "./BackgroundFailureStrip";
import { makeHiddenSession } from "./__fixtures__/notificationFixtures";

const failures = buildActivityRows(
  [],
  [makeHiddenSession({ id: "review:bad", backgroundOutcome: "failed" }), makeHiddenSession({ id: "review:worse", backgroundOutcome: "failed" })],
);

// Names each new failure so it is never only a count in an "N new" pill.
const meta: Meta<typeof BackgroundFailureStrip> = {
  component: BackgroundFailureStrip,
  title: "UI/BackgroundFailureStrip",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
  args: { failures, hrefFor: (r) => `/?session=${r.sessionId}`, onShow: () => {}, onOpen: () => {} },
};
export default meta;
type Story = StoryObj<typeof BackgroundFailureStrip>;

export const Default: Story = {};
export const Single: Story = { args: { failures: failures.slice(0, 1) } };
