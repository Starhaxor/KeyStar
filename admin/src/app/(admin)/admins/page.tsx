"use client";
import ConsoleSection, {
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import EmptyState from "@/components/console/EmptyState";
import StatusBadge from "@/components/console/StatusBadge";
import RowActions, { type RowAction } from "@/components/console/RowActions";
import ResetPasswordModal from "@/components/console/ResetPasswordModal";
import Label from "@/components/form/Label";
import { inputClasses } from "@/components/form/inputStyles";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useToast } from "@/context/ToastContext";
import { api, formatDateTime } from "@/lib/api";
import type { AdminAccount, AdminRole } from "@/lib/types";
import { GroupIcon } from "@/icons";
import React, { useCallback, useEffect, useMemo, useState } from "react";

const selectClasses =
  "h-11 w-full rounded-lg border appearance-none px-4 py-2.5 text-sm shadow-theme-xs focus:outline-hidden focus:ring-3 dark:bg-gray-900 dark:text-white/90 bg-transparent text-gray-800 border-gray-300 focus:border-brand-300 focus:ring-brand-500/10 dark:border-gray-700 dark:focus:border-brand-800";

// Admins page: lists admin accounts and lets holders of admins.write change
// another account's role or status. Editing your own account is blocked both
// here and by the backend (ADMIN_SELF_MODIFICATION).

export default function AdminsPage() {
  const { identity, hasPermission } = useAdminIdentity();
  const [admins, setAdmins] = useState<AdminAccount[]>([]);
  const [roles, setRoles] = useState<AdminRole[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState("");

  const [editing, setEditing] = useState<AdminAccount | null>(null);
  const [editRole, setEditRole] = useState("");
  const [editStatus, setEditStatus] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [createOpen, setCreateOpen] = useState(false);
  const [createEmail, setCreateEmail] = useState("");
  const [createPassword, setCreatePassword] = useState("");
  const [createRole, setCreateRole] = useState("viewer");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const [resetTarget, setResetTarget] = useState<AdminAccount | null>(null);
  const toast = useToast();

  const canWrite = hasPermission("admins.write");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const [adminList, roleList] = await Promise.all([
        api.admins(),
        api.roles(),
      ]);
      setAdmins(adminList.items ?? []);
      setRoles(roleList.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load admins");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("create") === "1") {
      window.history.replaceState({}, "", "/admins");
      openCreate();
    }
     
  }, []);

  function openEdit(admin: AdminAccount) {
    setEditing(admin);
    setEditRole(admin.role);
    setEditStatus(admin.status);
    setSaveError(null);
  }

  async function saveEdit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editing) return;
    setSaving(true);
    setSaveError(null);
    try {
      await api.updateAdmin(editing.id, { role: editRole, status: editStatus });
      setEditing(null);
      await load();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "Update failed");
    } finally {
      setSaving(false);
    }
  }

  function openCreate() {
    setCreateEmail("");
    setCreatePassword("");
    setCreateRole("viewer");
    setCreateError(null);
    setCreateOpen(true);
  }

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreating(true);
    setCreateError(null);
    try {
      await api.createAdmin(createEmail.trim(), createPassword, createRole);
      setCreateOpen(false);
      await load();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "Create failed");
    } finally {
      setCreating(false);
    }
  }

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return admins;
    return admins.filter(
      (admin) =>
        admin.email.toLowerCase().includes(q) ||
        admin.role.toLowerCase().includes(q) ||
        admin.status.toLowerCase().includes(q)
    );
  }, [admins, filter]);

  const ownerCount = admins.filter((a) => a.role === "owner").length;
  const activeCount = admins.filter((a) => a.status === "active").length;
  const mfaCount = admins.filter((a) => a.mfa_enrolled).length;

  return (
    <div>
      <PageTitle
        title="Admins"
        description="Administrator accounts, roles and account status."
      />
      {!loading && !error && (
        <div className="mb-4 flex flex-wrap gap-2">
          <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
            {admins.length} total
          </span>
          <span className="rounded-full bg-success-50 px-3 py-1 text-xs font-medium text-success-600 dark:bg-success-500/10 dark:text-success-400">
            {activeCount} active
          </span>
          <span className="rounded-full bg-brand-50 px-3 py-1 text-xs font-medium text-brand-600 dark:bg-brand-500/10 dark:text-brand-400">
            {ownerCount} owner
          </span>
          <span className="rounded-full bg-warning-50 px-3 py-1 text-xs font-medium text-warning-600 dark:bg-warning-500/10 dark:text-warning-400">
            {mfaCount} MFA enrolled
          </span>
        </div>
      )}
      <ConsoleSection
        title="Admin Accounts"
        description={`${admins.length} account(s)`}
        actions={
          <div className="flex items-center gap-2">
            <input
              type="search"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Type to filter..."
              className="h-10 w-56 rounded-lg border border-gray-300 bg-transparent px-3.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            />
            {canWrite ? (
              <Button size="sm" onClick={openCreate}>
                Add admin
              </Button>
            ) : undefined}
          </div>
        }
      >
        {loading && !error ? (
          <LoadingNote />
        ) : error ? (
          <ErrorNote message={error} />
        ) : admins.length === 0 ? (
          <EmptyState
            icon={<GroupIcon />}
            title="No admin accounts found"
            message="Console operators will appear here."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={<GroupIcon />}
            title="No matching admins"
            message={`Nothing matches “${filter}”.`}
          />
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 dark:border-gray-800">
              <tr className="text-xs uppercase text-gray-400">
                <th className="px-5 py-3 font-medium">Email</th>
                <th className="px-5 py-3 font-medium">Role</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">MFA</th>
                <th className="px-5 py-3 font-medium">Created</th>
                {canWrite && <th className="px-5 py-3 font-medium"></th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {filtered.map((admin) => {
                const isSelf = admin.id === identity?.id;
                return (
                  <tr key={admin.id} className="hover:bg-gray-50 dark:hover:bg-white/[0.02]">
                    <td className="px-5 py-3.5 text-gray-700 dark:text-gray-300">
                      {admin.email}
                      {isSelf && (
                        <span className="ml-2 rounded-full bg-brand-50 px-2 py-0.5 text-xs font-medium text-brand-500 dark:bg-brand-500/[0.1]">
                          you
                        </span>
                      )}
                    </td>
                    <td className="px-5 py-3.5">
                      <span className="inline-flex rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
                        {admin.role}
                      </span>
                    </td>
                    <td className="px-5 py-3.5">
                      <StatusBadge status={admin.status} />
                    </td>
                    <td className="px-5 py-3.5">
                      <span
                        className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${
                          admin.mfa_enrolled
                            ? "bg-success-50 text-success-600 dark:bg-success-500/[0.1]"
                            : "bg-warning-50 text-warning-600 dark:bg-warning-500/[0.1]"
                        }`}
                      >
                        {admin.mfa_enrolled ? "enabled" : "pending"}
                      </span>
                    </td>
                    <td className="whitespace-nowrap px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      {formatDateTime(admin.created_at)}
                    </td>
                    {canWrite && (
                      <td className="px-5 py-3.5 text-right">
                        <div className="flex justify-end">
                          {!isSelf && (
                            <RowActions
                              actions={
                                [
                                  {
                                    label: "Edit account",
                                    tone: "info",
                                    onClick: () => openEdit(admin),
                                  },
                                  {
                                    label: "Reset password",
                                    tone: "warning",
                                    onClick: () => setResetTarget(admin),
                                  },
                                ] as RowAction[]
                              }
                            />
                          )}
                        </div>
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </ConsoleSection>

      <Modal
        isOpen={editing !== null}
        onClose={() => setEditing(null)}
        className="max-w-md p-6"
      >
        {editing && (
          <form onSubmit={saveEdit} className="space-y-5">
            <div>
              <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
                Edit {editing.email}
              </h3>
              <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                Changes take effect on the account&apos;s next request.
              </p>
            </div>
            <div>
              <Label>Role</Label>
              <select
                className={selectClasses}
                value={editRole}
                onChange={(e) => setEditRole(e.target.value)}
              >
                {roles.map((role) => (
                  <option key={role.id} value={role.name}>
                    {role.name} — {role.description}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <Label>Status</Label>
              <select
                className={selectClasses}
                value={editStatus}
                onChange={(e) => setEditStatus(e.target.value)}
              >
                <option value="active">active</option>
                <option value="disabled">disabled</option>
              </select>
            </div>
            {saveError && (
              <p className="text-sm text-error-500" role="alert">
                {saveError}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <button
                type="button"
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300"
                onClick={() => setEditing(null)}
              >
                Cancel
              </button>
              <Button size="sm" disabled={saving}>
                {saving ? "Saving..." : "Save changes"}
              </Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        className="max-w-md p-6"
      >
        <form onSubmit={handleCreate} className="space-y-5">
          <div>
            <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
              Add admin
            </h3>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Creates a console account. The new admin must enroll MFA on their
              first login.
            </p>
          </div>
          <div>
            <Label>Email</Label>
            <input
              type="email"
              required
              value={createEmail}
              onChange={(e) => setCreateEmail(e.target.value)}
              placeholder="admin@example.com"
              className={inputClasses}
            />
          </div>
          <div>
            <Label>Password</Label>
            <input
              type="password"
              required
              minLength={12}
              value={createPassword}
              onChange={(e) => setCreatePassword(e.target.value)}
              placeholder="At least 12 characters"
              className={inputClasses}
            />
          </div>
          <div>
            <Label>Role</Label>
            <select
              className={selectClasses}
              value={createRole}
              onChange={(e) => setCreateRole(e.target.value)}
            >
              {roles.map((role) => (
                <option key={role.id} value={role.name}>
                  {role.name} — {role.description}
                </option>
              ))}
            </select>
          </div>
          {createError && (
            <p className="text-sm text-error-500" role="alert">
              {createError}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300"
              onClick={() => setCreateOpen(false)}
            >
              Cancel
            </button>
            <Button size="sm" disabled={creating}>
              {creating ? "Creating..." : "Create admin"}
            </Button>
          </div>
        </form>
      </Modal>

      <ResetPasswordModal
        open={resetTarget !== null}
        userEmail={resetTarget?.email ?? ""}
        onClose={() => setResetTarget(null)}
        onReset={async (password) => {
          if (!resetTarget) return { tempPassword: null };
          const response = await api.resetAdminPassword(
            resetTarget.id,
            password || undefined
          );
          await load();
          toast.success("Admin password reset", resetTarget.email);
          return { tempPassword: response.temp_password ?? null };
        }}
      />
    </div>
  );
}
