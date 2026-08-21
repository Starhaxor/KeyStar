type ApplicationNavigationTarget = {
  name: string;
  environment_mode: string;
};

export function applicationNavigationLabel(application: ApplicationNavigationTarget | null): string {
  if (!application) return "No application selected";

  const environment = application.environment_mode
    ? `${application.environment_mode.slice(0, 1).toUpperCase()}${application.environment_mode.slice(1)}`
    : "Application";

  return `${application.name} · ${environment}`;
}
