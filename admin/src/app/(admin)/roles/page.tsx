"use client";
import ConsoleSection, {
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import EmptyState from "@/components/console/EmptyState";
import ConfirmModal from "@/components/console/ConfirmModal";
import RowActions, { type RowAction } from "@/components/console/RowActions";
import Label from "@/components/form/Label";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useToast } from "@/context/ToastContext";
import { api } from "@/lib/api";
import { PERMISSION_GROUPS } from "@/lib/permissions";
import type { AdminRole, RoleMember } from "@/lib/types";
import { GroupIcon } from "@/icons";
import React, { useCallback, useEffect, useMemo, useState } from "react";

const inputClasses =
  "h-11 w-full rounded-lg border appearance-none px-4 py-2.5 text-sm shadow-theme-xs focus:outline-hidden focus:ring-3 dark:bg-gray-900 dark:text-white/90 bg-transparent text-gray-800 border-gray-300 focus:border-brand-300 focus:ring-brand-500/10 dark:border-gray-700 dark:focus:border-brand-800 placeholder:text-gray-400";

const TOTAL_PERMISSIONS = PERMISSION_GROUPS.reduce(
  (total, group) => total + group.permissions.length,
  0
);

type EditorState =
  | { mode: "create" }
  | { mode: "edit"; role: AdminRole }
  | null;

export default function RolesPage() {
  const { hasPermission } = useAdminIdentity();
  const toast = useToast();
  const [roles, setRoles] = useState<AdminRole[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState("");

  const [editor, setEditor] = useState<EditorState>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [permissions, setPermissions] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<AdminRole | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Expandable member panel per role.
  const [expandedRole, setExpandedRole] = useState<string | null>(null);
  const [membersByRole, setMembersByRole] = useState<Record<string, RoleMember[]>>({});
  const [membersLoading, setMembersLoading] = useState<string | null>(null);

  const canWrite = hasPermission("admins.write");

  const toggleMembers = useCallback(
    async (role: AdminRole) => {
      if (expandedRole === role.id) {
        setExpandedRole(null);
        return;
      }
      setExpandedRole(role.id);
      if (!membersByRole[role.id] && role.member_count > 0) {
        setMembersLoading(role.id);
        try {
          const response = await api.roleMembers(role.id);
          setMembersByRole((prev) => ({ ...prev, [role.id]: response.items ?? [] }));
        } catch {
          setMembersByRole((prev) => ({ ...prev, [role.id]: [] }));
        } finally {
          setMembersLoading(null);
        }
      }
    },
    [expandedRole, membersByRole]
  );

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.roles();
      setRoles(response.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load roles");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return roles;
    return roles.filter(
      (role) =>
        role.name.toLowerCase().includes(q) ||
        role.description.toLowerCase().includes(q)
    );
  }, [roles, filter]);

  function openCreate() {
    setName("");
    setDescription("");
    setPermissions([]);
    setSaveError(null);
    setEditor({ mode: "create" });
  }

  function openEdit(role: AdminRole) {
    setName(role.name);
    setDescription(role.description);
    setPermissions([...role.permissions]);
    setSaveError(null);
    setEditor({ mode: "edit", role });
  }

  function togglePermission(permissionId: string) {
    setPermissions((prev) =>
      prev.includes(permissionId)
        ? prev.filter((id) => id !== permissionId)
        : [...prev, permissionId]
    );
  }

  async function handleSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editor) return;
    setSaving(true);
    setSaveError(null);
    try {
      if (editor.mode === "create") {
        await api.createRole(name.trim(), description.trim(), permissions);
        toast.success("Role created", name.trim());
      } else {
        await api.updateRole(editor.role.id, description.trim(), permissions);
        toast.success("Role updated", editor.role.name);
      }
      setEditor(null);
      await load();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Save failed";
      setSaveError(message);
      toast.error("Save failed", message);
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleteBusy(true);
    setDeleteError(null);
    try {
      await api.deleteRole(deleteTarget.id);
      setDeleteTarget(null);
      await load();
      toast.success("Role deleted", deleteTarget.name);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Delete failed";
      setDeleteError(message);
      toast.error("Delete failed", message);
    } finally {
      setDeleteBusy(false);
    }
  }

  const customCount = roles.filter((r) => !r.built_in).length;

  return (
    <div>
      <PageTitle
        title="Roles & Permissions"
        description="Define who can do what in the console."
        actions={
          canWrite ? (
            <Button size="sm" onClick={openCreate}>
              Create role
            </Button>
          ) : undefined
        }
      />
      {!loading && !error && (
        <div className="mb-4 flex flex-wrap gap-2">
          <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
            {roles.length} total
          </span>
          <span className="rounded-full bg-brand-50 px-3 py-1 text-xs font-medium text-brand-600 dark:bg-brand-500/10 dark:text-brand-400">
            {roles.length - customCount} built-in
          </span>
          <span className="rounded-full bg-success-50 px-3 py-1 text-xs font-medium text-success-600 dark:bg-success-500/10 dark:text-success-400">
            {customCount} custom
          </span>
        </div>
      )}
      <ConsoleSection
        title="RBAC Roles"
        description={`${roles.length} role(s)`}
        actions={
          <input
            type="search"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Type to filter..."
            className="h-10 w-56 rounded-lg border border-gray-300 bg-transparent px-3.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
          />
        }
      >
        {loading && !error ? (
          <LoadingNote />
        ) : error ? (
          <ErrorNote message={error} />
        ) : roles.length === 0 ? (
          <EmptyState
            icon={<GroupIcon />}
            title="No roles found"
            message="Console access roles will appear here."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={<GroupIcon />}
            title="No matching roles"
            message={`Nothing matches “${filter}”.`}
          />
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 dark:border-gray-800">
              <tr className="text-xs uppercase text-gray-400">
                <th className="px-5 py-3 font-medium">Name</th>
                <th className="px-5 py-3 font-medium">Description</th>
                <th className="px-5 py-3 font-medium">Permissions</th>
                <th className="px-5 py-3 font-medium">Members</th>
                <th className="px-5 py-3 font-medium">Type</th>
                {canWrite && <th className="px-5 py-3 font-medium"></th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {filtered.map((role) => {
                const expanded = expandedRole === role.id;
                const members = membersByRole[role.id] ?? [];
                const membersBusy = membersLoading === role.id;
                const inUse = role.member_count > 0;
                return (
                  <React.Fragment key={role.id}>
                    <tr
                      className={`hover:bg-gray-50 dark:hover:bg-white/[0.02] ${
                        expanded ? "bg-gray-50 dark:bg-white/[0.02]" : ""
                      }`}
                    >
                      <td className="px-5 py-3.5">
                        <div className="flex items-center gap-2">
                          <button
                            type="button"
                            onClick={() => toggleMembers(role)}
                            aria-label={
                              expanded
                                ? `Hide members of ${role.name}`
                                : `Show members of ${role.name}`
                            }
                            className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-gray-400 transition-transform hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-white/5 dark:hover:text-white ${
                              expanded ? "rotate-90" : ""
                            }`}
                          >
                            <svg
                              width="12"
                              height="12"
                              viewBox="0 0 16 16"
                              fill="currentColor"
                            >
                              <path d="M6 3.5 11 8l-5 4.5V3.5z" />
                            </svg>
                          </button>
                          <span className="font-medium text-gray-800 dark:text-white/90">
                            {role.name}
                          </span>
                          {role.built_in && role.name === "owner" && (
                            <span
                              title="Owner is the super-admin role: full access and protected from deletion."
                              className="inline-flex items-center gap-1 rounded-full bg-warning-50 px-2 py-0.5 text-[11px] font-semibold text-warning-600 dark:bg-warning-500/10 dark:text-warning-400"
                            >
                              <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor">
                                <path d="M8 1.5 14 4v4c0 3.4-2.4 5.9-6 6.5C4.4 13.9 2 11.4 2 8V4l6-2.5z" />
                              </svg>
                              Super admin
                            </span>
                          )}
                          {role.built_in && role.name !== "owner" && (
                            <span
                              title="Built-in roles are fixed and protected from modification and deletion."
                              className="inline-flex items-center gap-1 rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-500 dark:bg-white/[0.05] dark:text-gray-400"
                            >
                              <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor">
                                <path d="M8 1.5 14 4v4c0 3.4-2.4 5.9-6 6.5C4.4 13.9 2 11.4 2 8V4l6-2.5z" />
                              </svg>
                              Protected
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {role.description || "—"}
                      </td>
                      <td className="px-5 py-3.5">
                        <span
                          title={`${role.permissions.length} of ${TOTAL_PERMISSIONS} assignable permissions`}
                          className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${
                            role.permissions.length === TOTAL_PERMISSIONS
                              ? "bg-success-50 text-success-600 dark:bg-success-500/[0.1] dark:text-success-400"
                              : "bg-gray-100 text-gray-700 dark:bg-white/[0.05] dark:text-gray-300"
                          }`}
                        >
                          {role.permissions.length} / {TOTAL_PERMISSIONS}
                        </span>
                      </td>
                      <td className="px-5 py-3.5">
                        <button
                          type="button"
                          onClick={() => toggleMembers(role)}
                          disabled={role.member_count === 0}
                          title={
                            role.member_count === 0
                              ? "No admins assigned to this role"
                              : expanded
                                ? "Hide members"
                                : "Show members"
                          }
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium transition-colors ${
                            role.member_count === 0
                              ? "cursor-default bg-gray-100 text-gray-400 dark:bg-white/[0.05] dark:text-gray-500"
                              : expanded
                                ? "bg-brand-50 text-brand-600 dark:bg-brand-500/[0.1] dark:text-brand-400"
                                : "bg-brand-50 text-brand-600 hover:bg-brand-100 dark:bg-brand-500/[0.1] dark:text-brand-400 dark:hover:bg-brand-500/[0.2]"
                          }`}
                        >
                          <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor">
                            <path d="M8 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zm0 1c-2.8 0-5 1.4-5 3.2V14h10v-1.8C13 10.4 10.8 9 8 9z" />
                          </svg>
                          {role.member_count} member{role.member_count === 1 ? "" : "s"}
                        </button>
                      </td>
                      <td className="px-5 py-3.5">
                        <span
                          className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${
                            role.built_in
                              ? "bg-brand-50 text-brand-600 dark:bg-brand-500/[0.1] dark:text-brand-400"
                              : "bg-success-50 text-success-600 dark:bg-success-500/[0.1] dark:text-success-400"
                          }`}
                        >
                          {role.built_in ? "built-in" : "custom"}
                        </span>
                      </td>
                      {canWrite && (
                        <td className="px-5 py-3.5 text-right">
                          <div className="flex justify-end">
                            {!role.built_in && (
                              <RowActions
                                actions={
                                  [
                                    {
                                      label: "Edit role",
                                      tone: "info",
                                      onClick: () => openEdit(role),
                                    },
                                    {
                                      label: inUse
                                        ? "Delete (assign members elsewhere first)"
                                        : "Delete",
                                      tone: "danger",
                                      disabled: inUse,
                                      onClick: () => {
                                        setDeleteError(null);
                                        setDeleteTarget(role);
                                      },
                                    },
                                  ] as RowAction[]
                                }
                              />
                            )}
                          </div>
                        </td>
                      )}
                    </tr>
                    {expanded && (
                      <tr className="border-t border-gray-100 bg-gray-50/60 dark:border-gray-800/60 dark:bg-white/[0.015]">
                        <td colSpan={canWrite ? 6 : 5} className="px-5 py-4">
                          <div className="space-y-2">
                            <p className="text-xs font-semibold uppercase text-gray-400">
                              Admins assigned to “{role.name}”
                            </p>
                            {membersBusy ? (
                              <p className="text-sm text-gray-400">Loading members…</p>
                            ) : members.length === 0 ? (
                              <p className="text-sm text-gray-400">
                                No members — this role has no admins assigned yet.
                              </p>
                            ) : (
                              <div className="flex flex-wrap gap-2">
                                {members.map((member) => (
                                  <span
                                    key={member.id}
                                    className="inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-sm dark:border-gray-700 dark:bg-gray-900"
                                  >
                                    <span className="font-medium text-gray-800 dark:text-white/90">
                                      {member.email}
                                    </span>
                                    <span
                                      className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
                                        member.status === "active"
                                          ? "bg-success-50 text-success-600 dark:bg-success-500/10 dark:text-success-400"
                                          : member.status === "disabled"
                                            ? "bg-error-50 text-error-600 dark:bg-error-500/10 dark:text-error-400"
                                            : "bg-warning-50 text-warning-600 dark:bg-warning-500/10 dark:text-warning-400"
                                      }`}
                                    >
                                      {member.status}
                                    </span>
                                    <span
                                      title={
                                        member.mfa_enrolled
                                          ? "MFA enrolled"
                                          : "MFA not enrolled — first login will require setup"
                                      }
                                      className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
                                        member.mfa_enrolled
                                          ? "bg-success-50 text-success-600 dark:bg-success-500/10 dark:text-success-400"
                                          : "bg-warning-50 text-warning-600 dark:bg-warning-500/10 dark:text-warning-400"
                                      }`}
                                    >
                                      {member.mfa_enrolled ? "MFA" : "no MFA"}
                                    </span>
                                  </span>
                                ))}
                              </div>
                            )}
                          </div>
                        </td>
                      </tr>
                    )}
                  </React.Fragment>
                );
              })}
            </tbody>
          </table>
        )}
      </ConsoleSection>

      <Modal
        isOpen={editor !== null}
        onClose={() => !saving && setEditor(null)}
        className="max-w-lg p-6"
      >
        {editor && (
          <form onSubmit={handleSave} className="space-y-5">
            <div>
              <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
                {editor.mode === "create" ? "Create role" : `Edit ${editor.role.name}`}
              </h3>
              <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {editor.mode === "create"
                  ? "Built-in roles (owner, viewer) are fixed; create a custom role to tailor access."
                  : "Built-in roles cannot be edited; this is a custom role."}
              </p>
            </div>
            {editor.mode === "create" && (
              <div>
                <Label>Name</Label>
                <input
                  type="text"
                  required
                  pattern="[a-z][a-z0-9_-]{0,31}"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="support-agent"
                  className={inputClasses}
                />
                <p className="mt-1 text-xs text-gray-400">
                  Lowercase letters, digits, dashes or underscores (max 32).
                </p>
              </div>
            )}
            <div>
              <Label>Description</Label>
              <input
                type="text"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="What is this role for?"
                maxLength={200}
                className={inputClasses}
              />
            </div>
            <div>
              <Label>Permissions</Label>
              <div className="max-h-72 space-y-4 overflow-y-auto rounded-xl border border-gray-200 p-4 dark:border-gray-800">
                {PERMISSION_GROUPS.map((group) => (
                  <div key={group.resource}>
                    <p className="mb-2 text-xs font-semibold uppercase text-gray-400">
                      {group.resource}
                    </p>
                    <div className="space-y-2">
                      {group.permissions.map((permission) => {
                        const checked = permissions.includes(permission.id);
                        return (
                          <label
                            key={permission.id}
                            className="flex cursor-pointer items-start gap-2.5 text-sm"
                          >
                            <input
                              type="checkbox"
                              checked={checked}
                              onChange={() => togglePermission(permission.id)}
                              className="mt-0.5 h-4 w-4 rounded border-gray-300 accent-brand-500 dark:border-gray-700"
                            />
                            <span
                              className={
                                checked
                                  ? "text-gray-800 dark:text-white/90"
                                  : "text-gray-500 dark:text-gray-400"
                              }
                            >
                              {permission.label}
                              <span className="ml-1.5 font-mono text-xs text-gray-400">
                                {permission.id}
                              </span>
                            </span>
                          </label>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
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
                onClick={() => setEditor(null)}
              >
                Cancel
              </button>
              <Button size="sm" disabled={saving}>
                {saving
                  ? "Saving..."
                  : editor.mode === "create"
                    ? "Create role"
                    : "Save changes"}
              </Button>
            </div>
          </form>
        )}
      </Modal>

      <ConfirmModal
        isOpen={deleteTarget !== null}
        title="Delete role"
        message={
          deleteTarget
            ? `Delete the custom role “${deleteTarget.name}”? Admins assigned to it will lose access until moved to another role. This cannot be undone.`
            : ""
        }
        confirmLabel="Delete"
        busy={deleteBusy}
        error={deleteError}
        onConfirm={handleDelete}
        onClose={() => setDeleteTarget(null)}
      />
    </div>
  );
}
