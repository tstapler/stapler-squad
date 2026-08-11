'use client';
import { createContext, useContext, useEffect, useState, ReactNode } from 'react';

interface ViewportContextValue {
  isMobile: boolean;    // < 600px
  isFoldable: boolean;  // 600px–899px
  isInnerScreen: boolean; // >= 900px
}

const ViewportContext = createContext<ViewportContextValue>({
  isMobile: true,
  isFoldable: false,
  isInnerScreen: false,
});

export function useViewport() {
  return useContext(ViewportContext);
}

export function ViewportProvider({ children }: { children?: ReactNode }) {
  const [viewport, setViewport] = useState<ViewportContextValue>({
    isMobile: true,
    isFoldable: false,
    isInnerScreen: false,
  });

  useEffect(() => {
    // Set CSS variables from visualViewport (keyboard height, viewport height)
    const vv = window.visualViewport;
    if (!vv) return;

    const update = () => {
      requestAnimationFrame(() => {
        // Must listen to both resize AND scroll events on iOS Safari —
        // scroll fires during keyboard transitions alongside resize.
        const kb = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
        document.documentElement.style.setProperty('--keyboard-height', `${kb}px`);
        document.documentElement.style.setProperty('--viewport-height', `${vv.height}px`);
      });
    };

    vv.addEventListener('resize', update);
    vv.addEventListener('scroll', update);
    update();
    return () => {
      vv.removeEventListener('resize', update);
      vv.removeEventListener('scroll', update);
    };
  }, []);

  useEffect(() => {
    // Track breakpoint state for responsive rendering
    const update = () => {
      const w = window.innerWidth;
      setViewport({
        isMobile: w < 600,
        isFoldable: w >= 600 && w < 900,
        isInnerScreen: w >= 900,
      });
    };
    update();
    window.addEventListener('resize', update);
    return () => window.removeEventListener('resize', update);
  }, []);

  return (
    <ViewportContext.Provider value={viewport}>
      {children}
    </ViewportContext.Provider>
  );
}
