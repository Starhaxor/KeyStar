type OrganizationChoice = { id: string; name?: string };
type ApplicationChoice = { id: string; name: string; environment_mode: string };
type ApplicationIDChoice = { id: string; status?: string };

export function initialOrganizationSelection(organizations: OrganizationChoice[]): string {
  return organizations.length === 1 ? organizations[0].id : "";
}

export function applicationSelectorOptions(applications: ApplicationChoice[]) {
  return applications.map((application) => ({
    value: application.id,
    label: `${application.name} · ${application.environment_mode.slice(0, 1).toUpperCase()}${application.environment_mode.slice(1)}`,
  }));
}

export function nextSelectedApplicationID(currentID: string | null, applications: ApplicationIDChoice[]): string | null {
  const activeApplications = applications.filter((application) => application.status === undefined || application.status === "active");
  if (currentID && activeApplications.some((application) => application.id === currentID)) return currentID;
  return activeApplications[0]?.id ?? null;
}
