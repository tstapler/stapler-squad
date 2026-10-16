// Shared movement threshold for touch-gesture disambiguation on the window
// tab strip: WindowTabStrip's long-press-to-rename check (Story 3.1.2) and
// useWindowSwipe's swipe-vs-scroll check (Epic 3.2, a later task) both need
// "did this touch move enough to not be a stationary hold" to agree on one
// number rather than two independently-tuned ones.
export const GESTURE_MOVEMENT_THRESHOLD_PX = 10;
