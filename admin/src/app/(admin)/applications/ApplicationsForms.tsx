import Field from "@/components/form/Field";
import Button from "@/components/ui/button/Button";
import type { Organization } from "@/lib/types";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-white px-3 text-sm text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90 dark:[color-scheme:dark]";

export function OrganizationForm({ name, busy, onNameChange, onSubmit, onCancel }: { name: string; busy: boolean; onNameChange: (name: string) => void; onSubmit: () => void; onCancel: () => void }) {
  return <form className="mt-4 space-y-4" onSubmit={(event) => { event.preventDefault(); onSubmit(); }}>
    <Field id="organization-name" label="Organization name" name="name"><input className={inputClass} value={name} onChange={(event) => onNameChange(event.target.value)} /></Field>
    <div className="flex justify-end gap-2"><Button size="sm" variant="outline" type="button" disabled={busy} onClick={onCancel}>Cancel</Button><Button size="sm" disabled={busy || !name.trim()}>Create</Button></div>
  </form>;
}

export function ApplicationForm({ organizations, organizationID, name, slug, busy, onOrganizationChange, onNameChange, onSlugChange, onSubmit, onCancel }: { organizations: Organization[]; organizationID: string; name: string; slug: string; busy: boolean; onOrganizationChange: (organizationID: string) => void; onNameChange: (name: string) => void; onSlugChange: (slug: string) => void; onSubmit: () => void; onCancel: () => void }) {
  return <form className="mt-4 space-y-4" onSubmit={(event) => { event.preventDefault(); onSubmit(); }}>
    <Field id="application-organization" label="Organization" name="organization_id"><select className={inputClass} value={organizationID} onChange={(event) => onOrganizationChange(event.target.value)}><option value="">Select organization</option>{organizations.map((organization) => <option key={organization.id} value={organization.id}>{organization.name}</option>)}</select></Field>
    <Field id="application-name" label="Application name" name="name"><input className={inputClass} value={name} onChange={(event) => onNameChange(event.target.value)} /></Field>
    <Field id="application-slug" label="Slug (optional)" name="slug"><input className={inputClass} value={slug} placeholder="desktop-client" onChange={(event) => onSlugChange(event.target.value)} /></Field>
    <div className="flex justify-end gap-2"><Button size="sm" variant="outline" type="button" disabled={busy} onClick={onCancel}>Cancel</Button><Button size="sm" disabled={busy || !organizationID || !name.trim()}>Create</Button></div>
  </form>;
}
