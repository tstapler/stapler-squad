"use client";

import { useEffect, useState, useCallback } from "react";
import { ReviewItem } from "@/gen/session/v1/types_pb";

interface UseReviewQueueNavigationOptions {
  items: ReviewItem[];
  onNavigate?: (item: ReviewItem, index: number) => void;
  enableKeyboardShortcuts?: boolean;
}

interface UseReviewQueueNavigationReturn {
  currentIndex: number;
  currentItem: ReviewItem | null;
  goToNext: () => void;
  goToPrevious: () => void;
  goToIndex: (index: number) => void;
  hasNext: boolean;
  hasPrevious: boolean;
}

/**
 * React hook for keyboard navigation through review queue items.
 *
 * Provides keyboard shortcuts for navigating through the review queue:
 * - `]` - Go to next item
 * - `[` - Go to previous item
 *
 * **Features:**
 * - Circular navigation (wraps around at ends)
 * - Keyboard shortcut integration
 * - Current item tracking
 * - Navigation callbacks
 *
 * @example
 * ```tsx
 * const { items } = useReviewQueue();
 * const { currentItem, currentIndex, goToNext, goToPrevious } = useReviewQueueNavigation({
 *   items,
 *   onNavigate: (item, index) => {
 *     console.log(`Navigated to ${item.sessionName} at index ${index}`);
 *   },
 * });
 *
 * return (
 *   <div>
 *     <p>Item {currentIndex + 1} of {items.length}</p>
 *     <p>{currentItem?.sessionName}</p>
 *     <button onClick={goToPrevious}>Previous</button>
 *     <button onClick={goToNext}>Next</button>
 *   </div>
 * );
 * ```
 */
export function useReviewQueueNavigation(
  options: UseReviewQueueNavigationOptions
): UseReviewQueueNavigationReturn {
  const { items, onNavigate, enableKeyboardShortcuts = true } = options;

  const [currentIndex, setCurrentIndex] = useState(0);

  // Reset index if items change and current index is out of bounds
  useEffect(() => {
    if (items.length === 0) {
      setCurrentIndex(0);
    } else if (currentIndex >= items.length) {
      setCurrentIndex(Math.max(0, items.length - 1));
    }
  }, [items, currentIndex]);

  // Get current item
  const currentItem = items.length > 0 ? items[currentIndex] : null;

  // Navigation handlers
  const goToNext = useCallback(() => {
    if (items.length === 0) return;

    const nextIndex = (currentIndex + 1) % items.length;
    setCurrentIndex(nextIndex);

    if (onNavigate && items[nextIndex]) {
      onNavigate(items[nextIndex], nextIndex);
    }
  }, [items, currentIndex, onNavigate]);

  const goToPrevious = useCallback(() => {
    if (items.length === 0) return;

    const prevIndex = currentIndex === 0 ? items.length - 1 : currentIndex - 1;
    setCurrentIndex(prevIndex);

    if (onNavigate && items[prevIndex]) {
      onNavigate(items[prevIndex], prevIndex);
    }
  }, [items, currentIndex, onNavigate]);

  const goToIndex = useCallback(
    (index: number) => {
      if (items.length === 0) return;
      if (index < 0 || index >= items.length) return;

      setCurrentIndex(index);

      if (onNavigate && items[index]) {
        onNavigate(items[index], index);
      }
    },
    [items, onNavigate]
  );

  // Keyboard shortcuts
  useEffect(() => {
    if (!enableKeyboardShortcuts) return;

    const handleKeyPress = (e: KeyboardEvent) => {
      // Ignore if user is typing in an input field
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement
      ) {
        return;
      }

      switch (e.key) {
        case "]":
          e.preventDefault();
          goToNext();
          break;
        case "[":
          e.preventDefault();
          goToPrevious();
          break;
      }
    };

    window.addEventListener("keydown", handleKeyPress);
    return () => window.removeEventListener("keydown", handleKeyPress);
  }, [enableKeyboardShortcuts, goToNext, goToPrevious]);

  // Calculate navigation state
  const hasNext = items.length > 0 && currentIndex < items.length - 1;
  const hasPrevious = items.length > 0 && currentIndex > 0;

  return {
    currentIndex,
    currentItem,
    goToNext,
    goToPrevious,
    goToIndex,
    hasNext,
    hasPrevious,
  };
}
