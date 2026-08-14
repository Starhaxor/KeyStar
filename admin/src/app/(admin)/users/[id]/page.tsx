"use client";
import ConsoleSection, {
  EmptyNote,
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import ConfirmModal from "@/components/console/ConfirmModal";
import StatusBadge from "@/components/console/StatusBadge";
import Button from "@/components/ui/button/Button";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { api, formatDateTime } from "@/lib/api";
import type { UserDetail } from "@/lib/types";
import Link from "next/link";
import React, { useCallback, useEffect, useState, use } from "react";

export default function UserDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  const { hasPermission } = useAdminIdentity();
  const [detail, setDetail] = useState<UserDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const [revokeOpen, setRevokeOpen] = useState(false);
  const [revokeBusy, setRevokeBusy] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);
  const [revokedNotice, setRevokedNotice] = useState<string | null>(null);

  const canWriteSessions = hasPermission("sessions.write");

  const load = useCallback(async () => {
    try {
      setError(null);
      const response = await api.userDetail(id);
      setDetail(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load user");
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  async function handleStatusChange() {
    if (!detail) return;
    const nextStatus = detail.user.status === "active" ? "disabled" : "active";
    setBusy(true);
    setActionError(null);
    try {
      await api.setUserStatus(detail.user.id, nextStatus);
      setConfirmOpen(false);
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Action failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleRevokeSessions() {
    if (!detail) return;
    setRevokeBusy(true);
    setRevokeError(null);
    try {
      const response = await api.revokeUserSessions(detail.user.id);
      setRevokeOpen(false);
      setRevokedNotice(
        response.revoked === 0
          ? "No active sessions were found for this user."
          : `${response.revoked} session(s) revoked.`
      );
      await load();
    } catch (err) {
      setRevokeError(err instanceof Error ? err.message : "Revoke failed");
    } finally {
      setRevokeBusy(false);
    }
  }

  if (error) {
    return (
      <div>
        <PageTitle title="User Detail" />
        <div className="rounded-2xl border border-gray-200 bg-white shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
          <ErrorNote message={error} />
        </div>
      </div>
    );
  }
  if (!detail) {
    return (
      <div>
        <PageTitle title="User Detail" />
        <div className="rounded-2xl border border-gray-200 bg-white shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
          <LoadingNote />
        </div>
      </div>
    );
  }

  const { user, licenses, devices, sessions } = detail;
  const nextStatus = user.status === "active" ? "disabled" : "active";

  return (
    <div className="space-y-6">
      <PageTitle
        title={user.email}
        description={`User ${user.id}`}
        actions={
          <>
            <Link href="/users">
              <Button variant="outline" size="sm">
                Back to users
              </Button>
            </Link>
            {canWriteSessions && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setRevokeError(null);
                  setRevokeOpen(true);
                }}
              >
                Revoke sessions
              </Button>
            )}
            <Button
              size="sm"
              onClick={() => {
                setActionError(null);
                setConfirmOpen(true);
              }}
            >
              {nextStatus === "disabled" ? "Disable user" : "Enable user"}
            </Button>
          </>
        }
      />

      {revokedNotice && (
        <div className="rounded-xl border border-success-200 bg-success-50 px-4 py-3 text-sm text-success-700 dark:border-success-500/30 dark:bg-success-500/10 dark:text-success-400">
          {revokedNotice}
        </div>
      )}

      <ConsoleSection title="Profile">
        <div className="grid grid-cols-1 gap-4 px-5 py-4 sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <span className="block text-xs uppercase text-gray-400">Status</span>
            <span className="mt-1 block">
              <StatusBadge status={user.status} />
            </span>
          </div>
          <div>
            <span className="block text-xs uppercase text-gray-400">Created</span>
            <span className="mt-1 block text-sm text-gray-700 dark:text-gray-300">
              {formatDateTime(user.created_at)}
            </span>
          </div>
          <div>
            <span className="block text-xs uppercase text-gray-400">Last login</span>
            <span className="mt-1 block text-sm text-gray-700 dark:text-gray-300">
              {formatDateTime(user.last_login_at)}
            </span>
          </div>
          <div>
            <span className="block text-xs uppercase text-gray-400">
              Active sessions
            </span>
            <span className="mt-1 block text-sm text-gray-700 dark:text-gray-300">
              {user.active_session_count}
            </span>
          </div>
        </div>
      </ConsoleSection>

      <ConsoleSection title="Licenses" description={`${licenses.length} license(s)`}>
        {licenses.length === 0 ? (
          <EmptyNote message="No licenses for this user." />
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 dark:border-gray-800">
              <tr className="text-xs uppercase text-gray-400">
                <th className="px-5 py-3 font-medium">Product</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Max Devices</th>
                <th className="px-5 py-3 font-medium">Expires</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {licenses.map((license) => (
                <tr key={license.id}>
                  <td className="px-5 py-3.5 text-gray-700 dark:text-gray-300">
                    {license.product}
                  </td>
                  <td className="px-5 py-3.5">
                    <StatusBadge status={license.status} />
                  </td>
                  <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                    {license.max_devices}
                  </td>
                  <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                    {formatDateTime(license.expires_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </ConsoleSection>

      <ConsoleSection title="Devices" description={`${devices.length} device(s)`}>
        {devices.length === 0 ? (
          <EmptyNote message="No devices registered." />
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 dark:border-gray-800">
              <tr className="text-xs uppercase text-gray-400">
                <th className="px-5 py-3 font-medium">Device</th>
                <th className="px-5 py-3 font-medium">TPM</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Last Seen</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {devices.map((device) => (
                <tr key={device.id}>
                  <td className="px-5 py-3.5 font-mono text-xs text-gray-700 dark:text-gray-300">
                    {device.id.slice(0, 13)}…
                  </td>
                  <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                    {device.tpm_registered ? "Yes" : "No"}
                  </td>
                  <td className="px-5 py-3.5">
                    <StatusBadge status={device.status} />
                  </td>
                  <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                    {formatDateTime(device.last_seen_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </ConsoleSection>

      <ConsoleSection title="Sessions" description={`${sessions.length} session(s)`}>
        {sessions.length === 0 ? (
          <EmptyNote message="No auth sessions." />
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 dark:border-gray-800">
              <tr className="text-xs uppercase text-gray-400">
                <th className="px-5 py-3 font-medium">Session</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Expires</th>
                <th className="px-5 py-3 font-medium">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {sessions.map((session) => (
                <tr key={session.id}>
                  <td className="px-5 py-3.5 font-mono text-xs text-gray-700 dark:text-gray-300">
                    {session.id.slice(0, 13)}…
                  </td>
                  <td className="px-5 py-3.5">
                    <StatusBadge status={session.status} />
                  </td>
                  <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                    {formatDateTime(session.expires_at)}
                  </td>
                  <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                    {formatDateTime(session.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </ConsoleSection>

      <ConfirmModal
        isOpen={confirmOpen}
        title={nextStatus === "disabled" ? "Disable user" : "Enable user"}
        message={
          nextStatus === "disabled"
            ? `Disable ${user.email}? Their logins will stop working until re-enabled.`
            : `Re-enable ${user.email}?`
        }
        confirmLabel={nextStatus === "disabled" ? "Disable" : "Enable"}
        busy={busy}
        error={actionError}
        onConfirm={handleStatusChange}
        onClose={() => setConfirmOpen(false)}
      />

      <ConfirmModal
        isOpen={revokeOpen}
        title="Revoke sessions"
        message={`Revoke every active auth session of ${user.email}? Their devices will need to log in again.`}
        confirmLabel="Revoke sessions"
        busy={revokeBusy}
        error={revokeError}
        onConfirm={handleRevokeSessions}
        onClose={() => setRevokeOpen(false)}
      />
    </div>
  );
}
