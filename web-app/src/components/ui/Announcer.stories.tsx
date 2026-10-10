import type { Meta, StoryObj } from "@storybook/react";
import { AnnouncerProvider } from "./Announcer";
import { useAnnounce } from "@/lib/hooks/useAnnounce";

function Demo() {
  const { announce, announceArrival } = useAnnounce();
  return (
    <div style={{ display: "flex", gap: 8 }}>
      <button onClick={() => announce("Moved 4 to tray")}>Announce receipt</button>
      <button onClick={() => announceArrival({ title: "Approve command?", pinned: true })}>
        Announce pinned arrival
      </button>
    </div>
  );
}

// The regions are visually hidden; this story exists to exercise the provider and its API.
const meta: Meta<typeof AnnouncerProvider> = {
  component: AnnouncerProvider,
  title: "UI/Announcer",
  render: () => (
    <AnnouncerProvider>
      <Demo />
    </AnnouncerProvider>
  ),
};
export default meta;
type Story = StoryObj<typeof AnnouncerProvider>;

export const Default: Story = {};
