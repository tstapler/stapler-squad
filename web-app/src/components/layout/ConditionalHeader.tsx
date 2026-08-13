"use client";

import { usePathname } from "next/navigation";
import { Header } from "./Header";

export function ConditionalHeader() {
  const pathname = usePathname();
  if (pathname === "/login" || pathname.startsWith("/test/")) return null;
  return <Header />;
}
