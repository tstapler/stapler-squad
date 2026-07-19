import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";
import {
  backdrop as reviewBackdrop,
  modal as reviewModal,
  modalHeader as reviewModalHeader,
  modalTitle as reviewModalTitle,
  modalLabel as reviewModalLabel,
  closeButton as reviewCloseButton,
  openTerminalLink as reviewOpenTerminalLink,
} from "./ReviewChangesModal.css";

// Reuse the ReviewChangesModal chrome (backdrop, header, title, close button) —
// same modal shell, different body layout below.
export const backdrop = reviewBackdrop;
export const modal = reviewModal;
export const modalHeader = reviewModalHeader;
export const modalTitle = reviewModalTitle;
export const modalLabel = reviewModalLabel;
export const closeButton = reviewCloseButton;
export const openTerminalLink = reviewOpenTerminalLink;

export const modalBody = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  background: vars.color.terminalBackground,
});

export const treePane = style({
  width: "280px",
  flexShrink: 0,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  borderRight: `1px solid ${vars.color.borderSubtle}`,
  "@media": {
    "(max-width: 768px)": {
      width: "160px",
    },
  },
});

export const contentPane = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  minWidth: 0,
});
