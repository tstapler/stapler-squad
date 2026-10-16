// +feature: ui-debug-menu
import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { DebugMenu } from "./DebugMenu";

// Dev-only debug modal. Server log-level and snapshot calls need the Go server;
// offline the toggles still render and the fetches fail silently.
const meta: Meta<typeof DebugMenu> = {
  component: DebugMenu,
  title: "UI/DebugMenu",
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof DebugMenu>;

function Harness({ startOpen }: { startOpen: boolean }) {
  const [isOpen, setIsOpen] = useState(startOpen);
  const triggerRef = useRef<HTMLButtonElement>(null);
  return (
    <div style={{ padding: 24 }}>
      <button ref={triggerRef} type="button" onClick={() => setIsOpen(true)}>
        Open debug menu
      </button>
      <DebugMenu isOpen={isOpen} onClose={() => setIsOpen(false)} triggerRef={triggerRef} />
    </div>
  );
}

export const Open: Story = { render: () => <Harness startOpen /> };
export const Closed: Story = { render: () => <Harness startOpen={false} /> };
