import { EmptyNote, LoadingNote, TableCard } from "@/components/console/ConsoleSection";
import type { Application, Organization } from "@/lib/types";

export default function ApplicationsView({ applications, organizations, loading }: { applications: Application[]; organizations: Organization[]; loading: boolean }) {
  return <TableCard>{loading ? <LoadingNote /> : applications.length === 0 ? <EmptyNote message="No applications have been created." /> : <table className="min-w-full text-left text-sm"><thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800"><tr><th className="px-5 py-3">Application</th><th>Organization</th><th>Status</th></tr></thead><tbody>{applications.map((application) => <tr key={application.id} className="border-b border-gray-100 dark:border-gray-800"><td className="px-5 py-4"><strong>{application.name}</strong><div className="text-xs text-gray-500">{application.slug}</div></td><td>{organizations.find((organization) => organization.id === application.organization_id)?.name ?? application.organization_id}</td><td>{application.status}</td></tr>)}</tbody></table>}</TableCard>;
}
