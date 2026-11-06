import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "@/gen/session/v1/types_pb";
import { PRSessionChip } from "./PRSessionChip";

jest.mock("./PRSessionChip.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));
jest.mock("@/lib/nav", () => ({
  Link: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => <a href={href} {...rest}>{children}</a>,
}));

const s = (id: string, title: string) => create(SessionSchema, { id, title });

describe("PRSessionChip", () => {
  it("PRSessionChip_should_renderNothing_When_NoSessionTracksThePr", () => {
    const { container } = render(<PRSessionChip sessions={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("PRSessionChip_should_linkTheSession_When_ExactlyOne", () => {
    render(<PRSessionChip sessions={[s("a", "refactor-auth")]} />);
    const link = screen.getByRole("link", { name: "session: refactor-auth" });
    expect(link).toHaveTextContent("session: refactor-auth");
    expect(link).not.toHaveAttribute("aria-label");
    expect(link).toHaveAttribute("href", "/?session=a");
  });

  it("PRSessionChip_should_openAMenuOfLinks_When_TwoOrMoreAndCloseOnEscapeReturningFocus", () => {
    render(<PRSessionChip sessions={[s("a", "refactor-auth"), s("b", "rework-auth")]} />);
    const trigger = screen.getByRole("button", { name: /2 sessions/ });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("link", { name: /Open session/ })).toBeNull();

    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("link", { name: "Open session refactor-auth" })).toHaveAttribute("href", "/?session=a");
    expect(screen.getByRole("link", { name: "Open session rework-auth" })).toHaveAttribute("href", "/?session=b");

    fireEvent.keyDown(screen.getByRole("link", { name: "Open session rework-auth" }), { key: "Escape" });
    expect(screen.queryByRole("link", { name: /Open session/ })).toBeNull();
    expect(trigger).toHaveFocus();
  });

  it("PRSessionChip_should_closeTheMenu_When_ClickingOutside", () => {
    render(<div><PRSessionChip sessions={[s("a", "x"), s("b", "y")]} /><button>outside</button></div>);
    fireEvent.click(screen.getByRole("button", { name: /2 sessions/ }));
    fireEvent.mouseDown(screen.getByRole("button", { name: "outside" }));
    expect(screen.queryByRole("link", { name: /Open session/ })).toBeNull();
  });
});
