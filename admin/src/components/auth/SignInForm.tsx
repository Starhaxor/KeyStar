"use client";
import Field from "@/components/form/Field";
import { inputClasses } from "@/components/form/inputStyles";
import Button from "@/components/ui/button/Button";
import { ApiError, api } from "@/lib/api";
import { EyeCloseIcon, EyeIcon } from "@/icons";
import { useRouter } from "next/navigation";
import React, { useEffect, useState } from "react";

function errorText(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 429) {
      return "Too many attempts. Please wait a moment and try again.";
    }
    return err.message || "Request failed.";
  }
  return "Backend is unreachable. Please try again.";
}

export default function SignInForm() {
  const router = useRouter();
  const [mode, setMode] = useState<"loading" | "signin" | "bootstrap">("loading");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [passwordConfirmation, setPasswordConfirmation] = useState("");
  const [bootstrapToken, setBootstrapToken] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Two-step login: after a correct password, enrolled accounts receive a
  // one-time MFA challenge that must be completed with a TOTP or recovery
  // code before the session cookie is issued.
  const [step, setStep] = useState<"password" | "mfa">("password");
  const [mfaToken, setMfaToken] = useState("");
  const [mfaCode, setMfaCode] = useState("");
  const [useRecoveryCode, setUseRecoveryCode] = useState(false);

  useEffect(() => {
    let active = true;
    api.bootstrapStatus()
      .then((response) => {
        if (active) setMode(response.setup_required ? "bootstrap" : "signin");
      })
      .catch((err) => {
        if (active) {
          setMode("signin");
          setError(errorText(err));
        }
      });
    return () => {
      active = false;
    };
  }, []);

  async function handleBootstrap(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting) return;
    if (password !== passwordConfirmation) {
      setError("Passwords do not match.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const normalizedEmail = email.trim();
      const response = await api.bootstrapRoot(normalizedEmail, password, bootstrapToken);
      if (response.session_created) {
        router.push("/security");
        router.refresh();
      } else {
        setMode("signin");
        setError("Root account was created. Sign in to continue MFA setup.");
      }
    } catch (err) {
      try {
        const status = await api.bootstrapStatus();
        if (!status.setup_required) {
          setMode("signin");
          setError("Root setup completed. Sign in to continue MFA setup.");
          return;
        }
      } catch {
        // Preserve the original bootstrap error when status is also unavailable.
      }
      setError(errorText(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const response = await api.login(email.trim(), password);
      if (response.mfa_required) {
        setMfaToken(response.mfa_token);
        setMfaCode("");
        setUseRecoveryCode(false);
        setStep("mfa");
      } else {
        router.push("/");
        router.refresh();
      }
    } catch (err) {
      setError(errorText(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleMfaSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const input = useRecoveryCode
        ? { recovery_code: mfaCode.trim() }
        : { code: mfaCode.trim() };
      await api.completeMfa(mfaToken, input);
      router.push("/");
      router.refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setSubmitting(false);
    }
  }

  if (mode === "loading") {
    return (
      <div className="flex flex-1 items-center justify-center lg:w-1/2">
        <div className="flex items-center gap-3 text-sm text-gray-500 dark:text-gray-400" role="status">
          <span className="h-2.5 w-2.5 animate-pulse rounded-full bg-brand-500" />
          Checking installation…
        </div>
      </div>
    );
  }

  if (mode === "bootstrap") {
    return (
      <div className="flex w-full flex-1 flex-col lg:w-1/2">
        <div className="flex w-full max-w-md flex-1 flex-col justify-center mx-auto">
          <div className="mb-8">
            <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-brand-500/20 bg-brand-50 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-brand-600 dark:bg-brand-500/10 dark:text-brand-300">
              <span className="h-1.5 w-1.5 rounded-full bg-brand-500" />
              First-run security setup
            </div>
            <h1 className="mb-2 font-semibold text-gray-800 text-title-sm dark:text-white/90 sm:text-title-md">
              Create root administrator
            </h1>
            <p className="text-sm leading-6 text-gray-500 dark:text-gray-400">
              This one-time owner account controls the KeyStar installation.
              MFA setup is required before the console unlocks.
            </p>
          </div>

          <div className="mb-7 grid grid-cols-3 gap-2" aria-label="Setup progress">
            {["Root account", "MFA", "Console"].map((label, index) => (
              <div key={label} className="min-w-0">
                <div className={`mb-2 h-1 rounded-full ${index === 0 ? "bg-brand-500" : "bg-gray-200 dark:bg-gray-800"}`} />
                <span className={`text-xs ${index === 0 ? "font-medium text-brand-600 dark:text-brand-300" : "text-gray-400"}`}>
                  {label}
                </span>
              </div>
            ))}
          </div>

          <form onSubmit={handleBootstrap}>
            <div className="space-y-5">
              <Field id="bootstrap-token" name="bootstrap-token" label={<>Bootstrap token <span className="text-error-500">*</span></>}>
                <input
                  className={inputClasses}
                  type="password"
                  autoComplete="off"
                  spellCheck={false}
                  value={bootstrapToken}
                  onChange={(event) => setBootstrapToken(event.target.value)}
                  required
                />
                <p className="mt-1.5 text-xs text-gray-400">Use the ADMIN_BOOTSTRAP_TOKEN configured on the server.</p>
              </Field>
              <Field id="root-email" name="email" label={<>Root email <span className="text-error-500">*</span></>}>
                <input
                  className={inputClasses}
                  placeholder="root@example.com"
                  type="email"
                  autoComplete="email"
                  spellCheck={false}
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  required
                  autoFocus
                />
              </Field>
              <Field id="root-password" name="password" label={<>Password <span className="text-error-500">*</span></>}>
                <div className="relative">
                  <input
                    className={inputClasses}
                    type={showPassword ? "text" : "password"}
                    minLength={12}
                    autoComplete="new-password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    required
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword((visible) => !visible)}
                    aria-label={showPassword ? "Hide password" : "Show password"}
                    className="absolute z-30 -translate-y-1/2 right-4 top-1/2 rounded focus:outline-none focus:ring-2 focus:ring-brand-500/40"
                  >
                    {showPassword ? <EyeIcon aria-hidden="true" className="fill-gray-500 dark:fill-gray-400" /> : <EyeCloseIcon aria-hidden="true" className="fill-gray-500 dark:fill-gray-400" />}
                  </button>
                </div>
                <p className="mt-1.5 text-xs text-gray-400">Use at least 12 characters.</p>
              </Field>
              <Field id="root-password-confirmation" name="password-confirmation" label={<>Confirm password <span className="text-error-500">*</span></>}>
                <input
                  className={inputClasses}
                  type={showPassword ? "text" : "password"}
                  minLength={12}
                  autoComplete="new-password"
                  value={passwordConfirmation}
                  onChange={(event) => setPasswordConfirmation(event.target.value)}
                  required
                />
              </Field>
              {error ? <p className="text-sm text-error-500" role="alert">{error}</p> : null}
              <Button className="w-full" size="sm" disabled={submitting}>
                {submitting ? "Creating secure root…" : "Create root and set up MFA"}
              </Button>
            </div>
          </form>
          <p className="mt-6 text-center text-xs leading-5 text-gray-400">
            Registration closes permanently after this account is created.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col flex-1 lg:w-1/2 w-full">
      <div className="flex flex-col justify-center flex-1 w-full max-w-md mx-auto">
        {step === "password" ? (
          <div>
            <div className="mb-5 sm:mb-8">
              <h1 className="mb-2 font-semibold text-gray-800 text-title-sm dark:text-white/90 sm:text-title-md">
                Sign In
              </h1>
              <p className="text-sm text-gray-500 dark:text-gray-400">
                Enter your admin email and password to access the console.
              </p>
            </div>
            <form onSubmit={handleSubmit}>
              <div className="space-y-6">
                <Field
                  id="sign-in-email"
                  name="email"
                  label={
                    <>
                    Email <span className="text-error-500">*</span>{" "}
                    </>
                  }
                >
                  <input
                    className={inputClasses}
                    placeholder="admin@example.com"
                    type="email"
                    autoComplete="email"
                    spellCheck={false}
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    required
                  />
                </Field>
                <Field
                  id="sign-in-password"
                  name="password"
                  label={
                    <>
                    Password <span className="text-error-500">*</span>{" "}
                    </>
                  }
                >
                  <div className="relative">
                    <input
                      className={inputClasses}
                      type={showPassword ? "text" : "password"}
                      placeholder="Enter your password"
                      autoComplete="current-password"
                      spellCheck={false}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      required
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      aria-label={showPassword ? "Hide password" : "Show password"}
                      className="absolute z-30 -translate-y-1/2 right-4 top-1/2 rounded focus:outline-none focus:ring-2 focus:ring-brand-500/40"
                    >
                      {showPassword ? (
                        <EyeIcon aria-hidden="true" className="fill-gray-500 dark:fill-gray-400" />
                      ) : (
                        <EyeCloseIcon aria-hidden="true" className="fill-gray-500 dark:fill-gray-400" />
                      )}
                    </button>
                  </div>
                </Field>
                {error && (
                  <p className="text-sm text-error-500" role="alert">
                    {error}
                  </p>
                )}
                <div>
                  <Button className="w-full" size="sm" disabled={submitting}>
                    {submitting ? "Signing in..." : "Sign in"}
                  </Button>
                </div>
              </div>
            </form>
          </div>
        ) : (
          <div>
            <div className="mb-5 sm:mb-8">
              <h1 className="mb-2 font-semibold text-gray-800 text-title-sm dark:text-white/90 sm:text-title-md">
                Two-Factor Verification
              </h1>
              <p className="text-sm text-gray-500 dark:text-gray-400">
                {useRecoveryCode
                  ? "Enter one of your single-use recovery codes."
                  : "Enter the 6-digit code from your authenticator app."}
              </p>
            </div>
            <form onSubmit={handleMfaSubmit}>
              <div className="space-y-6">
                <Field
                  id="sign-in-mfa-code"
                  name="mfa-code"
                  label={
                    <>
                    {useRecoveryCode ? "Recovery code" : "Authentication code"}{" "}
                    <span className="text-error-500">*</span>{" "}
                    </>
                  }
                >
                  <input
                    className={inputClasses}
                    placeholder={useRecoveryCode ? "XXXX-XXXX" : "123456"}
                    type="text"
                    inputMode={useRecoveryCode ? "text" : "numeric"}
                    autoComplete="one-time-code"
                    spellCheck={false}
                    value={mfaCode}
                    onChange={(e) => setMfaCode(e.target.value)}
                    autoFocus
                    required
                  />
                </Field>
                {error && (
                  <p className="text-sm text-error-500" role="alert">
                    {error}
                  </p>
                )}
                <div>
                  <Button className="w-full" size="sm" disabled={submitting}>
                    {submitting ? "Verifying..." : "Verify"}
                  </Button>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <button
                    type="button"
                    className="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
                    onClick={() => {
                      setStep("password");
                      setError(null);
                    }}
                  >
                    Back to sign in
                  </button>
                  <button
                    type="button"
                    className="text-brand-500 hover:text-brand-600"
                    onClick={() => {
                      setUseRecoveryCode(!useRecoveryCode);
                      setMfaCode("");
                      setError(null);
                    }}
                  >
                    {useRecoveryCode
                      ? "Use authenticator code"
                      : "Use a recovery code"}
                  </button>
                </div>
              </div>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}
