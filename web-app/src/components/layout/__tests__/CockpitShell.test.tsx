import React from "react";
import { render, screen } from "@testing-library/react";
import { CockpitShell } from "../CockpitShell";

jest.mock("../DrawerNav", () => ({
  DrawerNav: () => <nav data-testid="drawer-nav" aria-label="Main navigation" />,
}));

jest.mock("../BottomNav", () => ({
  BottomNav: () => <nav data-testid="bottom-nav" aria-label="Bottom navigation" />,
}));

jest.mock("@/components/ui/KeyboardShortcutOverlay", () => ({
  KeyboardShortcutOverlay: () => null,
}));

jest.mock("@/lib/shortcuts/useShortcut", () => ({
  useShortcut: jest.fn(),
}));

jest.mock("@/lib/contexts/NavigationContext", () => ({
  useNavigation: () => ({ isDrawerOpen: true, toggleDrawer: jest.fn() }),
}));

describe("CockpitShell", () => {
  it("renders BottomNav for mobile navigation", () => {
    render(<CockpitShell>children</CockpitShell>);
    expect(screen.getByTestId("bottom-nav")).toBeInTheDocument();
  });

  it("renders DrawerNav for desktop navigation", () => {
    render(<CockpitShell>children</CockpitShell>);
    expect(screen.getByTestId("drawer-nav")).toBeInTheDocument();
  });

  it("renders children inside the main content area", () => {
    render(<CockpitShell><span data-testid="child">content</span></CockpitShell>);
    expect(screen.getByTestId("child")).toBeInTheDocument();
  });

  it("renders both DrawerNav and BottomNav so mobile and desktop are both served", () => {
    render(<CockpitShell>test</CockpitShell>);
    // DrawerNav is CSS-hidden on mobile; BottomNav is CSS-hidden on desktop.
    // Both must be in the DOM so CSS can switch between them.
    expect(screen.getByTestId("drawer-nav")).toBeInTheDocument();
    expect(screen.getByTestId("bottom-nav")).toBeInTheDocument();
  });
});
