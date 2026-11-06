import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SavedForLaterSection } from "./SavedForLaterSection";
import { makeGuidanceRow } from "./__fixtures__/notificationFixtures";

jest.mock("@/lib/nav", () =>
  require("@/lib/nav/testing-mock").navMockFromNext({
    link: () => ({
      __esModule: true,
      default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => (
        <a href={href} {...props}>
          {children}
        </a>
      ),
    }),
  }),
);

const saved = (over = {}) =>
  makeGuidanceRow({ id: "g1", source: "agent_mcp", status: "answered", answer: "Redis", deliveryState: "saved_for_later", deliveryRoute: "resume", sessionUuid: "uuid-1", ...over });

describe("SavedForLaterSection (UX-AC79)", () => {
  it("renders nothing when no answer is saved", () => {
    const { container } = render(<SavedForLaterSection rows={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("is a collapsed disclosure with the count in its text", async () => {
    render(<SavedForLaterSection rows={[saved(), saved({ id: "g2", deliveryRoute: "read_only" })]} />);
    const button = screen.getByRole("button", { name: /Saved for later \(2\)/ });
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByTestId("saved-for-later-list")).toBeNull();
    await userEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByTestId("saved-for-later-list")).toBeInTheDocument();
  });

  it("lists session, state, question and saved answer with Open session as the only control", async () => {
    render(<SavedForLaterSection rows={[saved(), saved({ id: "g2", deliveryRoute: "read_only", questionText: "Ship it?", answer: "Yes", sessionUuid: "uuid-2" })]} sessionNames={{ "uuid-1": "review:pr-1042" }} />);
    await userEvent.click(screen.getByRole("button", { name: /Saved for later/ }));
    const stopped = screen.getByTestId("saved-for-later-g1");
    expect(stopped).toHaveTextContent("review:pr-1042 (stopped): Which cache? - Redis");
    expect(within(stopped).getByRole("link", { name: "Open session" })).toHaveAttribute("href", "/?session=uuid-1");
    expect(screen.getByTestId("saved-for-later-g2")).toHaveTextContent("(archived): Ship it? - Yes");
    expect(within(screen.getByTestId("saved-for-later-list")).queryAllByRole("button")).toHaveLength(0);
  });
});
