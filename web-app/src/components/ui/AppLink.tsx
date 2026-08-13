import Link, { LinkProps } from "next/link";
import { forwardRef, AnchorHTMLAttributes, ReactNode } from "react";

type AppLinkProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, keyof LinkProps> &
  LinkProps & {
    children?: ReactNode;
  };

/**
 * Custom Link wrapper that disables prefetching by default
 * to prevent CSS preload warnings in the browser console.
 *
 * @see https://github.com/vercel/next.js/discussions/49607
 */
export const AppLink = forwardRef<HTMLAnchorElement, AppLinkProps>(
  function AppLink({ prefetch = false, ...props }, ref) {
    return <Link ref={ref} prefetch={prefetch} {...props} />;
  }
);

export default AppLink;
