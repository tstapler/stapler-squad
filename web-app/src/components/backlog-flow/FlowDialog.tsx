"use client";

import { useRef, type ReactNode } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { Skeleton } from "@/components/ui/Skeleton";
import { content, footer, overlay, title } from "@/components/ui/Modal.css";
import * as pageStyles from "@/app/backlog/flow/flow.css";
import * as styles from "./flow.css";

const SKELETON_ROWS = 4;

interface FlowDialogProps {
  title: string;
  testId: string;
  onClose: () => void;
  /** Where focus goes on close: the button that opened the dialog. */
  returnFocusTo: HTMLElement | null;
  busy?: boolean;
  children: ReactNode;
}

/**
 * Modal shell shared by the drill-down drawer and the item timeline: focus lands on the heading,
 * Tab stays inside (Radix), Escape, X and Close all call `onClose`, focus returns to the invoker.
 */
export function FlowDialog({ title: heading, testId, onClose, returnFocusTo, busy = false, children }: FlowDialogProps) {
  const headingRef = useRef<HTMLHeadingElement>(null);
  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className={overlay} />
        <Dialog.Content
          className={`${content} ${styles.dialogContent}`}
          data-testid={testId}
          aria-busy={busy || undefined}
          aria-describedby={undefined}
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            headingRef.current?.focus();
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            returnFocusTo?.focus();
          }}
        >
          <Dialog.Title ref={headingRef} tabIndex={-1} className={`${title} ${styles.dialogTitle}`}>
            {heading}
          </Dialog.Title>
          <button type="button" className={styles.dialogClose} aria-label="Close dialog" onClick={onClose}>{/* analytics-exempt */}
            ×
          </button>
          {children}
          <div className={footer}>
            <button type="button" className={styles.textButton} onClick={onClose}>{/* analytics-exempt */}
              Close
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function DialogSkeleton() {
  return (
    <div className={pageStyles.skeletonRows} aria-busy="true" data-testid="flow-dialog-skeleton">
      {Array.from({ length: SKELETON_ROWS }, (_, i) => (
        <Skeleton key={i} height={20} />
      ))}
    </div>
  );
}

export function DialogError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className={pageStyles.stateBlock} role="alert">
      <p>
        {message} <button type="button" className={styles.textButton} onClick={onRetry}>{/* analytics-exempt */}Retry</button>
      </p>
    </div>
  );
}
