"use client";
import { useState } from "react";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import Label from "@/components/form/Label";

interface ResetPasswordModalProps {
  open: boolean;
  userEmail: string;
  onClose: () => void;
  // Returns the temporary password to display (may be null when the admin
  // supplied their own password and nothing needs showing).
  onReset: (password: string) => Promise<{ tempPassword: string | null }>;
}

const fieldClasses =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

export default function ResetPasswordModal({
  open,
  userEmail,
  onClose,
  onReset,
}: ResetPasswordModalProps) {
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tempPassword, setTempPassword] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const response = await onReset(password);
      if (response.tempPassword) {
        setTempPassword(response.tempPassword);
        setCopied(false);
        setPassword("");
      } else {
        setTempPassword(null);
        onClose();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Reset failed");
    } finally {
      setBusy(false);
    }
  }

  function handleClose() {
    if (busy) return;
    setPassword("");
    setError(null);
    setTempPassword(null);
    setCopied(false);
    onClose();
  }

  async function copyTemp() {
    if (!tempPassword) return;
    try {
      await navigator.clipboard.writeText(tempPassword);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  }

  return (
    <Modal isOpen={open} onClose={handleClose} className="max-w-md p-6">
      <form onSubmit={handleSubmit} className="space-y-5">
        <div>
          <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
            Reset password
          </h3>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Sets a new password for <span className="font-medium text-gray-700 dark:text-gray-300">{userEmail}</span>.
            Leave the field empty to generate a strong temporary password, or
            type your own.
          </p>
        </div>

        {tempPassword ? (
          <div className="rounded-xl border border-success-200 bg-success-50 p-4 dark:border-success-500/30 dark:bg-success-500/10">
            <p className="text-sm font-medium text-success-700 dark:text-success-400">
              Password updated — copy it now, it will not be shown again.
            </p>
            <div className="mt-3 flex items-center gap-2">
              <code className="flex-1 break-all rounded-lg bg-white px-3 py-2 font-mono text-sm text-gray-800 dark:bg-gray-900 dark:text-white/90">
                {tempPassword}
              </code>
              <Button size="sm" variant={copied ? "success" : "primary"} onClick={copyTemp}>
                {copied ? "Copied" : "Copy"}
              </Button>
            </div>
          </div>
        ) : (
          <>
            <div>
              <Label>New password (optional)</Label>
              <input
                type="text"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Leave empty to auto-generate"
                className={fieldClasses}
              />
            </div>
            {error && (
              <p className="text-sm text-error-500" role="alert">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <button
                type="button"
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300"
                onClick={handleClose}
              >
                Cancel
              </button>
              <Button variant="warning" size="sm" disabled={busy}>
                {busy ? "Resetting..." : "Reset password"}
              </Button>
            </div>
          </>
        )}
      </form>
    </Modal>
  );
}
