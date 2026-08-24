import { api } from "@/lib/api";
import type { Application, Organization } from "@/lib/types";
import { useCallback, useEffect, useState } from "react";

export default function useApplicationsData() {
  const [applications, setApplications] = useState<Application[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(async () => { setLoading(true); setError(null); try { const [applicationResponse, organizationResponse] = await Promise.all([api.applications(), api.organizations()]); setApplications(applicationResponse.items); setOrganizations(organizationResponse.items); } catch (loadError) { setError(loadError instanceof Error ? loadError.message : "Applications could not be loaded"); } finally { setLoading(false); } }, []);
  useEffect(() => { void load(); }, [load]);
  return { applications, organizations, loading, error, setError, load };
}
