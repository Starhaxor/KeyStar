"use client";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import React from "react";

interface ConfirmModalProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmLabel: string;
  busy: boolean;
  error?: string | null;
  onConfirm: () => void;
  onClose: () => void;
}

export default function ConfirmModal({
  isOpen,
  title,
  message,
  confirmLabel,
  busy,
  error,
  onConfirm,
  onClose,
}: ConfirmModalProps) {
  return (
    <Modal isOpen={isOpen} onClose={onClose} className="max-w-md p-6">
      <h3 className="mb-2 text-lg font-semibold text-gray-800 dark:text-white/90">
        {title}
      </h3>
      <p className="text-sm text-gray-500 dark:text-gray-400">{message}</p>
      {error && <p role="alert" className="mt-3 text-sm text-error-500">{error}</p>}
      <div className="mt-6 flex justify-end gap-3">
        <Button variant="outline" size="sm" onClick={onClose} disabled={busy}>
          Cancel
        </Button>
        <button
          onClick={onConfirm}
          disabled={busy}
          className="inline-flex items-center justify-center font-medium gap-2 rounded-lg transition px-4 py-3 text-sm bg-error-500 text-white hover:bg-error-600 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {busy ? "Working..." : confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
