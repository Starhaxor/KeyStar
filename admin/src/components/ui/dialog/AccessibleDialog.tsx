"use client";

import React, { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";

interface AccessibleDialogProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
  showCloseButton?: boolean;
  isFullscreen?: boolean;
  onBackdropClick?: () => void;
}

const focusableSelector = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

const dialogStack: symbol[] = [];
let savedBodyOverflow: string | undefined;

function isTopDialog(dialogId: symbol) {
  return dialogStack[dialogStack.length - 1] === dialogId;
}

function isVisibleFocusableElement(element: HTMLElement) {
  if (
    element.hidden ||
    element.closest('[aria-hidden="true"], [hidden], [inert], fieldset[disabled]')
  ) {
    return false;
  }

  const styles = window.getComputedStyle(element);
  return styles.display !== "none" && styles.visibility !== "hidden" && styles.visibility !== "collapse";
}

function getFocusableElements(container: HTMLElement) {
  return Array.from(container.querySelectorAll<HTMLElement>(focusableSelector)).filter(
    isVisibleFocusableElement
  );
}

export default function AccessibleDialog({
  isOpen,
  onClose,
  title = "Dialog",
  children,
  className = "",
  showCloseButton = true,
  isFullscreen = false,
  onBackdropClick,
}: AccessibleDialogProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);
  const dialogId = useRef(Symbol("accessible-dialog")).current;
  const headingId = useId();

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    if (!isOpen) return;

    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (dialogStack.length === 0) {
      savedBodyOverflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
    }
    dialogStack.push(dialogId);

    const handleDocumentKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && isTopDialog(dialogId)) {
        event.preventDefault();
        onCloseRef.current();
      }
    };
    document.addEventListener("keydown", handleDocumentKeyDown);

    const dialog = dialogRef.current;
    const firstFocusable = dialog && getFocusableElements(dialog)[0];
    (firstFocusable ?? dialog)?.focus();

    return () => {
      document.removeEventListener("keydown", handleDocumentKeyDown);
      const stackIndex = dialogStack.indexOf(dialogId);
      const wasTopDialog = isTopDialog(dialogId);
      if (stackIndex >= 0) {
        dialogStack.splice(stackIndex, 1);
      }
      if (dialogStack.length === 0) {
        document.body.style.overflow = savedBodyOverflow ?? "";
        savedBodyOverflow = undefined;
      }
      if (wasTopDialog && previousFocusRef.current?.isConnected) {
        previousFocusRef.current.focus();
      }
    };
  }, [dialogId, isOpen]);

  if (!isOpen || typeof document === "undefined") return null;

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Tab" || !isTopDialog(dialogId)) return;

    const dialog = dialogRef.current;
    if (!dialog) return;
    const focusable = getFocusableElements(dialog);
    if (focusable.length === 0) {
      event.preventDefault();
      dialog.focus();
      return;
    }

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const panelClasses = isFullscreen
    ? "relative h-full w-full"
    : "relative mx-4 w-full max-w-2xl rounded-3xl bg-white shadow-theme-xl dark:bg-gray-900";

  return createPortal(
    <div
      className="fixed inset-0 z-99999 flex items-center justify-center bg-gray-400/50 p-4 backdrop-blur-[32px] transition-opacity motion-reduce:transition-none"
      onMouseDown={(event) => {
        if (!isFullscreen && event.target === event.currentTarget) {
          if (!isTopDialog(dialogId)) return;
          (onBackdropClick ?? onClose)();
        }
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={headingId}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        className={`${panelClasses} ${className}`}
      >
        <div className="flex items-start justify-between gap-4">
          <h2 id={headingId} className="sr-only">
            {title}
          </h2>
          {showCloseButton && (
            <button
              type="button"
              onClick={onClose}
              aria-label="Close dialog"
              className="absolute right-3 top-3 z-999 flex h-9.5 w-9.5 items-center justify-center rounded-full bg-gray-100 text-gray-400 transition-colors hover:bg-gray-200 hover:text-gray-700 focus:outline-none focus:ring-3 focus:ring-brand-500/30 dark:bg-gray-800 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white sm:right-6 sm:top-6 sm:h-11 sm:w-11 motion-reduce:transition-none"
            >
              <svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none">
                <path
                  fillRule="evenodd"
                  clipRule="evenodd"
                  d="M6.04289 16.5413C5.65237 16.9318 5.65237 17.565 6.04289 17.9555C6.43342 18.346 7.06658 18.346 7.45711 17.9555L11.9987 13.4139L16.5408 17.956C16.9313 18.3466 17.5645 18.3466 17.955 17.956C18.3455 17.5655 18.3455 16.9323 17.955 16.5418L13.4129 11.9997L17.955 7.4576C18.3455 7.06707 18.3455 6.43391 17.955 6.04338C17.5645 5.65286 16.9313 5.65286 16.5408 6.04338L11.9987 10.5855L7.45711 6.0439C7.06658 5.65338 6.43342 5.65338 6.04289 6.0439C5.65237 6.43442 5.65237 7.06759 6.04289 7.45811L10.5845 11.9997L6.04289 16.5413Z"
                  fill="currentColor"
                />
              </svg>
            </button>
          )}
        </div>
        <div className="max-h-[calc(100dvh-2rem)] overflow-y-auto overscroll-contain">
          {children}
        </div>
      </div>
    </div>,
    document.body
  );
}
