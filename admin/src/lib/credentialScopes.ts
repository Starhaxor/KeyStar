export type CredentialType = "publishable" | "secret";

export type CredentialScopeOption = {
  value: string;
  label: string;
  description: string;
};

const PUBLISHABLE_SCOPES: CredentialScopeOption[] = [
  { value: "auth.login", label: "User sign-in", description: "Sign a user in from your application." },
  { value: "auth.register", label: "User registration", description: "Create new user accounts." },
  { value: "auth.refresh", label: "Refresh session", description: "Refresh an existing user session." },
  { value: "auth.logout", label: "Sign out", description: "End a signed-in user session." },
  { value: "license.activate", label: "Activate license", description: "Activate a product license." },
  { value: "license.me", label: "Current license", description: "Read the current user's license." },
  { value: "device.verify", label: "Verify device", description: "Verify and register the current device." },
  { value: "device.me", label: "Current device", description: "Read the current device status." },
  { value: "variables.read_public", label: "Public variables", description: "Read public application variables." },
  { value: "me.read", label: "Current user", description: "Read the signed-in user's profile." },
];

const SECRET_SCOPES: CredentialScopeOption[] = [
  { value: "users.read", label: "Read users", description: "View application users." },
  { value: "users.write", label: "Manage users", description: "Create or update application users." },
  { value: "licenses.read", label: "Read licenses", description: "View product licenses." },
  { value: "licenses.write", label: "Manage licenses", description: "Create, change, or revoke licenses." },
  { value: "devices.read", label: "Read devices", description: "View registered devices." },
  { value: "devices.write", label: "Manage devices", description: "Change or remove registered devices." },
  { value: "sessions.read", label: "Read sessions", description: "View user sessions." },
  { value: "sessions.revoke", label: "Revoke sessions", description: "End user sessions." },
  { value: "variables.read", label: "Read variables", description: "View all application variables." },
  { value: "variables.write", label: "Manage variables", description: "Create or update application variables." },
  { value: "webhooks.read", label: "Read webhooks", description: "View webhook endpoints." },
  { value: "webhooks.write", label: "Manage webhooks", description: "Create or update webhook endpoints." },
  { value: "analytics.read", label: "Read analytics", description: "View application analytics." },
];

export function scopeOptionsForCredentialType(type: CredentialType): CredentialScopeOption[] {
  return type === "publishable" ? PUBLISHABLE_SCOPES : SECRET_SCOPES;
}

export function defaultScopesForCredentialType(type: CredentialType): string[] {
  return type === "publishable"
    ? ["auth.login", "device.verify", "auth.refresh", "auth.logout"]
    : ["users.read"];
}
