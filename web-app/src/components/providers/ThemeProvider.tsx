"use client";

import { useEffect } from "react";
import { lightTheme, darkTheme } from "@/styles/theme.css";

export function ThemeProvider() {
  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");

    const apply = (dark: boolean) => {
      const html = document.documentElement;
      html.classList.remove(lightTheme, darkTheme);
      html.classList.add(dark ? darkTheme : lightTheme);
    };

    apply(mq.matches);

    const handler = (e: MediaQueryListEvent) => apply(e.matches);
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, []);

  return null;
}
