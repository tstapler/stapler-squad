/**
 * One behavioural suite for every `NavImpl`, so drift between impl.next, impl.router and impl.memory is a red test.
 *
 * For impl.next the harness mocks only the external `next/navigation` module and imports the real wrapper: the suite
 * proves the wrapper's own logic (normalisation, option mapping, ReplaceSearch), not Next itself. Parity with real
 * Next is evidenced only by the Next-baselined e2e specs (plan Stories 7.1-7.4).
 */
import { act, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import type { ReactElement } from "react";
import type { NavImpl, NavRouter, NavSearchParams } from "../impl.types";

export interface NavHarness {
  name: string;
  impl: NavImpl;
  /** Mounts `ui` in the implementation's environment with the browser URL and its history at `url`. */
  mount(ui: ReactElement, url: string): void;
  /** After `resetScroll: false` navigation, whether the implementation asked to keep the scroll position. */
  keptScroll(): boolean;
  /** After `<Redirect to>` mounted, whether the implementation redirected to `to`; `shown` is what the hooks report. */
  redirectedTo(to: string, shown: string | null): boolean;
  /** Parts a harness cannot observe (Next is mocked; its Link and search-param sync are not the wrapper's logic). */
  observes: { linkClick: boolean; replaceSearchHooks: boolean; scrollOption: boolean };
}

interface Handles {
  router: NavRouter;
  params: NavSearchParams;
  replaceSearch: (search: string) => void;
  bump: () => void;
}

const settle = () => act(async () => void (await new Promise((resolve) => setTimeout(resolve, 20))));
const location = () => screen.getByTestId("loc").textContent;

export function navContract(harness: NavHarness): void {
  const { impl } = harness;
  let handles: Handles;

  function Probe({ children }: { children?: React.ReactNode }) {
    const [, setCount] = useState(0);
    const router = impl.useRouter();
    const params = impl.useSearchParams();
    const pathname = impl.usePathname();
    const replaceSearch = impl.useReplaceSearch();
    handles = { router, params, replaceSearch, bump: () => setCount((count) => count + 1) };
    const search = params.toString();
    return (
      <>
        <output data-testid="loc">{`${pathname}${search ? `?${search}` : ""}`}</output>
        {children}
      </>
    );
  }

  const mount = (url: string, children?: React.ReactNode) => harness.mount(<Probe>{children}</Probe>, url);
  const run = async (action: () => void) => {
    act(action);
    await settle();
  };

  describe(`NavImpl contract: ${harness.name}`, () => {
    it("reports a pathname without the trailing slash, and `/` for the root", () => {
      mount("/review-queue/");
      expect(location()).toBe("/review-queue");
    });

    it("keeps the root path as `/`", () => {
      mount("/");
      expect(location()).toBe("/");
    });

    it("reads search params as a read-only view", () => {
      mount("/unfinished?tab=a&item=7");
      expect(handles.params.get("tab")).toBe("a");
      expect(handles.params.get("item")).toBe("7");
    });

    it("returns the same search-params object across re-renders with an unchanged URL", () => {
      mount("/unfinished?tab=a&item=7");
      const first = handles.params;
      act(() => handles.bump());
      expect(handles.params).toBe(first);
    });

    it("returns the same router object across re-renders", () => {
      mount("/start");
      const first = handles.router;
      act(() => handles.bump());
      expect(handles.router).toBe(first);
    });

    it("push adds an entry and back returns to the previous one; forward goes again", async () => {
      mount("/start");
      await run(() => handles.router.push("/one"));
      expect(location()).toBe("/one");
      await run(() => handles.router.back());
      expect(location()).toBe("/start");
      await run(() => handles.router.forward());
      expect(location()).toBe("/one");
    });

    it("replace swaps the current entry instead of adding one", async () => {
      mount("/start");
      await run(() => handles.router.push("/one"));
      await run(() => handles.router.replace("/two"));
      expect(location()).toBe("/two");
      await run(() => handles.router.back());
      expect(location()).toBe("/start");
    });

    it("push and replace carry a query string", async () => {
      mount("/start");
      await run(() => handles.router.push("/?new=true"));
      expect(location()).toBe("/?new=true");
      await run(() => handles.router.replace("/history?tab=b"));
      expect(location()).toBe("/history?tab=b");
    });

    (harness.observes.scrollOption ? it.each : it.skip.each)([
      ["push", (router: NavRouter, options?: { resetScroll?: boolean }) => router.push("/a", options)],
      ["replace", (router: NavRouter, options?: { resetScroll?: boolean }) => router.replace("/a", options)],
    ])("%s with resetScroll:false asks to keep the scroll position, and by default does not", async (_name, go) => {
      mount("/start");
      await run(() => go(handles.router));
      expect(harness.keptScroll()).toBe(false);
      await run(() => go(handles.router, { resetScroll: false }));
      expect(harness.keptScroll()).toBe(true);
    });

    it("Link renders an anchor with its href", () => {
      mount("/start", <impl.Link href="/rules?x=1" data-testid="link">Rules</impl.Link>);
      expect(screen.getByTestId("link").getAttribute("href")).toBe("/rules?x=1");
    });

    it("leaves a same-page hash link to the browser", async () => {
      mount("/start", <impl.Link href="#main-content" data-testid="skip">Skip</impl.Link>);
      fireEvent.click(screen.getByTestId("skip"));
      await settle();
      expect(location()).toBe("/start");
    });

    (harness.observes.linkClick ? it : it.skip)("Link click pushes, and with `replace` replaces", async () => {
      mount(
        "/start",
        <>
          <impl.Link href="/pushed" data-testid="push">p</impl.Link>
          <impl.Link href="/replaced" replace data-testid="replace">r</impl.Link>
        </>,
      );
      fireEvent.click(screen.getByTestId("push"));
      await settle();
      expect(location()).toBe("/pushed");
      fireEvent.click(screen.getByTestId("replace"));
      await settle();
      expect(location()).toBe("/replaced");
      await run(() => handles.router.back());
      expect(location()).toBe("/start");
    });

    it("Redirect goes to its target", async () => {
      mount("/start", <impl.Redirect to="/settings?tab=config-files" />);
      await settle();
      expect(harness.redirectedTo("/settings?tab=config-files", location())).toBe(true);
    });

    it("useReplaceSearch changes the query, keeps the path, and keeps foreign history.state", async () => {
      mount("/unfinished?tab=a");
      window.history.replaceState({ ...window.history.state, ssqTray: true }, "");
      await run(() => handles.replaceSearch("?tab=b"));
      expect(window.location.pathname).toBe("/unfinished");
      expect(window.location.search).toBe("?tab=b");
      expect(window.history.state).toEqual(expect.objectContaining({ ssqTray: true }));
      if (harness.observes.replaceSearchHooks) expect(location()).toBe("/unfinished?tab=b");
    });

    it("useReplaceSearch accepts a query without the leading `?` and clears with an empty string", async () => {
      mount("/unfinished?tab=a");
      await run(() => handles.replaceSearch("tab=c"));
      expect(window.location.search).toBe("?tab=c");
      await run(() => handles.replaceSearch(""));
      expect(window.location.search).toBe("");
      expect(window.location.pathname).toBe("/unfinished");
    });

    it("replaceHash sets and clears the hash, keeping state", () => {
      mount("/x?y=1");
      window.history.replaceState({ ...window.history.state, keep: 1 }, "");
      impl.replaceHash("#file=a%2Fb");
      expect(window.location.hash).toBe("#file=a%2Fb");
      impl.replaceHash("");
      expect(window.location.hash).toBe("");
      expect(window.location.search).toBe("?y=1");
      expect(window.history.state).toEqual(expect.objectContaining({ keep: 1 }));
    });

    it("pushTrayEntry adds one marked entry and, with replace, reuses the current one", () => {
      mount("/");
      const before = window.history.length;
      impl.pushTrayEntry();
      expect(window.history.length).toBe(before + 1);
      expect(window.history.state).toEqual(expect.objectContaining({ ssqTray: true }));
      impl.pushTrayEntry(true);
      expect(window.history.length).toBe(before + 1);
    });

    it("pushHistoryMarker adds a keyed marker to the state; replace reuses the entry", () => {
      mount("/");
      window.history.replaceState({ ...window.history.state, keep: 1 }, "");
      const before = window.history.length;
      impl.pushHistoryMarker("depth", 2);
      expect(window.history.length).toBe(before + 1);
      expect(window.history.state).toEqual(expect.objectContaining({ keep: 1, depth: 2 }));
      impl.pushHistoryMarker("depth", 3, true);
      expect(window.history.length).toBe(before + 1);
      expect(window.history.state).toEqual(expect.objectContaining({ keep: 1, depth: 3 }));
    });
  });
}

