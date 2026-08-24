"use client";

import { ErrorNote, PageTitle } from "@/components/console/ConsoleSection";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useApplication } from "@/context/ApplicationContext";
import { api } from "@/lib/api";
import { initialOrganizationSelection } from "@/lib/applicationSelection";
import { useEffect, useState } from "react";
import { ApplicationForm, OrganizationForm } from "./ApplicationsForms";
import ApplicationsView from "./ApplicationsView";
import useApplicationsData from "./useApplicationsData";

export default function ApplicationsPage() {
  const { hasPermission } = useAdminIdentity();
  const { refresh: refreshApplications } = useApplication();
  const { applications, organizations, loading, error, setError, load } = useApplicationsData();
  const [organizationOpen, setOrganizationOpen] = useState(false);
  const [applicationOpen, setApplicationOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [organizationName, setOrganizationName] = useState("");
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [organizationID, setOrganizationID] = useState("");
  const canWrite = hasPermission("applications.write");

  useEffect(() => {
    if (!organizationID) setOrganizationID(initialOrganizationSelection(organizations));
  }, [organizationID, organizations]);

  async function createOrganization() {
    setBusy(true);
    try {
      await api.createOrganization(organizationName.trim());
      setOrganizationName("");
      setOrganizationOpen(false);
      await load();
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "Organization could not be created");
    } finally {
      setBusy(false);
    }
  }

  async function createApplication() {
    setBusy(true);
    try {
      await api.createApplication({ organization_id: organizationID, name: name.trim(), slug: slug.trim() });
      setName("");
      setSlug("");
      setOrganizationID("");
      setApplicationOpen(false);
      await Promise.all([load(), refreshApplications()]);
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "Application could not be created");
    } finally {
      setBusy(false);
    }
  }

  return <>
    <PageTitle title="Applications" description="Manage organizations and their isolated KeyStar applications." actions={canWrite ? <div className="flex gap-2"><Button size="sm" variant="outline" onClick={() => setOrganizationOpen(true)}>Add organization</Button><Button size="sm" onClick={() => setApplicationOpen(true)}>Add application</Button></div> : undefined} />
    {error && <div className="mb-4"><ErrorNote message={error} /></div>}
    <ApplicationsView applications={applications} organizations={organizations} loading={loading} />
    <Modal isOpen={organizationOpen} onClose={() => !busy && setOrganizationOpen(false)} className="max-w-md p-6"><h2 className="text-lg font-semibold">Create organization</h2><OrganizationForm name={organizationName} busy={busy} onNameChange={setOrganizationName} onSubmit={createOrganization} onCancel={() => setOrganizationOpen(false)} /></Modal>
    <Modal isOpen={applicationOpen} onClose={() => !busy && setApplicationOpen(false)} className="max-w-md p-6"><h2 className="text-lg font-semibold">Create application</h2><ApplicationForm organizations={organizations} organizationID={organizationID} name={name} slug={slug} busy={busy} onOrganizationChange={setOrganizationID} onNameChange={setName} onSlugChange={setSlug} onSubmit={createApplication} onCancel={() => setApplicationOpen(false)} /></Modal>
  </>;
}
