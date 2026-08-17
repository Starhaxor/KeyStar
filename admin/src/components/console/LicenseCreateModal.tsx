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
  const [value, setValue] = useState(30);
  const [unit, setUnit] = useState("days");
  const [maxDevices, setMaxDevices] = useState(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<CreatedLicense | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (open) {
      setEmail(defaultEmail);
      setValue(30);
      setUnit("days");
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
      const response = await api.createLicense(email.trim(), { value, unit }, maxDevices);
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
              <Label>Duration</Label>
              <div className="flex gap-2">
                <input
                  className={fieldClasses}
                  type="number"
                  min={1}
                  max={1000}
                  required
                  value={value}
                  onChange={(e) =>
                    setValue(Math.max(1, Number(e.target.value) || 1))
                  }
                />
                <select
                  className={`${fieldClasses} w-32 px-2`}
                  value={unit}
                  onChange={(e) => setUnit(e.target.value)}
                >
                  <option value="hours">Hours</option>
                  <option value="days">Days</option>
                  <option value="weeks">Weeks</option>
                  <option value="months">Months</option>
                  <option value="years">Years</option>
                </select>
              </div>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {[
                  { label: "1 h", value: 1, unit: "hours" },
                  { label: "12 h", value: 12, unit: "hours" },
                  { label: "7 d", value: 7, unit: "days" },
                  { label: "30 d", value: 30, unit: "days" },
                  { label: "1 mo", value: 1, unit: "months" },
                  { label: "1 y", value: 1, unit: "years" },
                ].map((preset) => {
                  const active = value === preset.value && unit === preset.unit;
                  return (
                    <button
                      key={preset.label}
                      type="button"
                      onClick={() => {
                        setValue(preset.value);
                        setUnit(preset.unit);
                      }}
                      className={`rounded-full border px-2.5 py-1 text-xs font-medium transition-colors ${
                        active
                          ? "border-brand-500 bg-brand-50 text-brand-700 dark:border-brand-500/60 dark:bg-brand-500/10 dark:text-brand-400"
                          : "border-gray-300 text-gray-500 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-400 dark:hover:bg-gray-800"
                      }`}
                    >
                      {preset.label}
                    </button>
                  );
                })}
              </div>
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
