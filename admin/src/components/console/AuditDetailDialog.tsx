"use client";

import AccessibleDialog from "@/components/ui/dialog/AccessibleDialog";
import { formatDateTime } from "@/lib/api";
import type { AuditEntry } from "@/lib/types";
import Link from "next/link";
import { type ReactNode, useState } from "react";

interface AuditDetailDialogProps {
  entry: AuditEntry | null;
  isOpen: boolean;
  onClose: () => void;
}

const unsafeMetadataKey = /(token|secret|password|fingerprint|hash|authorization|cookie|api[_-]?key|credential|private[_-]?key|error|stack|exception|trace|message|failure|cause|debug|diagnostic)/i;
const safeScalarKeys = new Set([
  "code", "count", "devices", "email", "environment", "environment_mode",
  "extend", "grace_hours", "level", "max_devices", "name", "replacement_id",
  "revoked", "role", "slug", "status", "type", "user_email",
]);
const safeObjectKeys = new Set(["before", "after"]);
const unsafeText = /(bearer\s+|-----BEGIN |\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.|\b(?:error|exception|stack trace|traceback|panic|connection failed|connection refused|econnrefused|database)\b|\bat\s+.+\.(?:go|ts|tsx|js|java|py):\d+)/i;
const encodedSecret = /(?:[a-f\d]{32,}|[A-Za-z\d+/_-]{48,}={0,2})/i;
const maxTextLength = 240;
const maxCollectionItems = 30;

function isSafeText(value: string) {
  return value.length <= maxTextLength && !/[\r\n\t]/.test(value) && !unsafeText.test(value) && !encodedSecret.test(value);
}

function isSafeScalarValue(key: string, value: string) {
  if (!isSafeText(value)) return false;
  if (key === "email" || key === "user_email") return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
  if (["count", "devices", "grace_hours", "level", "max_devices", "revoked"].includes(key)) return /^\d+$/.test(value);
  if (key === "extend") return /^\d+\s+(?:hour|day|week|month|year)s?$/.test(value);
  if (key === "replacement_id") return /^[\da-f]{8}-[\da-f-]{27,}$/i.test(value);
  return /^[\p{L}\p{N}][\p{L}\p{N} ._/-]{0,120}$/u.test(value);
}

function isPlainRecord(value: object): value is Record<string, unknown> {
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function SafeMetadataValue({ value, metadataKey, visited }: { value: unknown; metadataKey?: string; visited: WeakSet<object> }) {
  if (metadataKey && unsafeMetadataKey.test(metadataKey)) return <span>[redacted]</span>;
  if (metadataKey && !safeScalarKeys.has(metadataKey) && !safeObjectKeys.has(metadataKey)) return <span>[redacted]</span>;
  if (value === null || value === undefined) return <span>—</span>;
  if (typeof value === "string") {
    return <span>{metadataKey && isSafeScalarValue(metadataKey, value) ? value : "[redacted]"}</span>;
  }
  if (typeof value === "number") {
    return <span>{Number.isFinite(value) ? String(value) : "[redacted]"}</span>;
  }
  if (typeof value === "boolean") {
    return <span>{String(value)}</span>;
  }
  if (Array.isArray(value)) {
    if (!metadataKey || !safeObjectKeys.has(metadataKey)) return <span>[redacted]</span>;
    if (visited.has(value)) return <span>[redacted]</span>;
    visited.add(value);
    return (
      <ul className="list-inside list-disc space-y-1">
        {value.slice(0, maxCollectionItems).map((item, index) => <li key={index}><SafeMetadataValue value={item} visited={visited} /></li>)}
        {value.length > maxCollectionItems && <li>[additional values omitted]</li>}
      </ul>
    );
  }
  if (typeof value === "object" && isPlainRecord(value)) {
    if (metadataKey && !safeObjectKeys.has(metadataKey)) return <span>[redacted]</span>;
    if (visited.has(value)) return <span>[redacted]</span>;
    visited.add(value);
    return (
      <dl className="space-y-2 border-l border-gray-200 pl-3 dark:border-gray-700">
        {Object.entries(value).slice(0, maxCollectionItems).map(([key, nestedValue]) => (
          <div key={key} className="grid gap-1 sm:grid-cols-[9rem_1fr] sm:gap-3">
            <dt className="font-mono text-xs text-gray-500 dark:text-gray-400">{key}</dt>
            <dd className="break-all"><SafeMetadataValue value={nestedValue} metadataKey={key} visited={visited} /></dd>
          </div>
        ))}
        {Object.keys(value).length > maxCollectionItems && <div>[additional values omitted]</div>}
      </dl>
    );
  }
  return <span>[unsupported value]</span>;
}

function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 border-b border-gray-100 py-3 last:border-0 dark:border-gray-800 sm:grid-cols-[10rem_1fr] sm:gap-4">
      <dt className="text-xs font-medium uppercase tracking-wide text-gray-400">{label}</dt>
      <dd className="min-w-0 break-all text-sm text-gray-700 dark:text-gray-300">{children}</dd>
    </div>
  );
}

export default function AuditDetailDialog({ entry, isOpen, onClose }: AuditDetailDialogProps) {
  const [copied, setCopied] = useState(false);
  if (!entry) return null;

  const resourceId = entry.resource_id;
  const userHref = entry.resource_type === "user" && entry.resource_id ? `/users/${entry.resource_id}` : null;
  const visitedMetadata = new WeakSet<object>();

  async function copyResourceId() {
    if (!resourceId) return;
    try {
      await navigator.clipboard.writeText(resourceId);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  }

  return (
    <AccessibleDialog isOpen={isOpen} onClose={onClose} title="Audit event details" className="max-w-2xl p-6">
      <div className="space-y-5">
        <div>
          <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">Audit event details</h3>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">Recorded administrator activity and safe event metadata.</p>
        </div>
        <dl>
          <DetailRow label="Actor">{entry.actor_email}</DetailRow>
          <DetailRow label="Action">{entry.action}</DetailRow>
          <DetailRow label="Resource type">{entry.resource_type || "—"}</DetailRow>
          <DetailRow label="Resource ID">
            {entry.resource_id ? (
              <span className="flex flex-wrap items-center gap-2">
                {userHref ? <Link href={userHref} className="font-mono text-xs text-brand-500 hover:text-brand-600 dark:text-brand-400">{entry.resource_id}</Link> : <code className="font-mono text-xs">{entry.resource_id}</code>}
                <button type="button" aria-label="Copy resource ID" onClick={copyResourceId} className="rounded-md border border-gray-300 px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5">
                  {copied ? "Copied" : "Copy"}
                </button>
              </span>
            ) : "—"}
          </DetailRow>
          <DetailRow label="Timestamp">{formatDateTime(entry.created_at)}</DetailRow>
        </dl>
        <div>
          <h4 className="text-sm font-semibold text-gray-800 dark:text-white/90">Metadata</h4>
          <div className="mt-2 rounded-lg bg-gray-50 p-4 text-sm text-gray-700 dark:bg-white/[0.03] dark:text-gray-300">
            {Object.keys(entry.metadata ?? {}).length > 0 ? <SafeMetadataValue value={entry.metadata} visited={visitedMetadata} /> : "No metadata recorded."}
          </div>
        </div>
      </div>
    </AccessibleDialog>
  );
}
