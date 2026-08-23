"use client";
import ConsoleSection, {
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import Button from "@/components/ui/button/Button";
import Label from "@/components/form/Label";
import { inputClasses } from "@/components/form/inputStyles";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { ApiError, api } from "@/lib/api";
import { useRouter } from "next/navigation";
import React, { useState } from "react";

// Security page: TOTP enrollment is mandatory before the console unlocks,
// so this route is one of the few the backend allows for unenrolled admins.

export default function SecurityPage() {
  const router = useRouter();
  const { identity, loading, refresh } = useAdminIdentity();

  // Enrollment flow state
  const [secret, setSecret] = useState<string | null>(null);
  const [provisioningUri, setProvisioningUri] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);

  // Disable flow state
  const [disablePassword, setDisablePassword] = useState("");
  const [showDisable, setShowDisable] = useState(false);

  if (loading) {
    return (
      <div>
        <PageTitle title="Security" description="Two-factor authentication settings." />
        <ConsoleSection title="Two-Factor Authentication">
          <LoadingNote />
        </ConsoleSection>
      </div>
    );
  }

  const enrolled = Boolean(identity?.mfa_enrolled);

  async function startEnrollment() {
    setBusy(true);
    setActionError(null);
    try {
      const response = await api.mfaEnrollStart();
      setSecret(response.secret);
      setProvisioningUri(response.provisioning_uri);
      setCode("");
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Failed to start enrollment");
    } finally {
      setBusy(false);
    }
  }

  async function confirmEnrollment(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setActionError(null);
    try {
      const response = await api.mfaEnrollConfirm(code.trim());
      setRecoveryCodes(response.recovery_codes);
      await refresh();
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Verification failed");
    } finally {
      setBusy(false);
    }
  }

  async function disableMfa(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setActionError(null);
    try {
      await api.mfaDisable(disablePassword);
      // The backend revokes every session, so return to the sign-in page.
      router.push("/signin");
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Failed to disable MFA");
      setBusy(false);
    }
  }

  async function copySecret() {
    if (!secret) return;
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard may be unavailable; the secret stays visible on screen.
    }
  }

  return (
    <div>
      <PageTitle
        title="Security"
        description="Two-factor authentication protects this admin account."
      />
      <div className="space-y-6">
        {/* Recovery codes are shown once, right after enrollment. */}
        {recoveryCodes ? (
          <ConsoleSection
            title="Save your recovery codes"
            description="Each code works once if you lose access to your authenticator. Store them somewhere safe — they will not be shown again."
          >
            <div className="p-5">
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                {recoveryCodes.map((rc) => (
                  <span
                    key={rc}
                    className="rounded-lg bg-gray-100 px-3 py-2 font-mono text-sm text-gray-700 dark:bg-white/[0.05] dark:text-gray-300"
                  >
                    {rc}
                  </span>
                ))}
              </div>
              <div className="mt-5">
                <Button size="sm" onClick={() => setRecoveryCodes(null)}>
                  I have saved my codes
                </Button>
              </div>
            </div>
          </ConsoleSection>
        ) : enrolled ? (
          <ConsoleSection
            title="Two-Factor Authentication"
            description="MFA is enabled on this account."
          >
            <div className="p-5">
              <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
                Your account requires an authenticator code at every sign-in.
                Disabling MFA revokes all active sessions and signs you out.
              </p>
              {showDisable ? (
                <form onSubmit={disableMfa} className="max-w-sm space-y-4">
                  <div>
                    <Label>
                      Confirm your password{" "}
                      <span className="text-error-500">*</span>{" "}
                    </Label>
                    <input
                      className={inputClasses}
                      type="password"
                      autoComplete="current-password"
                      value={disablePassword}
                      onChange={(e) => setDisablePassword(e.target.value)}
                      required
                    />
                  </div>
                  {actionError && (
                    <p className="text-sm text-error-500" role="alert">
                      {actionError}
                    </p>
                  )}
                  <div className="flex gap-2">
                    <Button size="sm" disabled={busy}>
                      {busy ? "Disabling..." : "Disable MFA"}
                    </Button>
                    <button
                      type="button"
                      className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300"
                      onClick={() => {
                        setShowDisable(false);
                        setActionError(null);
                        setDisablePassword("");
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              ) : (
                <button
                  type="button"
                  className="rounded-lg border border-error-500/30 px-4 py-2 text-sm font-medium text-error-500 hover:bg-error-50 dark:hover:bg-error-500/[0.05]"
                  onClick={() => setShowDisable(true)}
                >
                  Disable two-factor authentication
                </button>
              )}
            </div>
          </ConsoleSection>
        ) : (
          <ConsoleSection
            title="Set up two-factor authentication"
            description="Scan the secret into an authenticator app, then verify with a code."
          >
            <div className="p-5">
              {secret ? (
                <form onSubmit={confirmEnrollment} className="max-w-lg space-y-5">
                  <div>
                    <Label>Secret key</Label>
                    <div className="flex items-center gap-2">
                      <code className="flex-1 break-all rounded-lg bg-gray-100 px-3 py-2 font-mono text-sm text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
                        {secret}
                      </code>
                      <button
                        type="button"
                        className="shrink-0 rounded-lg border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300"
                        onClick={copySecret}
                      >
                        {copied ? "Copied" : "Copy"}
                      </button>
                    </div>
                    {provisioningUri && (
                      <p className="mt-2 break-all text-xs text-gray-400 dark:text-gray-500">
                        {provisioningUri}
                      </p>
                    )}
                  </div>
                  <div>
                    <Label>
                      Code from your authenticator{" "}
                      <span className="text-error-500">*</span>{" "}
                    </Label>
                    <input
                      className={inputClasses}
                      placeholder="123456"
                      type="text"
                      inputMode="numeric"
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      required
                    />
                  </div>
                  {actionError && (
                    <p className="text-sm text-error-500" role="alert">
                      {actionError}
                    </p>
                  )}
                  <div className="flex gap-2">
                    <Button size="sm" disabled={busy}>
                      {busy ? "Verifying..." : "Verify and enable"}
                    </Button>
                    <button
                      type="button"
                      className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300"
                      onClick={() => {
                        setSecret(null);
                        setProvisioningUri(null);
                        setActionError(null);
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              ) : (
                <div>
                  <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
                    You will need an authenticator app (Google Authenticator,
                    1Password, Aegis, etc.). Access to the console stays locked
                    until MFA is enabled.
                  </p>
                  {actionError && (
                    <p className="mb-3 text-sm text-error-500" role="alert">
                      {actionError}
                    </p>
                  )}
                  <Button size="sm" disabled={busy} onClick={startEnrollment}>
                    {busy ? "Preparing..." : "Start setup"}
                  </Button>
                </div>
              )}
            </div>
          </ConsoleSection>
        )}
      </div>
    </div>
  );
}
