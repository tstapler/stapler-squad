// +feature: notification-tray
import type { Meta, StoryObj } from "@storybook/react";
import { TrayOverflowMenu } from "./TrayOverflowMenu";

// Undoable actions above a divider, the irreversible one below it.
const meta: Meta<typeof TrayOverflowMenu> = {
  component: TrayOverflowMenu,
  title: "UI/TrayOverflowMenu",
  decorators: [(Story) => <div style={{ width: 320, height: 240 }}><Story /></div>],
  args: {
    groups: [
      [{ key: "clear-informational", label: "Clear informational (20)", caption: "Undo available", onSelect: () => {} }],
      [{ key: "clear-history", label: "Clear history...", caption: "cannot be undone", onSelect: () => {} }],
      [{ key: "collapse-all", label: "Collapse all groups", onSelect: () => {} }],
    ],
  },
};
export default meta;
type Story = StoryObj<typeof TrayOverflowMenu>;

export const Default: Story = {};
