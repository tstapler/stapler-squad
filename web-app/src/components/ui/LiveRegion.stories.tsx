// +feature: ui-live-region
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { LiveRegion } from "./LiveRegion";

// Visually hidden (sr-only) ARIA live region; stories show the announced text
// beside it so the canvas is inspectable.
const meta: Meta<typeof LiveRegion> = {
  component: LiveRegion,
  title: "UI/LiveRegion",
};
export default meta;
type Story = StoryObj<typeof LiveRegion>;

export const Polite: Story = {
  render: () => (
    <div>
      <p>Announced politely (role=status): &quot;Session created&quot;</p>
      <LiveRegion message="Session created" />
    </div>
  ),
};
export const AssertiveAlert: Story = {
  render: () => (
    <div>
      <p>Announced assertively (role=alert): &quot;Connection lost&quot;</p>
      <LiveRegion message="Connection lost" politeness="assertive" role="alert" />
    </div>
  ),
};
export const EmptyMessage: Story = {
  render: () => (
    <div>
      <p>Empty message: nothing is announced.</p>
      <LiveRegion message="" />
    </div>
  ),
};

function Announcer() {
  const [n, setN] = useState(0);
  return (
    <div>
      <button type="button" onClick={() => setN((v) => v + 1)}>Announce update</button>
      <p>Announcements sent: {n}</p>
      <LiveRegion message={n === 0 ? "" : `Update ${n} applied`} />
    </div>
  );
}
export const Interactive: Story = { render: () => <Announcer /> };
