import type { MutableRefObject, Ref, RefCallback } from "react";

/**
 * Composes multiple refs (callback refs or ref objects) into a single ref callback, so one
 * DOM node can be attached to several independent ref sources — e.g. a virtualizer's scroll
 * ref and dnd-kit's `useDroppable` `setNodeRef` on the same scrollable container.
 */
export function mergeRefs<T>(...refs: Array<Ref<T> | undefined | null>): RefCallback<T> {
  return (node: T) => {
    for (const ref of refs) {
      if (ref == null) continue;
      if (typeof ref === "function") {
        ref(node);
      } else {
        (ref as MutableRefObject<T | null>).current = node;
      }
    }
  };
}
