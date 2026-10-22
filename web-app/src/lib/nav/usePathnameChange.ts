import { useEffect, useRef } from "react";
import { usePathname } from "./impl.router";

/** Runs `onChange` after a commit that changed the normalised pathname; never on first render, search or hash changes. */
export function usePathnameChange(onChange: (pathname: string) => void): void {
  const pathname = usePathname();
  const previous = useRef(pathname);
  const latest = useRef(onChange);
  latest.current = onChange;
  useEffect(() => {
    if (previous.current === pathname) return;
    previous.current = pathname;
    latest.current(pathname);
  }, [pathname]);
}
