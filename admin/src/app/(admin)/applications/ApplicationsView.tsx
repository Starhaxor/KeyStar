import { EmptyNote, LoadingNote, TableCard } from "@/components/console/ConsoleSection";
import type { Application, Organization } from "@/lib/types";
import ApplicationLifecycleControls from "./ApplicationLifecycleControls";

function StatusBadge({ status }: { status: Application["status"] }) {
  const colors = status === "disabled" ? "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300" : status === "maintenance" ? "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-200" : "bg-success-50 text-success-700 dark:bg-success-500/15 dark:text-success-300";
  return <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${colors}`}>{status}</span>;
}

export default function ApplicationsView({ applications, organizations, loading, canWrite, onRefresh }: { applications: Application[]; organizations: Organization[]; loading: boolean; canWrite: boolean; onRefresh: () => Promise<void> }) {
  return <TableCard>{loading ? <LoadingNote /> : applications.length === 0 ? <EmptyNote message="No applications have been created." /> : <div className="overflow-x-auto"><table className="min-w-full text-left text-sm"><thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800"><tr><th className="px-5 py-3">Application</th><th>Organization</th><th>Status</th>{canWrite && <th className="px-5 py-3 text-right">Actions</th>}</tr></thead><tbody>{applications.map((application) => <tr key={application.id} className="border-b border-gray-100 align-top dark:border-gray-800"><td className="px-5 py-4"><strong>{application.name}</strong><div className="text-xs text-gray-500">{application.slug}</div></td><td className="py-4">{organizations.find((organization) => organization.id === application.organization_id)?.name ?? application.organization_id}</td><td className="py-4"><StatusBadge status={application.status} /></td>{canWrite && <td className="px-5 py-4"><ApplicationLifecycleControls application={application} canWrite={canWrite} onRefresh={onRefresh} /></td>}</tr>)}</tbody></table></div>}</TableCard>;
}
