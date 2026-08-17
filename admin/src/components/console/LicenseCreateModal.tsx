"use client";
import React, { useEffect, useState } from "react";
import Label from "@/components/form/Label";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { useToast } from "@/context/ToastContext";
import { api } from "@/lib/api";
import type { CreatedLicense } from "@/lib/types";

const fieldClasses =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

export default function LicenseCreateModal({
  open,
  defaultEmail = "",
  onClose,
  onCreated,
}: {
  open: boolean;
  defaultEmail?: string;
  onClose: () => void;
  onCreated?: () => Promise<void> | void;
}) {
  const toast = useToast();
  const [email, setEmail] = useState(defaultEmail);
  const [days, setDays] = useState(30);
  const [maxDevices, setMaxDevices] = useState(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<CreatedLicense | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (open) {
      setEmail(defaultEmail);
      setDays(30);
      setMaxDevices(1);
      setError(null);
      setCreated(null);
      setCopied(false);
    }
  }, [open, defaultEmail]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const response = await api.createLicense(
        email.trim(),
        days,
        maxDevices
      );
      setCreated(response);
      setCopied(false);
      await onCreated?.();
      toast.success("License created", email.trim());
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "License creation failed";
      setError(message);
      toast.error("License creation failed", message);
    } finally {
      setBusy(false);
    }
  }

  async function copyKey() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.key);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  function handleClose() {
    if (busy) return;
    setCreated(null);
    onClose();
  }

  return (
    <Modal
      isOpen={open || created !== null}
      onClose={handleClose}
      className="max-w-md p-6"
    >
      {created ? (
        <div>
          <h3 className="mb-2 text-lg font-semibold text-gray-800 dark:text-white/90">
            License Created
          </h3>
          <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
            This is the only time the plaintext key is shown. Copy it now —
            only its HMAC is stored.
          </p>
          <div className="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-gray-800 dark:bg-white/[0.03]">
            <code className="flex-1 break-all font-mono text-sm text-gray-800 dark:text-white/90">
              {created.key}
            </code>
            <button
              onClick={copyKey}
              className="shrink-0 rounded-lg bg-brand-500 px-3 py-1.5 text-xs font-medium text-white hover:bg-brand-600"
            >
              {copied ? "Copied" : "Copy"}
            </button>
          </div>
          <div className="mt-4 flex justify-end">
            <Button size="sm" onClick={handleClose}>
              Done
            </Button>
          </div>
        </div>
      ) : (
        <form onSubmit={handleCreate} className="space-y-4">
          <div>
            <h3 className="mb-1 text-lg font-semibold text-gray-800 dark:text-white/90">
              Create License
            </h3>
            <p className="text-sm text-gray-500 dark:text-gray-400">
              Issue a product license to an end user.
            </p>
          </div>
          <div>
            <Label>User Email</Label>
            <input
              className={fieldClasses}
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="user@example.com"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label>Duration (days)</Label>
              <input
                className={fieldClasses}
                type="number"
                min={1}
                max={3650}
                required
                value={days}
                onChange={(e) => setDays(Number(e.target.value))}
              />
            </div>
            <div>
              <Label>Max Devices</Label>
              <input
                className={fieldClasses}
                type="number"
                min={1}
                max={10000}
                required
                value={maxDevices}
                onChange={(e) => setMaxDevices(Number(e.target.value))}
              />
            </div>
          </div>
          {error && (
            <p className="text-sm text-error-500" role="alert">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              disabled={busy}
              onClick={handleClose}
              className="inline-flex items-center justify-center font-medium gap-2 rounded-lg transition px-4 py-3 text-sm bg-white text-gray-700 ring-1 ring-inset ring-gray-300 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-400 dark:ring-gray-700 disabled:opacity-50"
            >
              Cancel
            </button>
            <Button size="sm" disabled={busy}>
              {busy ? "Creating..." : "Create"}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
