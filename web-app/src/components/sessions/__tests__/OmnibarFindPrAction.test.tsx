import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { OmnibarFindPrAction } from "../OmnibarFindPrAction";
import * as lookup from "@/lib/omnibar/findSessionsForPr";

const push = jest.fn();
const addNotification = jest.fn();

jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.memory"),
  useRouter: () => ({ push }),
}));
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotificationCommands: () => ({ addNotification }),
}));
jest.mock("@/lib/omnibar/findSessionsForPr", () => ({
  ...jest.requireActual("@/lib/omnibar/findSessionsForPr"),
  findSessionsForPr: jest.fn(),
}));

const find = lookup.findSessionsForPr as jest.Mock;
const pr = { host: "github.com", owner: "acme", repo: "app", number: 12 };

function setup() {
  const props = { pr, onOpenSession: jest.fn(), onDone: jest.fn(), onFocusInput: jest.fn() };
  render(<OmnibarFindPrAction {...props} />);
  return props;
}

beforeEach(() => jest.clearAllMocks());

describe("OmnibarFindPrAction", () => {
  it("OmnibarFindPrAction_should_nameTheTargetPR", () => {
    setup();
    expect(screen.getByRole("button", { name: /Find sessions for acme\/app#12/ })).toBeInTheDocument();
  });

  it("OmnibarFindPrAction_should_openTheSessionAndToastWithViewAll_When_oneMatch", async () => {
    find.mockResolvedValue({ kind: "single", sessionId: "u1", title: "one", sourceLabel: "Tracked", viewAllHref: "/artifacts?pr=acme/app%2312" });
    const props = setup();
    fireEvent.click(screen.getByTestId("omnibar-find-pr-action"));
    await waitFor(() => expect(props.onOpenSession).toHaveBeenCalledWith("u1"));
    expect(props.onDone).toHaveBeenCalled();
    const toast = addNotification.mock.calls[0][0];
    expect(toast.title).toBe("Opened the session tracking acme/app#12 (Tracked)");
    expect(toast.viewLabel).toBe("View all");
    toast.onView();
    expect(push).toHaveBeenCalledWith("/artifacts?pr=acme/app%2312");
  });

  it("OmnibarFindPrAction_should_goToTheArtifactsList_When_noneOrSeveralMatch", async () => {
    find.mockResolvedValue({ kind: "list", href: "/artifacts?pr=acme/app%2312" });
    const props = setup();
    fireEvent.click(screen.getByTestId("omnibar-find-pr-action"));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/artifacts?pr=acme/app%2312"));
    expect(props.onOpenSession).not.toHaveBeenCalled();
    expect(addNotification).not.toHaveBeenCalled();
  });

  it("OmnibarFindPrAction_should_stayOpenWithRetry_When_lookupFails", async () => {
    find.mockRejectedValueOnce(new Error("boom"));
    const props = setup();
    fireEvent.click(screen.getByTestId("omnibar-find-pr-action"));
    expect(await screen.findByText("Couldn't look up sessions for this PR")).toBeInTheDocument();
    expect(props.onDone).not.toHaveBeenCalled();

    find.mockResolvedValue({ kind: "list", href: "/artifacts?pr=acme/app%2312" });
    fireEvent.click(screen.getByTestId("omnibar-find-pr-retry"));
    await waitFor(() => expect(props.onDone).toHaveBeenCalled());
    expect(screen.queryByTestId("omnibar-find-pr-error")).not.toBeInTheDocument();
  });

  it("OmnibarFindPrAction_should_keepKeysAwayFromTheOmnibarSubmit_When_enterOrArrowUp", () => {
    const props = setup();
    const outer = jest.fn();
    render(<div onKeyDown={outer}><OmnibarFindPrAction {...props} /></div>);
    const buttons = screen.getAllByTestId("omnibar-find-pr-action");
    const button = buttons[buttons.length - 1];
    fireEvent.keyDown(button, { key: "Enter" });
    fireEvent.keyDown(button, { key: "ArrowUp" });
    expect(outer).not.toHaveBeenCalled();
    expect(props.onFocusInput).toHaveBeenCalled();
  });
});
