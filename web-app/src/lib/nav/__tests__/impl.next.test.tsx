/**
 * The real `impl.next.tsx` (jest.requireActual, bypassing the global "@/lib/nav" stub) over a mocked
 * `next/navigation` that is backed by the in-memory history. Proves the wrapper's own logic, not Next itself.
 */
import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import type * as NextImplModule from "../impl.next";
import { createMemoryNav } from "../impl.memory";
import { navContract } from "./nav-contract";

type NavOptions = { scroll?: boolean } | undefined;

let mockStore = createMemoryNav(["/"]);
let mockLastOptions: NavOptions;
const mockRedirects: string[] = [];

// doMock, not jest.mock: this suite tests impl.next itself, so it must keep mocking next/navigation after the
// test-mock codemod has moved every other suite onto "@/lib/nav".
jest.doMock("next/navigation", () => {
  const { useMemo, useSyncExternalStore } = jest.requireActual("react");
  const subscribe = (listener: () => void) => mockStore.subscribe(listener);
  const read = () => mockStore.getLocation();
  const router = {
    push: (href: string, options: NavOptions) => {
      mockLastOptions = options;
      mockStore.router.push(href);
    },
    replace: (href: string, options: NavOptions) => {
      mockLastOptions = options;
      mockStore.router.replace(href);
    },
    back: () => mockStore.router.back(),
    forward: () => mockStore.router.forward(),
  };
  return {
    useRouter: () => router,
    usePathname: () => useSyncExternalStore(subscribe, read, read).split("?")[0],
    useSearchParams: () => {
      const search = useSyncExternalStore(subscribe, read, read).split("?")[1] ?? "";
      return useMemo(() => new URLSearchParams(search), [search]);
    },
    redirect: (to: string) => {
      mockRedirects.push(to);
    },
  };
});

const nextModule = jest.requireActual<typeof NextImplModule>("../impl.next");

beforeEach(() => {
  mockStore = createMemoryNav(["/"]);
  mockLastOptions = undefined;
  mockRedirects.length = 0;
});

it("is the real wrapper, not the global facade stub", () => {
  expect(jest.isMockFunction(nextModule.useRouter)).toBe(false);
  expect(nextModule.nextImpl.useRouter).toBe(nextModule.useRouter);
  expect(nextModule).not.toBe(jest.requireMock("@/lib/nav"));
});

navContract({
  name: "impl.next",
  impl: nextModule.nextImpl,
  mount(ui: ReactElement, url: string) {
    window.history.pushState(null, "", url);
    mockStore = createMemoryNav([url]);
    render(ui);
  },
  keptScroll: () => mockLastOptions?.scroll === false,
  redirectedTo: (to) => mockRedirects.includes(to),
  // next/link is globally bridged to the facade and Next syncs raw history writes itself: neither is this wrapper's logic.
  observes: { linkClick: false, replaceSearchHooks: false, scrollOption: true },
});
