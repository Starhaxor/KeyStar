"use client";
import { useState } from "react";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import Label from "@/components/form/Label";

interface PromoteAdminModalProps {
  open: boolean;
  userEmail: string;
  onClose: () => void;
  // Returns the temporary password that was generated (displayed exactly once).
  onPromote: (role: string) => Promise<{ tempPassword: string }>;
}

const fieldClasses =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

export default function PromoteAdminModal({
  open,
  userEmail,
  onClose,
  onPromote,
}: PromoteAdminModalProps) {
  const [role, setRole] = useState("viewer");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tempPassword, setTempPassword] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const response = await onPromote(role);
      setTempPassword(response.tempPassword);
      setCopied(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Promote failed");
    } finally {
      setBusy(false);
    }
  }

  function handleClose() {
    if (busy) return;
    setRole("viewer");
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
      {tempPassword ? (
        <div className="space-y-5">
          <div>
            <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
              Admin account created
            </h3>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Share this temporary password with{" "}
              <span className="font-medium text-gray-700 dark:text-gray-300">
                {userEmail}
              </span>{" "}
              over a trusted channel — it will not be shown again. The new admin
              must enroll MFA on their first login.
            </p>
          </div>
          <div className="rounded-xl border border-success-200 bg-success-50 p-4 dark:border-success-500/30 dark:bg-success-500/10">
            <div className="flex items-center gap-2">
              <code className="flex-1 break-all rounded-lg bg-white px-3 py-2 font-mono text-sm text-gray-800 dark:bg-gray-900 dark:text-white/90">
                {tempPassword}
              </code>
              <Button size="sm" variant={copied ? "success" : "primary"} onClick={copyTemp}>
                {copied ? "Copied" : "Copy"}
              </Button>
            </div>
          </div>
          <div className="flex justify-end">
            <Button size="sm" onClick={handleClose}>
              Done
            </Button>
          </div>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div>
            <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
              Promote to admin
            </h3>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Grants {userEmail} access to the admin console. A strong temporary
              password is generated — the user&apos;s existing password is never
              reused for console access.
            </p>
          </div>
          <div>
            <Label>Email</Label>
            <input
              type="email"
              value={userEmail}
              readOnly
              disabled
              className={fieldClasses}
            />
          </div>
          <div>
            <Label>Role</Label>
            <select
              className={fieldClasses}
              value={role}
              onChange={(e) => setRole(e.target.value)}
            >
              <option value="viewer">viewer</option>
              <option value="owner">owner</option>
            </select>
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
            <Button variant="info" size="sm" disabled={busy}>
              {busy ? "Creating..." : "Create admin"}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
