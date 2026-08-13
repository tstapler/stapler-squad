"use client";

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  ReactNode,
} from "react";

const STORAGE_KEY = "nav-drawer-open";
const BREAKPOINT_PX = 1024;

interface NavigationContextValue {
  isDrawerOpen: boolean;
  toggleDrawer: () => void;
  openDrawer: () => void;
  closeDrawer: () => void;
}

const NavigationContext = createContext<NavigationContextValue | null>(null);

export function useNavigation(): NavigationContextValue {
  const ctx = useContext(NavigationContext);
  if (!ctx) {
    throw new Error("useNavigation must be used within a NavigationProvider");
  }
  return ctx;
}

interface NavigationProviderProps {
  children: ReactNode;
}

export function NavigationProvider({ children }: NavigationProviderProps) {
  // Read initial state from localStorage, but defer to a useEffect so SSR
  // and client render match (avoids hydration mismatch).
  const [isDrawerOpen, setIsDrawerOpen] = useState(true);
  const initialized = useRef(false);

  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;

    const narrow = window.innerWidth < BREAKPOINT_PX;
    if (narrow) {
      setIsDrawerOpen(false);
      return;
    }
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored !== null) {
        setIsDrawerOpen(stored === "true");
      }
    } catch {
      // localStorage unavailable — keep default
    }
  }, []);

  // Auto-close when viewport shrinks below breakpoint
  useEffect(() => {
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const width = entry.contentRect.width;
        if (width < BREAKPOINT_PX) {
          setIsDrawerOpen(false);
        }
      }
    });
    observer.observe(document.documentElement);
    return () => observer.disconnect();
  }, []);

  const persist = useCallback((open: boolean) => {
    try {
      localStorage.setItem(STORAGE_KEY, String(open));
    } catch {
      // ignore
    }
  }, []);

  const openDrawer = useCallback(() => {
    setIsDrawerOpen(true);
    persist(true);
  }, [persist]);

  const closeDrawer = useCallback(() => {
    setIsDrawerOpen(false);
    persist(false);
  }, [persist]);

  const toggleDrawer = useCallback(() => {
    setIsDrawerOpen((prev) => {
      const next = !prev;
      persist(next);
      return next;
    });
  }, [persist]);

  return (
    <NavigationContext.Provider
      value={{ isDrawerOpen, toggleDrawer, openDrawer, closeDrawer }}
    >
      {children}
    </NavigationContext.Provider>
  );
}
