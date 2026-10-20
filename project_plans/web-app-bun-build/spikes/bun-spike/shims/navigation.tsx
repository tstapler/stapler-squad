import { useSyncExternalStore } from "react";
const sub = (cb: () => void) => { addEventListener("popstate", cb); return () => removeEventListener("popstate", cb); };
export const usePathname = () => useSyncExternalStore(sub, () => location.pathname, () => "/");
export const useSearchParams = () => new URLSearchParams(useSyncExternalStore(sub, () => location.search, () => ""));
export const useParams = () => ({});
export const useRouter = () => ({
  push: (u: string) => { history.pushState({}, "", u); dispatchEvent(new PopStateEvent("popstate")); },
  replace: (u: string) => { history.replaceState({}, "", u); dispatchEvent(new PopStateEvent("popstate")); },
  back: () => history.back(), forward: () => history.forward(), refresh: () => {}, prefetch: () => {},
});
export const redirect = (u: string) => { location.assign(u); };
export const notFound = () => { throw new Error("not found"); };
