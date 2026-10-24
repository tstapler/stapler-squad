import * as fs from "fs";
import * as path from "path";
import React from "react";
import { render, screen } from "@testing-library/react";
import { RootShell } from "./RootShell";

jest.mock("@/components/providers/ViewportProvider", () => ({
  ViewportProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
jest.mock("@/components/ui/ErrorBoundary", () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
jest.mock("@/lib/contexts/AuthContext", () => ({
  AuthProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
jest.mock("./layout.css", () => ({ mainContent: "main-content-class" }));
jest.mock("./Providers", () => ({
  Providers: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
jest.mock("@/components/layout/CockpitShell", () => ({
  CockpitShell: ({ children }: { children: React.ReactNode }) => <div data-testid="cockpit-shell">{children}</div>,
}));
jest.mock("@/components/system/TmuxVersionMismatchBanner", () => ({
  TmuxVersionMismatchBanner: () => <div data-testid="tmux-banner" />,
}));
jest.mock("@/components/system/ForkPressureStatusBanner", () => ({
  ForkPressureStatusBanner: () => <div data-testid="fork-banner" />,
}));
jest.mock("@/components/ui/TrayErrorBoundary", () => ({
  TrayErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
jest.mock("@/components/ui/NotificationPanel", () => ({
  NotificationPanel: () => <div data-testid="notification-panel" />,
}));

describe("RootShell", () => {
  it("renders the skip link, main#main-content with children, banners and the notification panel inside the cockpit shell", () => {
    render(<RootShell><span>child</span></RootShell>);

    const skip = screen.getByText("Skip to main content");
    expect(skip).toHaveAttribute("href", "#main-content");

    const main = document.getElementById("main-content");
    expect(main?.tagName).toBe("MAIN");
    expect(main).toHaveTextContent("child");
    expect(main).toContainElement(screen.getByTestId("tmux-banner"));
    expect(main).toContainElement(screen.getByTestId("fork-banner"));

    const shell = screen.getByTestId("cockpit-shell");
    expect(shell).toContainElement(screen.getByTestId("notification-panel"));
    expect(main).not.toContainElement(screen.getByTestId("notification-panel"));
  });

  it("attaches mainRef to the layout main and makes it a programmatic focus target", () => {
    const mainRef = React.createRef<HTMLElement>();
    render(<RootShell mainRef={mainRef}><span>child</span></RootShell>);
    expect(mainRef.current).toBe(document.getElementById("main-content"));
    expect(mainRef.current?.getAttribute("tabindex")).toBe("-1");
  });

  // The global style mock cannot compute styles, so the rule is checked in the source: one outline removal, on mainContent only.
  it("removes the focus outline only on the layout main's own class", () => {
    const source = fs.readFileSync(path.join(__dirname, "layout.css.ts"), "utf8");
    expect(source.match(/outline/g)).toHaveLength(1);
    expect(source).toMatch(/export const mainContent = style\(\{[\s\S]*"&:focus, &:focus-visible": \{ outline: "none" \}[\s\S]*\}\);\s*$/);
    expect(source).not.toMatch(/boxShadow/);
  });
});
