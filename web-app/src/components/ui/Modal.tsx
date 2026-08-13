"use client";
import * as Dialog from "@radix-ui/react-dialog";
import { overlay, content, title, description, footer, closeButton } from "./Modal.css";

export const Modal = Dialog.Root;
export const ModalTrigger = Dialog.Trigger;

const srOnly: React.CSSProperties = {
  position: "absolute",
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: "hidden",
  clipPath: "inset(50%)",
  whiteSpace: "nowrap",
  border: 0,
};

interface ModalContentProps extends React.ComponentPropsWithoutRef<typeof Dialog.Content> {
  showClose?: boolean;
  /** Accessible title rendered visually hidden. Use when no visible ModalTitle is present. */
  fallbackTitle?: string;
}

export function ModalContent({ children, showClose = true, fallbackTitle, className, ...props }: ModalContentProps) {
  return (
    <Dialog.Portal>
      <Dialog.Overlay className={overlay} />
      <Dialog.Content className={[content, className].filter(Boolean).join(" ")} {...props}>
        {/* Provide an accessible name for the dialog when no visible ModalTitle is used.
            Pass fallbackTitle when children contain no ModalTitle; omit it when
            children include a ModalTitle (which renders its own Dialog.Title). */}
        {fallbackTitle != null && (
          <Dialog.Title style={srOnly}>{fallbackTitle}</Dialog.Title>
        )}
        {showClose && (
          <Dialog.Close className={closeButton} aria-label="Close">
            ×
          </Dialog.Close>
        )}
        {children}
      </Dialog.Content>
    </Dialog.Portal>
  );
}

export function ModalTitle({ children, ...props }: React.ComponentPropsWithoutRef<typeof Dialog.Title>) {
  return (
    <Dialog.Title className={title} {...props}>
      {children}
    </Dialog.Title>
  );
}

export function ModalDescription({
  children,
  ...props
}: React.ComponentPropsWithoutRef<typeof Dialog.Description>) {
  return (
    <Dialog.Description className={description} {...props}>
      {children}
    </Dialog.Description>
  );
}

export const ModalClose = Dialog.Close;

export function ModalFooter({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={footer} {...props}>
      {children}
    </div>
  );
}
