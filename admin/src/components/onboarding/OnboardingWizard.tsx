"use client";

import Field from "@/components/form/Field";
import AccessibleDialog from "@/components/ui/dialog/AccessibleDialog";
import Button from "@/components/ui/button/Button";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useApplication } from "@/context/ApplicationContext";
import { api, type OnboardingProgress } from "@/lib/api";
import { reportClientError } from "@/lib/clientError";
import { defaultScopesForCredentialType } from "@/lib/credentialScopes";
import type { Organization } from "@/lib/types";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";

import { deriveOnboardingStep, type OnboardingStep } from "./onboardingState";

const inputClass = "h-11 w-full rounded-lg border border-gray-300 bg-white px-3.5 text-sm text-gray-800 shadow-theme-xs outline-none transition focus:border-brand-500 focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90 dark:[color-scheme:dark]";

const stepOrder: { id: OnboardingStep; label: string }[] = [
  { id: "application", label: "Application" },
  { id: "credential", label: "Environment & key" },
  { id: "catalog", label: "Product & plan" },
  { id: "license", label: "Test license" },
  { id: "complete", label: "Ready" },
];

type RevealedSecret = { kind: "credential" | "license"; value: string };

function completedStep(current: OnboardingStep, candidate: OnboardingStep) {
  return stepOrder.findIndex((step) => step.id === candidate) < stepOrder.findIndex((step) => step.id === current);
}

export default function OnboardingWizard() {
  const { hasPermission } = useAdminIdentity();
  const { applications, selectedApplicationID, selectApplication, refresh: refreshApplications } = useApplication();
  const [progress, setProgress] = useState<OnboardingProgress | null>(null);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [environment, setEnvironment] = useState<"test" | "live">("test");
  const [credentialName, setCredentialName] = useState("Application SDK");
  const [productName, setProductName] = useState("");
  const [productSlug, setProductSlug] = useState("");
  const [planName, setPlanName] = useState("Test plan");
  const [planCode, setPlanCode] = useState("test");
  const [licenseEmail, setLicenseEmail] = useState("");
  const [revealedSecret, setRevealedSecret] = useState<RevealedSecret | null>(null);
  const [copied, setCopied] = useState(false);
  const [organizationDialogOpen, setOrganizationDialogOpen] = useState(false);
  const [applicationDialogOpen, setApplicationDialogOpen] = useState(false);
  const [organizationName, setOrganizationName] = useState("");
  const [applicationName, setApplicationName] = useState("");
  const [applicationSlug, setApplicationSlug] = useState("");
  const [organizationID, setOrganizationID] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextProgress, organizationResponse] = await Promise.all([
        api.onboardingProgress(),
        api.organizations(),
      ]);
      setProgress(nextProgress);
      setOrganizations(organizationResponse.items);
      setOrganizationID((current) => current || organizationResponse.items[0]?.id || "");
    } catch (loadError) {
      setError(reportClientError(loadError, "Unable to load onboarding progress. Try again."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const step = useMemo(() => progress ? deriveOnboardingStep(progress) : "application", [progress]);

  async function runAction(action: () => Promise<void>, safeMessage: string) {
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (actionError) {
      setError(reportClientError(actionError, safeMessage));
    } finally {
      setBusy(false);
    }
  }

  function reveal(kind: RevealedSecret["kind"], value: string) {
    setCopied(false);
    setRevealedSecret({ kind, value });
  }

  async function createCredential(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await runAction(async () => {
      const response = await api.createCredential({
        name: credentialName.trim(),
        environment,
        type: "publishable",
        scopes: defaultScopesForCredentialType("publishable"),
      });
      reveal("credential", response.key);
      await load();
    }, "Unable to create the credential. Try again.");
  }

  async function createCatalog(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!progress) return;
    await runAction(async () => {
      let productID = progress.product?.id;
      if (!productID) {
        const productResponse = await api.createProduct(productName.trim(), productSlug.trim());
        productID = productResponse.product.id;
      }
      await api.createPlan(productID, {
        name: planName.trim(),
        code: planCode.trim(),
        level: 1,
        max_devices: 1,
      });
      await load();
    }, "Unable to create the product and plan. Try again.");
  }

  async function createTestLicense(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!progress?.product || !progress.plan) return;
    await runAction(async () => {
      const response = await api.createLicense(
        licenseEmail.trim(),
        { value: 7, unit: "days" },
        1,
        { productId: progress.product!.id, planId: progress.plan!.id }
      );
      reveal("license", response.key);
      await load();
    }, "Unable to issue the test license. Verify the user email and try again.");
  }

  async function createOrganization(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await runAction(async () => {
      const response = await api.createOrganization(organizationName.trim());
      const nextOrganizations = [...organizations, response.organization];
      setOrganizations(nextOrganizations);
      setOrganizationID(response.organization.id);
      setOrganizationName("");
      setOrganizationDialogOpen(false);
      setApplicationDialogOpen(true);
    }, "Unable to create the organization. Try again.");
  }

  async function createApplication(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await runAction(async () => {
      const response = await api.createApplication({
        organization_id: organizationID,
        name: applicationName.trim(),
        slug: applicationSlug.trim(),
      });
      await refreshApplications();
      setApplicationDialogOpen(false);
      selectApplication(response.application.id);
    }, "Unable to create the application. Try again.");
  }

  async function copySecret() {
    if (!revealedSecret) return;
    try {
      await navigator.clipboard.writeText(revealedSecret.value);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  const canCreateApplication = hasPermission("applications.write");
  const canCreateCredential = hasPermission("credentials.write");
  const canCreateCatalog = hasPermission("catalog.write");
  const canCreateLicense = hasPermission("licenses.write");

  return (
    <div className="space-y-6">
      <section className="overflow-hidden rounded-3xl border border-gray-200 bg-white shadow-theme-sm dark:border-gray-800 dark:bg-white/[0.03]">
        <div className="border-b border-gray-200 bg-linear-to-r from-gray-950 to-gray-800 px-6 py-7 text-white dark:border-gray-800 dark:from-black dark:to-gray-900">
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-brand-300">Guided setup</p>
          <h1 className="mt-2 text-2xl font-semibold">Launch a verified application</h1>
          <p className="mt-2 max-w-2xl text-sm text-gray-300">Every completed step is read from application resources on the server, so returning to this page resumes from durable state.</p>
        </div>

        <ol className="grid border-b border-gray-200 bg-gray-50 sm:grid-cols-5 dark:border-gray-800 dark:bg-white/[0.02]" aria-label="Onboarding progress">
          {stepOrder.map((item, index) => {
            const isCurrent = item.id === step;
            const isComplete = completedStep(step, item.id) || step === "complete";
            return (
              <li key={item.id} className={`border-b border-gray-200 px-4 py-4 last:border-0 sm:border-b-0 sm:border-r dark:border-gray-800 ${isCurrent ? "bg-brand-50 dark:bg-brand-500/10" : ""}`} aria-current={isCurrent ? "step" : undefined}>
                <span className={`flex h-7 w-7 items-center justify-center rounded-full text-xs font-semibold ${isComplete ? "bg-success-500 text-white" : isCurrent ? "bg-brand-500 text-white" : "bg-gray-200 text-gray-500 dark:bg-gray-800"}`}>{isComplete ? "✓" : index + 1}</span>
                <span className="mt-2 block text-xs font-medium text-gray-700 dark:text-gray-300">{item.label}</span>
              </li>
            );
          })}
        </ol>

        <div className="grid gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_280px]">
          <div>
            {loading ? (
              <p className="py-12 text-center text-sm text-gray-500">Loading onboarding progress…</p>
            ) : error && !progress ? (
              <div className="rounded-xl border border-error-200 bg-error-50 p-5 text-sm text-error-700" role="alert"><p>{error}</p><button type="button" className="mt-3 font-semibold underline" onClick={() => void load()}>Retry</button></div>
            ) : (
              <>
                {error && <p className="mb-4 rounded-xl border border-error-200 bg-error-50 p-4 text-sm text-error-700" role="alert">{error}</p>}
                {step === "application" && (
                  <StepPanel title="Select or create an application" description="Choose the organization and application boundary for this setup.">
                    <p className="text-sm text-gray-500">Use the application selector or create a new isolated application.</p>
                  </StepPanel>
                )}
                {step === "credential" && (
                  <StepPanel title="Create a publishable credential" description="Choose the SDK environment, then create the client-facing key used by your application.">
                    <form className="space-y-5" onSubmit={createCredential}>
                      <Field id="onboarding-environment" name="environment" label="Environment" description={progress?.application?.environment_mode === "shared" ? "This application shares its data boundary across test and live credentials." : "Test and live credentials remain distinct within this application."}>
                        <select className={inputClass} value={environment} onChange={(event) => setEnvironment(event.target.value as "test" | "live")}>
                          <option value="test">Test — recommended for setup</option>
                          <option value="live">Live — production traffic</option>
                        </select>
                      </Field>
                      <Field id="onboarding-credential-name" name="name" label="Credential name" description="A descriptive label visible to administrators; the secret is shown only once.">
                        <input className={inputClass} value={credentialName} maxLength={64} onChange={(event) => setCredentialName(event.target.value)} />
                      </Field>
                      <Button size="sm" disabled={busy || !canCreateCredential || !credentialName.trim()}>{busy ? "Creating…" : "Create credential"}</Button>
                      {!canCreateCredential && <PermissionNote permission="credentials.write" />}
                    </form>
                  </StepPanel>
                )}
                {step === "catalog" && progress && (
                  <StepPanel title="Create a product and plan" description={progress.product ? `Add the first active plan to ${progress.product.name}.` : "Define what you sell and the entitlement tier the test license will use."}>
                    <form className="space-y-5" onSubmit={createCatalog}>
                      {!progress.product && (
                        <div className="grid gap-4 sm:grid-cols-2">
                          <Field id="onboarding-product-name" name="product_name" label="Product name"><input className={inputClass} value={productName} onChange={(event) => setProductName(event.target.value)} /></Field>
                          <Field id="onboarding-product-slug" name="product_slug" label="Product slug (optional)"><input className={inputClass} value={productSlug} placeholder="desktop-client" onChange={(event) => setProductSlug(event.target.value)} /></Field>
                        </div>
                      )}
                      <div className="grid gap-4 sm:grid-cols-2">
                        <Field id="onboarding-plan-name" name="plan_name" label="Plan name"><input className={inputClass} value={planName} onChange={(event) => setPlanName(event.target.value)} /></Field>
                        <Field id="onboarding-plan-code" name="plan_code" label="Plan code"><input className={inputClass} value={planCode} onChange={(event) => setPlanCode(event.target.value)} /></Field>
                      </div>
                      <Button size="sm" disabled={busy || !canCreateCatalog || (!progress.product && !productName.trim()) || !planName.trim() || !planCode.trim()}>{busy ? "Creating…" : "Create product and plan"}</Button>
                      {!canCreateCatalog && <PermissionNote permission="catalog.write" />}
                    </form>
                  </StepPanel>
                )}
                {step === "license" && progress && (
                  <StepPanel title="Issue a test license" description={`Create a seven-day, one-device license for ${progress.product?.name ?? "your product"} on ${progress.plan?.name ?? "the active plan"}.`}>
                    <form className="space-y-5" onSubmit={createTestLicense}>
                      <Field id="onboarding-license-email" name="user_email" label="Existing test user email" description="The user must already exist in this application.">
                        <input className={inputClass} type="email" required value={licenseEmail} placeholder="tester@example.com" onChange={(event) => setLicenseEmail(event.target.value)} />
                      </Field>
                      <Button size="sm" disabled={busy || !canCreateLicense || !licenseEmail.trim() || !progress.product || !progress.plan}>{busy ? "Issuing…" : "Issue test license"}</Button>
                      {!canCreateLicense && <PermissionNote permission="licenses.write" />}
                    </form>
                  </StepPanel>
                )}
                {step === "complete" && (
                  <StepPanel title="Application setup is complete" description="A publishable credential, active product and plan, and license now exist for this application.">
                    <div className="flex flex-wrap gap-3"><Link className="rounded-lg bg-brand-500 px-4 py-3 text-sm font-medium text-white hover:bg-brand-600" href="/credentials">Manage credentials</Link><Link className="rounded-lg border border-gray-300 px-4 py-3 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300" href="/licenses">View licenses</Link></div>
                  </StepPanel>
                )}
              </>
            )}
          </div>

          <aside className="rounded-2xl border border-gray-200 bg-gray-50 p-5 dark:border-gray-800 dark:bg-white/[0.02]">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-gray-400">Application context</p>
            <label className="mt-4 block text-sm font-medium text-gray-700 dark:text-gray-300" htmlFor="onboarding-application">Current application</label>
            <select id="onboarding-application" className={`${inputClass} mt-2`} value={selectedApplicationID ?? ""} disabled={applications.length === 0} onChange={(event) => selectApplication(event.target.value)}>
              <option value="" disabled>Select application</option>
              {applications.map((application) => <option key={application.id} value={application.id}>{application.name}</option>)}
            </select>
            {progress?.application && <dl className="mt-5 space-y-3 text-sm"><ResourceCount label="Publishable credentials" value={progress.credential_count} /><ResourceCount label="Active products" value={progress.product_count} /><ResourceCount label="Active plans" value={progress.plan_count} /><ResourceCount label="Licenses" value={progress.license_count} /></dl>}
            <div className="mt-5 space-y-2">
              <Button type="button" size="sm" variant="outline" className="w-full" disabled={!canCreateApplication} onClick={() => setApplicationDialogOpen(true)}>Create application</Button>
              <button type="button" className="w-full text-center text-xs font-medium text-gray-500 hover:text-brand-500" disabled={!canCreateApplication} onClick={() => setOrganizationDialogOpen(true)}>Create organization</button>
            </div>
          </aside>
        </div>
      </section>

      <AccessibleDialog isOpen={organizationDialogOpen} onClose={() => !busy && setOrganizationDialogOpen(false)} title="Create organization" className="max-w-lg p-6">
        <h2 className="text-lg font-semibold text-gray-800 dark:text-white/90">Create organization</h2>
        <form className="mt-5 space-y-5" onSubmit={createOrganization}><Field id="onboarding-organization-name" name="name" label="Organization name"><input className={inputClass} value={organizationName} onChange={(event) => setOrganizationName(event.target.value)} /></Field><div className="flex justify-end gap-2"><Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => setOrganizationDialogOpen(false)}>Cancel</Button><Button size="sm" disabled={busy || !organizationName.trim()}>Create organization</Button></div></form>
      </AccessibleDialog>

      <AccessibleDialog isOpen={applicationDialogOpen} onClose={() => !busy && setApplicationDialogOpen(false)} title="Create application" className="max-w-lg p-6">
        <h2 className="text-lg font-semibold text-gray-800 dark:text-white/90">Create application</h2>
        <form className="mt-5 space-y-5" onSubmit={createApplication}>
          <Field id="onboarding-application-organization" name="organization_id" label="Organization"><select className={inputClass} value={organizationID} onChange={(event) => setOrganizationID(event.target.value)}><option value="">Select organization</option>{organizations.map((organization) => <option key={organization.id} value={organization.id}>{organization.name}</option>)}</select></Field>
          <Field id="onboarding-application-name" name="name" label="Application name"><input className={inputClass} value={applicationName} onChange={(event) => setApplicationName(event.target.value)} /></Field>
          <Field id="onboarding-application-slug" name="slug" label="Slug (optional)"><input className={inputClass} value={applicationSlug} placeholder="desktop-client" onChange={(event) => setApplicationSlug(event.target.value)} /></Field>
          <div className="flex justify-end gap-2"><Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => setApplicationDialogOpen(false)}>Cancel</Button><Button size="sm" disabled={busy || !organizationID || !applicationName.trim()}>Create application</Button></div>
        </form>
      </AccessibleDialog>

      <AccessibleDialog isOpen={revealedSecret !== null} onClose={() => setRevealedSecret(null)} title={revealedSecret?.kind === "license" ? "Test license created" : "Credential created"} className="max-w-xl p-6">
        {revealedSecret && <div><p className="text-xs font-semibold uppercase tracking-[0.16em] text-success-600">Created successfully</p><h2 className="mt-2 text-xl font-semibold text-gray-800 dark:text-white/90">{revealedSecret.kind === "license" ? "Save the test license" : "Save the credential now"}</h2><p className="mt-2 text-sm text-gray-500">This plaintext value is shown once and is not saved by the console.</p><div className="mt-5 flex items-start gap-3 rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-gray-800 dark:bg-white/[0.03]"><code className="min-w-0 flex-1 break-all font-mono text-sm text-gray-800 dark:text-white/90">{revealedSecret.value}</code><Button type="button" size="sm" onClick={() => void copySecret()}>{copied ? "Copied" : `Copy ${revealedSecret.kind}`}</Button></div><div className="mt-5 flex justify-end"><Button type="button" size="sm" variant="outline" onClick={() => setRevealedSecret(null)}>Done</Button></div></div>}
      </AccessibleDialog>
    </div>
  );
}

function StepPanel({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <section aria-labelledby={`step-${title.toLowerCase().replaceAll(" ", "-")}`}><p className="text-xs font-semibold uppercase tracking-[0.16em] text-brand-500">Next step</p><h2 id={`step-${title.toLowerCase().replaceAll(" ", "-")}`} className="mt-2 text-xl font-semibold text-gray-800 dark:text-white/90">{title}</h2><p className="mt-2 max-w-2xl text-sm text-gray-500 dark:text-gray-400">{description}</p><div className="mt-6 max-w-2xl">{children}</div></section>;
}

function PermissionNote({ permission }: { permission: string }) {
  return <p className="text-xs text-warning-600">The <code>{permission}</code> permission is required for this step.</p>;
}

function ResourceCount({ label, value }: { label: string; value: number }) {
  return <div className="flex items-center justify-between gap-4"><dt className="text-gray-500">{label}</dt><dd className="font-semibold text-gray-800 dark:text-white/90">{value}</dd></div>;
}
