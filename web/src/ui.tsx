import {
  Button,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@holosama/ui/react";
import type { ReactNode } from "react";
export function ErrorNotice({ error }: { error?: string }) {
  return error ? (
    <div className="notice error" role="alert">
      {error}
    </div>
  ) : null;
}
export function Confirm({
  title,
  children,
  busy,
  onClose,
  onConfirm,
}: {
  title: string;
  children: ReactNode;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <Modal
      isOpen
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
      isDismissable={!busy}
      hideCloseButton={busy}
      className="holo-modal"
    >
      <ModalContent>
        <ModalHeader>{title}</ModalHeader>
        <ModalBody>{children}</ModalBody>
        <ModalFooter>
          <Button variant="ghost" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button disabled={busy} onClick={onConfirm}>
            确认
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
