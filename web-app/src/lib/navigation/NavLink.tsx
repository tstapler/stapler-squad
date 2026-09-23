"use client";

import Link, { LinkProps } from "next/link";
import { forwardRef, AnchorHTMLAttributes, ReactNode } from "react";

type NavLinkProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, keyof LinkProps> &
  LinkProps & {
    children?: ReactNode;
  };

/**
 * Custom Link wrapper that disables prefetching by default
 * to prevent CSS preload warnings in the browser console.
 *
 * Lives in `lib/` (rather than `components/ui/`) so `shared/**` components
 * can use it too — `boundaries/dependencies` in .eslintrc.json disallows
 * `shared` -> `ui`, but allows `shared` -> `lib`. `AppLink` re-exports this
 * under its established name for existing `ui`/`sessions`-tree callers.
 *
 * @see https://github.com/vercel/next.js/discussions/49607
 */
export const NavLink = forwardRef<HTMLAnchorElement, NavLinkProps>(
  function NavLink({ prefetch = false, ...props }, ref) {
    return <Link ref={ref} prefetch={prefetch} {...props} />;
  }
);
