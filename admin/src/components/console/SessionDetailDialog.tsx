"use client";

import AccessibleDialog from "@/components/ui/dialog/AccessibleDialog";
import { formatDateTime } from "@/lib/api";
import type { ConsoleSession } from "@/lib/types";
import Link from "next/link";
import type { ReactNode } from "react";

interface SessionDetailDialogProps {
  session: ConsoleSession | null;
  isOpen: boolean;
  onClose: () => void;
}

function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 border-b border-gray-100 py-3 last:border-0 dark:border-gray-800 sm:grid-cols-[10rem_1fr] sm:gap-4">
      <dt className="text-xs font-medium uppercase tracking-wide text-gray-400">{label}</dt>
      <dd className="min-w-0 break-all text-sm text-gray-700 dark:text-gray-300">{children}</dd>
    </div>
  );
}

export default function SessionDetailDialog({ session, isOpen, onClose }: SessionDetailDialogProps) {
  if (!session) return null;

  return (
    <AccessibleDialog isOpen={isOpen} onClose={onClose} title="Session details" className="max-w-xl p-6">
      <div className="space-y-5">
        <div>
          <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">Session details</h3>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Safe session metadata only. Authentication tokens and hardware fingerprints are never displayed.
          </p>
        </div>
        <dl>
          <DetailRow label="Session ID"><code className="font-mono text-xs">{session.id}</code></DetailRow>
          <DetailRow label="User">
            <Link href={`/users/${session.user_id}`} className="text-brand-500 hover:text-brand-600 dark:text-brand-400">
              {session.user_email}
            </Link>
          </DetailRow>
          <DetailRow label="User ID"><code className="font-mono text-xs">{session.user_id}</code></DetailRow>
          <DetailRow label="License ID"><code className="font-mono text-xs">{session.license_id}</code></DetailRow>
          <DetailRow label="Status">{session.status}</DetailRow>
          <DetailRow label="Expires">{formatDateTime(session.expires_at)}</DetailRow>
          <DetailRow label="Created">{formatDateTime(session.created_at)}</DetailRow>
        </dl>
      </div>
    </AccessibleDialog>
  );
}
