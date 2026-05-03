// +feature: drawer-nav
import React, { createContext, useContext, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { DrawerNav } from "./DrawerNav";

// ---------------------------------------------------------------------------
// Mock NavigationContext so DrawerNav renders without a full Next.js router
// ---------------------------------------------------------------------------
interface NavigationContextValue {
  isDrawerOpen: boolean;
  toggleDrawer: () => void;
  openDrawer: () => void;
  closeDrawer: () => void;
}

const NavigationContext = createContext<NavigationContextValue | null>(null);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
(useContext as any)._mock = undefined; // keep TS happy

function MockNavigationProvider({
  children,
  defaultOpen = true,
}: {
  children: React.ReactNode;
  defaultOpen?: boolean;
}) {
  const [isDrawerOpen, setIsDrawerOpen] = useState(defaultOpen);
  return (
    <NavigationContext.Provider
      value={{
        isDrawerOpen,
        toggleDrawer: () => setIsDrawerOpen((v) => !v),
        openDrawer: () => setIsDrawerOpen(true),
        closeDrawer: () => setIsDrawerOpen(false),
      }}
    >
      {children}
    </NavigationContext.Provider>
  );
}

const meta: Meta<typeof DrawerNav> = {
  component: DrawerNav,
  title: "Layout/DrawerNav",
  decorators: [
    (Story) => (
      <MockNavigationProvider>
        <div style={{ height: "100vh", display: "flex" }}>
          <Story />
        </div>
      </MockNavigationProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DrawerNav>;

export const Default: Story = {
  args: {},
};

export const WithBadges: Story = {
  args: {
    reviewQueueCount: 3,
    sessionCount: 12,
  },
};

export const CollapsedByDefault: Story = {
  decorators: [
    (Story) => (
      <MockNavigationProvider defaultOpen={false}>
        <div style={{ height: "100vh", display: "flex" }}>
          <Story />
        </div>
      </MockNavigationProvider>
    ),
  ],
  args: {},
};
