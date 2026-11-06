// +feature: session:guidance-structured
import type { Meta, StoryObj } from "@storybook/react";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";
import { SavedForLaterSection } from "./SavedForLaterSection";

const row = (over: Partial<PlainGuidanceRequest>): PlainGuidanceRequest => ({
  id: "g1",
  scope: "session",
  sessionUuid: "7c1e0000-0000-4000-8000-0000000000aa",
  questionText: "Which cache should we use?",
  questionType: "multiple-choice",
  options: ["Redis", "Memcached"],
  answer: "Redis",
  status: "answered",
  createdAt: undefined,
  answeredAt: undefined,
  deliveryRoute: "resume",
  deliveryState: "saved_for_later",
  ...over,
});

// Read-only disclosure of answers waiting for a stopped or archived session to resume.
const meta: Meta<typeof SavedForLaterSection> = {
  component: SavedForLaterSection,
  title: "UI/SavedForLaterSection",
  decorators: [(Story) => <div style={{ width: 400 }}><Story /></div>],
  args: { rows: [row({}), row({ id: "g2", deliveryRoute: "read_only", questionText: "Ship it?", answer: "Yes" })], sessionNames: { "7c1e0000-0000-4000-8000-0000000000aa": "review:pr-1042" } },
};
export default meta;
type Story = StoryObj<typeof SavedForLaterSection>;

export const Collapsed: Story = {};
export const Empty: Story = { args: { rows: [] } };
