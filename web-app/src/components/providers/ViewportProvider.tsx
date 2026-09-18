'use client';
import { createContext, useContext, useEffect, useState, ReactNode } from 'react';

interface ViewportContextValue {
  isMobile: boolean;    // < 600px
  isFoldable: boolean;  // 600px–899px
  isInnerScreen: boolean; // >= 900px
  hasFinePointer: boolean; // real mouse/trackpad attached (matchMedia: hover:hover and pointer:fine)
  isVirtualKeyboardOpen: boolean; // on-screen/virtual keyboard currently showing (visualViewport shrink)
}

const ViewportContext = createContext<ViewportContextValue>({
  isMobile: true,
  isFoldable: false,
  isInnerScreen: false,
  hasFinePointer: false,
  isVirtualKeyboardOpen: false,
});

export function useViewport() {
  return useContext(ViewportContext);
}

export function ViewportProvider({ children }: { children?: ReactNode }) {
  const [viewport, setViewport] = useState<ViewportContextValue>({
    isMobile: true,
    isFoldable: false,
    isInnerScreen: false,
    hasFinePointer: false,
    isVirtualKeyboardOpen: false,
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
        // Small threshold avoids false positives from browser chrome/rounding jitter.
        setViewport(prev => ({ ...prev, isVirtualKeyboardOpen: kb > 100 }));
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
    // Detects a real mouse/trackpad attached to an otherwise-touch device
    // (Bluetooth/USB mouse on Android Chrome or iPadOS Safari) — updates live.
    // Must use any-hover/any-pointer, not hover/pointer: the non-"any" features
    // reflect the primary input mechanism, which stays touch/coarse on most
    // mobile browsers even once a mouse is attached.
    const mq = window.matchMedia('(any-hover: hover) and (any-pointer: fine)');
    const update = () => {
      setViewport(prev => ({ ...prev, hasFinePointer: mq.matches }));
    };
    mq.addEventListener('change', update);
    update();
    return () => mq.removeEventListener('change', update);
  }, []);

  useEffect(() => {
    // Track breakpoint state for responsive rendering
    const update = () => {
      const w = window.innerWidth;
      setViewport(prev => ({
        ...prev,
        isMobile: w < 600,
        isFoldable: w >= 600 && w < 900,
        isInnerScreen: w >= 900,
      }));
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
