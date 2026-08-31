"use client";

import { EmptyNote, LoadingNote, TableCard } from "@/components/console/ConsoleSection";
import type { Application, Organization } from "@/lib/types";
import { useState } from "react";
import ApplicationLifecycleControls from "./ApplicationLifecycleControls";

function StatusBadge({ status }: { status: Application["status"] }) {
  const colors = status === "disabled" ? "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300" : status === "maintenance" ? "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-200" : "bg-success-50 text-success-700 dark:bg-success-500/15 dark:text-success-300";
  return <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${colors}`}>{status}</span>;
}

function ApplicationIdentity({ application }: { application: Application }) {
  const [copied, setCopied] = useState(false);

  async function copyID() {
    try {
      await navigator.clipboard.writeText(application.id);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className="mt-2 flex max-w-md items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-2 dark:border-gray-800 dark:bg-white/[0.03]">
      <code className="min-w-0 flex-1 break-all font-mono text-[11px] leading-4 text-gray-500 dark:text-gray-400">{application.id}</code>
      <button type="button" aria-label={`Copy ${application.name} application ID`} onClick={() => void copyID()} className="shrink-0 rounded-md border border-gray-300 bg-white px-2 py-1 text-[11px] font-semibold text-gray-600 transition-colors hover:border-brand-300 hover:text-brand-600 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300 dark:hover:border-brand-500/50 dark:hover:text-brand-400">
        {copied ? "Copied" : "Copy ID"}
      </button>
    </div>
  );
}

export default function ApplicationsView({ applications, organizations, loading, canWrite, onRefresh }: { applications: Application[]; organizations: Organization[]; loading: boolean; canWrite: boolean; onRefresh: () => Promise<void> }) {
  return <TableCard>{loading ? <LoadingNote /> : applications.length === 0 ? <EmptyNote message="No applications have been created." /> : <div className="overflow-x-auto"><table className="min-w-full text-left text-sm"><thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800"><tr><th className="px-5 py-3">Application</th><th>Organization</th><th>Status</th>{canWrite && <th className="px-5 py-3 text-right">Actions</th>}</tr></thead><tbody>{applications.map((application) => <tr key={application.id} className="border-b border-gray-100 align-top dark:border-gray-800"><td className="px-5 py-4"><strong>{application.name}</strong><div className="text-xs text-gray-500">{application.slug}</div><ApplicationIdentity application={application} /></td><td className="py-4">{organizations.find((organization) => organization.id === application.organization_id)?.name ?? application.organization_id}</td><td className="py-4"><StatusBadge status={application.status} /></td>{canWrite && <td className="px-5 py-4"><ApplicationLifecycleControls application={application} canWrite={canWrite} onRefresh={onRefresh} /></td>}</tr>)}</tbody></table></div>}</TableCard>;
}
