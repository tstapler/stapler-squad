// +feature: ui-modal
import type { Meta, StoryObj } from "@storybook/react";
import { Modal, ModalTrigger, ModalContent, ModalTitle, ModalDescription } from "./Modal";
import { Button } from "./Button";

const meta: Meta = { title: "UI/Modal" };
export default meta;
type Story = StoryObj;

export const Closed: Story = {
  render: () => (
    <Modal>
      <ModalTrigger asChild><Button>Open</Button></ModalTrigger>
      <ModalContent>
        <ModalTitle>Confirm</ModalTitle>
        <ModalDescription>Are you sure?</ModalDescription>
      </ModalContent>
    </Modal>
  ),
};
export const OpenByDefault: Story = {
  render: () => (
    <Modal defaultOpen>
      <ModalContent>
        <ModalTitle>Confirm</ModalTitle>
        <ModalDescription>Are you sure?</ModalDescription>
      </ModalContent>
    </Modal>
  ),
};
