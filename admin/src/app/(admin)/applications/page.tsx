"use client";

import { EmptyNote, ErrorNote, LoadingNote, PageTitle } from "@/components/console/ConsoleSection";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useApplication } from "@/context/ApplicationContext";
import { api } from "@/lib/api";
import { initialOrganizationSelection } from "@/lib/applicationSelection";
import type { Application, Organization } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-white px-3 text-sm text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90 dark:[color-scheme:dark]";

export default function ApplicationsPage() {
  const { hasPermission } = useAdminIdentity();
  const { refresh: refreshApplications } = useApplication();
  const [applications, setApplications] = useState<Application[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [orgOpen, setOrgOpen] = useState(false); const [appOpen, setAppOpen] = useState(false); const [busy, setBusy] = useState(false);
  const [orgName, setOrgName] = useState(""); const [name, setName] = useState(""); const [slug, setSlug] = useState(""); const [organizationID, setOrganizationID] = useState("");
  const canWrite = hasPermission("applications.write");
  const load = useCallback(async () => { setLoading(true); setError(null); try { const [appResponse, orgResponse] = await Promise.all([api.applications(), api.organizations()]); setApplications(appResponse.items); setOrganizations(orgResponse.items); } catch (err) { setError(err instanceof Error ? err.message : "Applications could not be loaded"); } finally { setLoading(false); } }, []);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => { if (!organizationID) setOrganizationID(initialOrganizationSelection(organizations)); }, [organizationID, organizations]);
  async function createOrg() { setBusy(true); try { await api.createOrganization(orgName.trim()); setOrgName(""); setOrgOpen(false); await load(); } catch (err) { setError(err instanceof Error ? err.message : "Organization could not be created"); } finally { setBusy(false); } }
  async function createApp() { setBusy(true); try { await api.createApplication({ organization_id: organizationID, name: name.trim(), slug: slug.trim() }); setName(""); setSlug(""); setOrganizationID(""); setAppOpen(false); await Promise.all([load(), refreshApplications()]); } catch (err) { setError(err instanceof Error ? err.message : "Application could not be created"); } finally { setBusy(false); } }
  return <><PageTitle title="Applications" description="Manage organizations and their isolated KeyStar applications." actions={canWrite ? <div className="flex gap-2"><Button size="sm" variant="outline" onClick={() => setOrgOpen(true)}>Add organization</Button><Button size="sm" onClick={() => setAppOpen(true)}>Add application</Button></div> : undefined} />{error && <div className="mb-4"><ErrorNote message={error} /></div>}<div className="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">{loading ? <LoadingNote /> : applications.length === 0 ? <EmptyNote message="No applications have been created." /> : <table className="min-w-full text-left text-sm"><thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800"><tr><th className="px-5 py-3">Application</th><th>Organization</th><th>Status</th></tr></thead><tbody>{applications.map((app) => <tr key={app.id} className="border-b border-gray-100 dark:border-gray-800"><td className="px-5 py-4"><strong>{app.name}</strong><div className="text-xs text-gray-500">{app.slug}</div></td><td>{organizations.find((org) => org.id === app.organization_id)?.name ?? app.organization_id}</td><td>{app.status}</td></tr>)}</tbody></table>}</div><Modal isOpen={orgOpen} onClose={() => !busy && setOrgOpen(false)} className="max-w-md p-6"><h2 className="text-lg font-semibold">Create organization</h2><div className="mt-4 space-y-4"><input className={inputClass} placeholder="Organization name" value={orgName} onChange={(event) => setOrgName(event.target.value)} /><div className="flex justify-end gap-2"><Button size="sm" variant="outline" onClick={() => setOrgOpen(false)}>Cancel</Button><Button size="sm" disabled={busy || !orgName.trim()} onClick={createOrg}>Create</Button></div></div></Modal><Modal isOpen={appOpen} onClose={() => !busy && setAppOpen(false)} className="max-w-md p-6"><h2 className="text-lg font-semibold">Create application</h2><div className="mt-4 space-y-4"><select className={inputClass} value={organizationID} onChange={(event) => setOrganizationID(event.target.value)}><option value="">Select organization</option>{organizations.map((org) => <option key={org.id} value={org.id}>{org.name}</option>)}</select><input className={inputClass} placeholder="Application name" value={name} onChange={(event) => setName(event.target.value)} /><input className={inputClass} placeholder="Slug (optional)" value={slug} onChange={(event) => setSlug(event.target.value)} /><div className="flex justify-end gap-2"><Button size="sm" variant="outline" onClick={() => setAppOpen(false)}>Cancel</Button><Button size="sm" disabled={busy || !organizationID || !name.trim()} onClick={createApp}>Create</Button></div></div></Modal></>;
}
