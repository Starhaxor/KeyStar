"use client";
import React from "react";
import AccessibleDialog from "@/components/ui/dialog/AccessibleDialog";

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  className?: string;
  children: React.ReactNode;
  showCloseButton?: boolean;
  isFullscreen?: boolean;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  children,
  className,
  showCloseButton = true,
  isFullscreen = false,
}) => {
  return (
    <AccessibleDialog
      isOpen={isOpen}
      onClose={onClose}
      title="Dialog"
      className={className}
      showCloseButton={showCloseButton}
      isFullscreen={isFullscreen}
    >
      {children}
    </AccessibleDialog>
  );
};
