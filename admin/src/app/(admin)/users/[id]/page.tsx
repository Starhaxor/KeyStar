"use client";
import ConsoleSection, {
  EmptyNote,
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import ConfirmModal from "@/components/console/ConfirmModal";
import LicenseCreateModal from "@/components/console/LicenseCreateModal";
import PromoteAdminModal from "@/components/console/PromoteAdminModal";
import ResetPasswordModal from "@/components/console/ResetPasswordModal";
import RowActions, { type RowAction } from "@/components/console/RowActions";
import StatusBadge from "@/components/console/StatusBadge";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import Label from "@/components/form/Label";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useToast } from "@/context/ToastContext";
import { api, formatDateTime } from "@/lib/api";
import type { UserDetail } from "@/lib/types";
import Link from "next/link";
import React, { useCallback, useEffect, useState, use } from "react";

const fieldClasses =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

type Tab = "profile" | "licenses" | "devices" | "sessions";

const tabs: { id: Tab; label: string }[] = [
  { id: "profile", label: "Profile" },
  { id: "licenses", label: "Licenses" },
  { id: "devices", label: "Devices" },
  { id: "sessions", label: "Sessions" },
];

export default function UserDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  const { hasPermission } = useAdminIdentity();
  const toast = useToast();
  const [detail, setDetail] = useState<UserDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("profile");

  // Account status
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  // Revoke all sessions (header action)
  const [revokeOpen, setRevokeOpen] = useState(false);
  const [revokeBusy, setRevokeBusy] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);
  const [revokedNotice, setRevokedNotice] = useState<string | null>(null);

  // License actions
  const [licenseOpen, setLicenseOpen] = useState(false);
  const [extendTarget, setExtendTarget] = useState<
    UserDetail["licenses"][number] | null
  >(null);
  const [extendDays, setExtendDays] = useState(30);
  const [extendMaxDevices, setExtendMaxDevices] = useState(0);
  const [extendBusy, setExtendBusy] = useState(false);
  const [extendError, setExtendError] = useState<string | null>(null);
  const [licenseRevokeTarget, setLicenseRevokeTarget] = useState<
    UserDetail["licenses"][number] | null
  >(null);
  const [licenseRevokeBusy, setLicenseRevokeBusy] = useState(false);
  const [licenseRevokeError, setLicenseRevokeError] = useState<string | null>(null);

  // Device actions
  const [deviceResetTarget, setDeviceResetTarget] = useState<
    UserDetail["devices"][number] | null
  >(null);
  const [deviceResetBusy, setDeviceResetBusy] = useState(false);
  const [deviceResetError, setDeviceResetError] = useState<string | null>(null);
  const [deviceRevokeTarget, setDeviceRevokeTarget] = useState<
    UserDetail["devices"][number] | null
  >(null);
  const [deviceRevokeBusy, setDeviceRevokeBusy] = useState(false);
  const [deviceRevokeError, setDeviceRevokeError] = useState<string | null>(null);

  // Session action
  const [sessionRevokeTarget, setSessionRevokeTarget] = useState<
    UserDetail["sessions"][number] | null
  >(null);
  const [sessionRevokeBusy, setSessionRevokeBusy] = useState(false);
  const [sessionRevokeError, setSessionRevokeError] = useState<string | null>(null);

  // Password reset
  const [resetOpen, setResetOpen] = useState(false);

  // Promote to admin
  const [promoteOpen, setPromoteOpen] = useState(false);

  // Notes
  const [notesText, setNotesText] = useState("");
  const [notesBusy, setNotesBusy] = useState(false);
  const [notesError, setNotesError] = useState<string | null>(null);
  const [notesNotice, setNotesNotice] = useState<string | null>(null);

  // Ban / unban
  const [banOpen, setBanOpen] = useState(false);
  const [banReason, setBanReason] = useState("");
  const [banBusy, setBanBusy] = useState(false);
  const [banError, setBanError] = useState<string | null>(null);

  // Reset all devices (HWID reset)
  const [resetDevicesOpen, setResetDevicesOpen] = useState(false);
  const [resetDevicesBusy, setResetDevicesBusy] = useState(false);
  const [resetDevicesError, setResetDevicesError] = useState<string | null>(null);
  const [resetDevicesNotice, setResetDevicesNotice] = useState<string | null>(null);

  const canWriteSessions = hasPermission("sessions.write");
  const canWriteLicenses = hasPermission("licenses.write");
  const canWriteDevices = hasPermission("devices.write");
  const canWriteAdmins = hasPermission("admins.write");
  const canWriteUsers = hasPermission("users.write");

  const load = useCallback(async () => {
    try {
      setError(null);
      const response = await api.userDetail(id);
      setDetail(response);
      setNotesText(response.notes ?? "");
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
      toast.success(nextStatus === "disabled" ? "User disabled" : "User enabled", detail.user.email);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Action failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleSaveNotes() {
    if (!detail) return;
    setNotesBusy(true);
    setNotesError(null);
    setNotesNotice(null);
    try {
      await api.setUserNotes(detail.user.id, notesText);
      setNotesNotice("Notes saved");
      toast.success("Notes saved", detail.user.email);
    } catch (err) {
      setNotesError(err instanceof Error ? err.message : "Save failed");
    } finally {
      setNotesBusy(false);
    }
  }

  async function handleBan() {
    if (!detail) return;
    setBanBusy(true);
    setBanError(null);
    try {
      await api.banUser(detail.user.id, banReason.trim());
      setBanOpen(false);
      setBanReason("");
      await load();
      toast.success("User banned", detail.user.email);
    } catch (err) {
      setBanError(err instanceof Error ? err.message : "Ban failed");
    } finally {
      setBanBusy(false);
    }
  }

  async function handleUnban() {
    if (!detail) return;
    setBusy(true);
    setActionError(null);
    try {
      await api.unbanUser(detail.user.id);
      await load();
      toast.success("User unbanned", detail.user.email);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Action failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleResetDevices() {
    if (!detail) return;
    setResetDevicesBusy(true);
    setResetDevicesError(null);
    setResetDevicesNotice(null);
    try {
      const response = await api.resetUserDevices(detail.user.id);
      setResetDevicesOpen(false);
      setResetDevicesNotice(
        `${response.devices} device${response.devices === 1 ? "" : "s"} reset — hardware will re-register on next launch.`
      );
      await load();
      toast.success("Devices reset", detail.user.email);
    } catch (err) {
      setResetDevicesError(err instanceof Error ? err.message : "Reset failed");
    } finally {
      setResetDevicesBusy(false);
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
      toast.success("Sessions revoked", detail.user.email);
    } catch (err) {
      setRevokeError(err instanceof Error ? err.message : "Revoke failed");
    } finally {
      setRevokeBusy(false);
    }
  }

  async function handleExtend(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!extendTarget) return;
    setExtendBusy(true);
    setExtendError(null);
    try {
      await api.updateLicense(extendTarget.id, extendDays, extendMaxDevices);
      setExtendTarget(null);
      await load();
      toast.success("License extended", extendTarget.user_email);
    } catch (err) {
      setExtendError(err instanceof Error ? err.message : "Update failed");
    } finally {
      setExtendBusy(false);
    }
  }

  async function handleLicenseRevoke() {
    if (!licenseRevokeTarget) return;
    setLicenseRevokeBusy(true);
    setLicenseRevokeError(null);
    try {
      await api.revokeLicense(licenseRevokeTarget.id);
      setLicenseRevokeTarget(null);
      await load();
      toast.success("License revoked", licenseRevokeTarget.user_email);
    } catch (err) {
      setLicenseRevokeError(err instanceof Error ? err.message : "Revoke failed");
    } finally {
      setLicenseRevokeBusy(false);
    }
  }

  async function handleDeviceReset() {
    if (!deviceResetTarget) return;
    setDeviceResetBusy(true);
    setDeviceResetError(null);
    try {
      await api.resetDevice(deviceResetTarget.id);
      setDeviceResetTarget(null);
      await load();
      toast.success("Device reset", deviceResetTarget.user_email);
    } catch (err) {
      setDeviceResetError(err instanceof Error ? err.message : "Reset failed");
    } finally {
      setDeviceResetBusy(false);
    }
  }

  async function handleDeviceRevoke() {
    if (!deviceRevokeTarget) return;
    setDeviceRevokeBusy(true);
    setDeviceRevokeError(null);
    try {
      await api.revokeDevice(deviceRevokeTarget.id);
      setDeviceRevokeTarget(null);
      await load();
      toast.success("Device revoked", deviceRevokeTarget.user_email);
    } catch (err) {
      setDeviceRevokeError(err instanceof Error ? err.message : "Revoke failed");
    } finally {
      setDeviceRevokeBusy(false);
    }
  }

  async function handleSessionRevoke() {
    if (!sessionRevokeTarget) return;
    setSessionRevokeBusy(true);
    setSessionRevokeError(null);
    try {
      await api.revokeSession(sessionRevokeTarget.id);
      setSessionRevokeTarget(null);
      await load();
      toast.success("Session revoked", sessionRevokeTarget.user_email);
    } catch (err) {
      setSessionRevokeError(err instanceof Error ? err.message : "Revoke failed");
    } finally {
      setSessionRevokeBusy(false);
    }
  }

  async function handlePromote(role: string) {
    if (!detail) return { tempPassword: "" };
    const response = await api.promoteToAdmin(detail.user.id, role);
    await load();
    toast.success("Admin account created", detail.user.email);
    return { tempPassword: response.temp_password };
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
            {canWriteAdmins && (
              <Button
                variant="info"
                size="sm"
                onClick={() => setPromoteOpen(true)}
              >
                Promote to admin
              </Button>
            )}
            {canWriteLicenses && (
              <Button
                variant="success"
                size="sm"
                onClick={() => setLicenseOpen(true)}
              >
                Add license
              </Button>
            )}
            {canWriteSessions && (
              <Button
                variant="warning"
                size="sm"
                onClick={() => {
                  setRevokeError(null);
                  setRevokeOpen(true);
                }}
              >
                Revoke sessions
              </Button>
            )}
            {canWriteUsers && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setResetOpen(true)}
              >
                Reset password
              </Button>
            )}
            {canWriteDevices && devices.length > 0 && (
              <Button
                variant="warning"
                size="sm"
                onClick={() => {
                  setResetDevicesError(null);
                  setResetDevicesOpen(true);
                }}
              >
                Reset devices
              </Button>
            )}
            {user.status === "banned" ? (
              <Button variant="success" size="sm" onClick={handleUnban} disabled={busy}>
                Unban user
              </Button>
            ) : (
              <>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => {
                    setBanError(null);
                    setBanOpen(true);
                  }}
                >
                  Ban user
                </Button>
                <Button
                  variant={nextStatus === "disabled" ? "danger" : "success"}
                  size="sm"
                  onClick={() => {
                    setActionError(null);
                    setConfirmOpen(true);
                  }}
                >
                  {nextStatus === "disabled" ? "Disable user" : "Enable user"}
                </Button>
              </>
            )}
          </>
        }
      />

      {revokedNotice && (
        <div className="rounded-xl border border-success-200 bg-success-50 px-4 py-3 text-sm text-success-700 dark:border-success-500/30 dark:bg-success-500/10 dark:text-success-400">
          {revokedNotice}
        </div>
      )}
      {resetDevicesNotice && (
        <div className="rounded-xl border border-warning-200 bg-warning-50 px-4 py-3 text-sm text-warning-700 dark:border-warning-500/30 dark:bg-warning-500/10 dark:text-warning-400">
          {resetDevicesNotice}
        </div>
      )}
      {user.status === "banned" && (
        <div className="rounded-xl border border-error-200 bg-error-50 px-4 py-3 text-sm text-error-700 dark:border-error-500/30 dark:bg-error-500/10 dark:text-error-400">
          <span className="font-semibold">Banned</span>
          {detail.banned_at && ` · ${formatDateTime(detail.banned_at)}`}
          {detail.ban_reason && (
            <span className="mt-1 block">Reason: {detail.ban_reason}</span>
          )}
        </div>
      )}

      {/* Horizontal tab bar */}
      <div className="flex items-center gap-1 overflow-x-auto border-b border-gray-200 no-scrollbar dark:border-gray-800">
        {tabs.map((item) => {
          const count =
            item.id === "licenses"
              ? licenses.length
              : item.id === "devices"
                ? devices.length
                : item.id === "sessions"
                  ? sessions.length
                  : null;
          const active = tab === item.id;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => setTab(item.id)}
              className={`shrink-0 whitespace-nowrap rounded-t-lg border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
                active
                  ? "border-brand-500 text-brand-600 dark:border-brand-400 dark:text-brand-400"
                  : "border-transparent text-gray-600 hover:bg-gray-100 hover:text-gray-800 dark:text-gray-400 dark:hover:bg-white/5 dark:hover:text-gray-200"
              }`}
            >
              {item.label}
              {count !== null && (
                <span
                  className={`ml-2 rounded-full px-2 py-0.5 text-xs ${
                    active
                      ? "bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-400"
                      : "bg-gray-100 text-gray-500 dark:bg-white/[0.05] dark:text-gray-400"
                  }`}
                >
                  {count}
                </span>
              )}
            </button>
          );
        })}
      </div>

      {tab === "profile" && (
        <>
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

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <div className="rounded-2xl border border-gray-200 bg-white p-5 shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
              <span className="block text-sm text-gray-500 dark:text-gray-400">Licenses</span>
              <span className="mt-1 block text-2xl font-semibold text-gray-800 dark:text-white/90">
                {licenses.length}
              </span>
            </div>
            <div className="rounded-2xl border border-gray-200 bg-white p-5 shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
              <span className="block text-sm text-gray-500 dark:text-gray-400">Devices</span>
              <span className="mt-1 block text-2xl font-semibold text-gray-800 dark:text-white/90">
                {devices.length}
              </span>
            </div>
            <div className="rounded-2xl border border-gray-200 bg-white p-5 shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
              <span className="block text-sm text-gray-500 dark:text-gray-400">Sessions</span>
              <span className="mt-1 block text-2xl font-semibold text-gray-800 dark:text-white/90">
                {sessions.length}
              </span>
            </div>
          </div>

          <ConsoleSection
            title="Notes"
            description="KeyAuth-style notes visible only to administrators."
            actions={
              canWriteUsers ? (
                <Button
                  size="sm"
                  disabled={notesBusy}
                  onClick={handleSaveNotes}
                >
                  {notesBusy ? "Saving..." : "Save notes"}
                </Button>
              ) : undefined
            }
          >
            <div className="px-5 py-4">
              <textarea
                value={notesText}
                onChange={(e) => setNotesText(e.target.value)}
                maxLength={4000}
                rows={4}
                placeholder="Internal notes about this user (license history, support context, warnings)..."
                disabled={!canWriteUsers}
                className="w-full resize-y rounded-lg border border-gray-300 bg-transparent px-4 py-3 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 disabled:opacity-60 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
              />
              {notesNotice && (
                <p className="mt-2 text-sm text-success-600 dark:text-success-400">
                  {notesNotice}
                </p>
              )}
              {notesError && (
                <p className="mt-2 text-sm text-error-500">{notesError}</p>
              )}
            </div>
          </ConsoleSection>
        </>
      )}

      {tab === "licenses" && (
        <ConsoleSection
          title="Licenses"
          description={`${licenses.length} license(s)`}
          actions={
            canWriteLicenses ? (
              <Button size="sm" onClick={() => setLicenseOpen(true)}>
                Add license
              </Button>
            ) : undefined
          }
        >
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
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {licenses.map((license) => {
                  const actions: RowAction[] = [
                    {
                      label: "Extend",
                      tone: "warning",
                      disabled: license.status === "revoked",
                      onClick: () => {
                        setExtendError(null);
                        setExtendDays(30);
                        setExtendMaxDevices(license.max_devices);
                        setExtendTarget(license);
                      },
                    },
                    {
                      label: "Revoke",
                      tone: "danger",
                      disabled: license.status === "revoked",
                      onClick: () => {
                        setLicenseRevokeError(null);
                        setLicenseRevokeTarget(license);
                      },
                    },
                  ];
                  return (
                    <tr
                      key={license.id}
                      className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                    >
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
                      <td className="px-5 py-3.5">
                        <div className="flex justify-end">
                          <RowActions actions={actions} />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </ConsoleSection>
      )}

      {tab === "devices" && (
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
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {devices.map((device) => {
                  const actions: RowAction[] = [];
                  if (canWriteDevices) {
                    actions.push({
                      label: "Reset",
                      tone: "warning",
                      onClick: () => {
                        setDeviceResetError(null);
                        setDeviceResetTarget(device);
                      },
                    });
                  }
                  actions.push({
                    label: "Revoke",
                    tone: "danger",
                    disabled: device.status === "revoked",
                    onClick: () => {
                      setDeviceRevokeError(null);
                      setDeviceRevokeTarget(device);
                    },
                  });
                  return (
                    <tr
                      key={device.id}
                      className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                    >
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
                      <td className="px-5 py-3.5">
                        <div className="flex justify-end">
                          <RowActions actions={actions} />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </ConsoleSection>
      )}

      {tab === "sessions" && (
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
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {sessions.map((session) => {
                  const actions: RowAction[] = [
                    {
                      label: "Revoke",
                      tone: "danger",
                      disabled: session.status === "revoked",
                      onClick: () => {
                        setSessionRevokeError(null);
                        setSessionRevokeTarget(session);
                      },
                    },
                  ];
                  return (
                    <tr
                      key={session.id}
                      className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                    >
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
                      <td className="px-5 py-3.5">
                        <div className="flex justify-end">
                          <RowActions actions={actions} />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </ConsoleSection>
      )}

      <LicenseCreateModal
        open={licenseOpen}
        defaultEmail={user.email}
        onClose={() => setLicenseOpen(false)}
        onCreated={load}
      />

      {/* Extend license */}
      <Modal
        isOpen={extendTarget !== null}
        onClose={() => !extendBusy && setExtendTarget(null)}
        className="max-w-md p-6"
      >
        {extendTarget && (
          <form onSubmit={handleExtend}>
            <h3 className="mb-4 text-lg font-semibold text-gray-800 dark:text-white/90">
              Extend License
            </h3>
            <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
              {extendTarget.user_email} · currently expires{" "}
              {formatDateTime(extendTarget.expires_at)}
            </p>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">
                  Extend by (days)
                </label>
                <input
                  className={fieldClasses}
                  type="number"
                  min={0}
                  max={3650}
                  value={extendDays}
                  onChange={(e) => setExtendDays(Number(e.target.value))}
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">
                  Max Devices
                </label>
                <input
                  className={fieldClasses}
                  type="number"
                  min={1}
                  max={10000}
                  value={extendMaxDevices}
                  onChange={(e) => setExtendMaxDevices(Number(e.target.value))}
                />
              </div>
            </div>
            {extendError && (
              <p className="mt-3 text-sm text-error-500" role="alert">
                {extendError}
              </p>
            )}
            <div className="mt-5 flex justify-end gap-3">
              <button
                type="button"
                disabled={extendBusy}
                onClick={() => setExtendTarget(null)}
                className="inline-flex items-center justify-center font-medium gap-2 rounded-lg transition px-4 py-3 text-sm bg-white text-gray-700 ring-1 ring-inset ring-gray-300 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-400 dark:ring-gray-700 disabled:opacity-50"
              >
                Cancel
              </button>
              <Button size="sm" disabled={extendBusy}>
                {extendBusy ? "Saving..." : "Save"}
              </Button>
            </div>
          </form>
        )}
      </Modal>

      {/* Promote to admin */}
      <PromoteAdminModal
        open={promoteOpen}
        userEmail={user.email}
        onClose={() => setPromoteOpen(false)}
        onPromote={handlePromote}
      />

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

      <ConfirmModal
        isOpen={licenseRevokeTarget !== null}
        title="Revoke license"
        message={
          licenseRevokeTarget
            ? `Revoke the license of ${licenseRevokeTarget.user_email}? Active sessions for it will be expired. This cannot be undone.`
            : ""
        }
        confirmLabel="Revoke"
        busy={licenseRevokeBusy}
        error={licenseRevokeError}
        onConfirm={handleLicenseRevoke}
        onClose={() => setLicenseRevokeTarget(null)}
      />

      <ConfirmModal
        isOpen={deviceResetTarget !== null}
        title="Reset device"
        message={
          deviceResetTarget
            ? `Delete the hardware registration for ${deviceResetTarget.user_email}? The device will be able to register again on its next launch.`
            : ""
        }
        confirmLabel="Reset"
        busy={deviceResetBusy}
        error={deviceResetError}
        onConfirm={handleDeviceReset}
        onClose={() => setDeviceResetTarget(null)}
      />

      <ConfirmModal
        isOpen={deviceRevokeTarget !== null}
        title="Revoke device"
        message={
          deviceRevokeTarget
            ? `Revoke this device for ${deviceRevokeTarget.user_email}? The device will no longer be able to authenticate.`
            : ""
        }
        confirmLabel="Revoke"
        busy={deviceRevokeBusy}
        error={deviceRevokeError}
        onConfirm={handleDeviceRevoke}
        onClose={() => setDeviceRevokeTarget(null)}
      />

      <ResetPasswordModal
        open={resetOpen}
        userEmail={user.email}
        onClose={() => setResetOpen(false)}
        onReset={async (password) => {
          const response = await api.resetUserPassword(
            user.id,
            password || undefined
          );
          await load();
          toast.success("Password reset", user.email);
          return { tempPassword: response.temp_password ?? null };
        }}
      />

      <ConfirmModal
        isOpen={sessionRevokeTarget !== null}
        title="Revoke session"
        message={
          sessionRevokeTarget
            ? `Revoke this session for ${sessionRevokeTarget.user_email}? They will be signed out immediately.`
            : ""
        }
        confirmLabel="Revoke"
        busy={sessionRevokeBusy}
        error={sessionRevokeError}
        onConfirm={handleSessionRevoke}
        onClose={() => setSessionRevokeTarget(null)}
      />

      {/* Ban user */}
      <Modal
        isOpen={banOpen}
        onClose={() => !banBusy && setBanOpen(false)}
        className="max-w-md p-6"
      >
        <div className="space-y-5">
          <div>
            <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
              Ban user
            </h3>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Banned users cannot sign in until unbanned. The reason is recorded
              and shown on this profile.
            </p>
          </div>
          <div>
            <Label>Reason *</Label>
            <textarea
              value={banReason}
              onChange={(e) => setBanReason(e.target.value)}
              rows={3}
              maxLength={500}
              placeholder="e.g. license abuse, chargeback, shared account..."
              className="w-full resize-y rounded-lg border border-gray-300 bg-transparent px-4 py-3 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            />
          </div>
          {banError && (
            <p className="text-sm text-error-500" role="alert">
              {banError}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300"
              onClick={() => setBanOpen(false)}
            >
              Cancel
            </button>
            <Button
              size="sm"
              disabled={banBusy || banReason.trim() === ""}
              onClick={handleBan}
            >
              {banBusy ? "Banning..." : "Ban user"}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Reset all devices (HWID reset) */}
      <ConfirmModal
        isOpen={resetDevicesOpen}
        title="Reset all devices"
        message={`Delete every registered device of ${user.email}? All their hardware will need to re-register on the next launch. This is a full HWID reset.`}
        confirmLabel="Reset devices"
        busy={resetDevicesBusy}
        error={resetDevicesError}
        onConfirm={handleResetDevices}
        onClose={() => setResetDevicesOpen(false)}
      />
    </div>
  );
}
