"use client";
import React from "react";
import AccessibleDialog from "@/components/ui/dialog/AccessibleDialog";

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
  showCloseButton?: boolean;
  isFullscreen?: boolean;
}

function findHeading(children: React.ReactNode): React.ReactNode | undefined {
  for (const child of React.Children.toArray(children)) {
    if (!React.isValidElement<{ children?: React.ReactNode }>(child)) continue;

    if (typeof child.type === "string" && /^h[1-6]$/.test(child.type)) {
      return child.props.children;
    }

    const nestedHeading = findHeading(child.props.children);
    if (nestedHeading) return nestedHeading;
  }
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  children,
  className,
  showCloseButton = true,
  isFullscreen = false,
}) => {
  const dialogTitle = title ?? findHeading(children) ?? "Modal dialog";

  return (
    <AccessibleDialog
      isOpen={isOpen}
      onClose={onClose}
      title={dialogTitle}
      className={className}
      showCloseButton={showCloseButton}
      isFullscreen={isFullscreen}
    >
      {children}
    </AccessibleDialog>
  );
};
