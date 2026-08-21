"use client";
import ConsoleSection, {
  ErrorNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import EmptyState from "@/components/console/EmptyState";
import StatusBadge from "@/components/console/StatusBadge";
import RowActions, { type RowAction } from "@/components/console/RowActions";
import Label from "@/components/form/Label";
import Pagination from "@/components/tables/Pagination";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { TableSkeleton } from "@/components/common/Skeleton";
import ConfirmModal from "@/components/console/ConfirmModal";
import ResetPasswordModal from "@/components/console/ResetPasswordModal";
import ExportCsvButton from "@/components/common/ExportCsvButton";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useToast } from "@/context/ToastContext";
import { api, formatDateTime } from "@/lib/api";
import type { PageResult, ConsoleUser } from "@/lib/types";
import Link from "next/link";
import React, { useCallback, useEffect, useState } from "react";

const inputClasses =
  "h-11 w-full rounded-lg border appearance-none px-4 py-2.5 text-sm shadow-theme-xs focus:outline-hidden focus:ring-3 dark:bg-gray-900 dark:text-white/90 bg-transparent text-gray-800 border-gray-300 focus:border-brand-300 focus:ring-brand-500/10 dark:border-gray-700 dark:focus:border-brand-800 placeholder:text-gray-400";

export default function UsersPage() {
  const { hasPermission } = useAdminIdentity();
  const toast = useToast();
  const [result, setResult] = useState<PageResult<ConsoleUser> | null>(null);
  const [page, setPage] = useState(1);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const [createOpen, setCreateOpen] = useState(false);
  const [createEmail, setCreateEmail] = useState("");
  const [createPassword, setCreatePassword] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const [statusTarget, setStatusTarget] = useState<ConsoleUser | null>(null);
  const [statusBusy, setStatusBusy] = useState(false);
  const [statusError, setStatusError] = useState<string | null>(null);

  const [revokeTarget, setRevokeTarget] = useState<ConsoleUser | null>(null);
  const [revokeBusy, setRevokeBusy] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const [resetTarget, setResetTarget] = useState<ConsoleUser | null>(null);

  // Bulk selection
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkConfirm, setBulkConfirm] = useState<
    "enable" | "disable" | "revoke" | null
  >(null);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [bulkError, setBulkError] = useState<string | null>(null);

  const canWrite = hasPermission("users.write");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.users(page, search, statusFilter);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load users");
    } finally {
      setLoading(false);
    }
  }, [page, search, statusFilter]);

  useEffect(() => {
    load();
  }, [load]);

  function updateStatusFilter(next: string) {
    setStatusFilter(next);
    setPage(1);
  }

  // Live search: debounce keystrokes, then refetch from page 1.
  useEffect(() => {
    const timer = setTimeout(() => {
      setPage(1);
      setSearch(searchInput.trim());
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  function openCreate() {
    setCreateEmail("");
    setCreatePassword("");
    setCreateError(null);
    setCreateOpen(true);
  }

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreating(true);
    setCreateError(null);
    try {
      await api.createUser(createEmail.trim(), createPassword);
      setCreateOpen(false);
      setPage(1);
      await load();
      toast.success("User created", createEmail.trim());
    } catch (err) {
      const message = err instanceof Error ? err.message : "Create failed";
      setCreateError(message);
      toast.error("Create failed", message);
    } finally {
      setCreating(false);
    }
  }

  async function handleStatusChange() {
    if (!statusTarget) return;
    setStatusBusy(true);
    setStatusError(null);
    const next = statusTarget.status === "active" ? "disabled" : "active";
    try {
      if (statusTarget.status === "banned") {
        await api.unbanUser(statusTarget.id);
        setStatusTarget(null);
        await load();
        toast.success("User unbanned", statusTarget.email);
      } else {
        await api.setUserStatus(statusTarget.id, next);
        setStatusTarget(null);
        await load();
        toast.success(next === "disabled" ? "User disabled" : "User enabled", statusTarget.email);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Update failed";
      setStatusError(message);
      toast.error("Update failed", message);
    } finally {
      setStatusBusy(false);
    }
  }

  async function handleRevoke() {
    if (!revokeTarget) return;
    setRevokeBusy(true);
    setRevokeError(null);
    try {
      await api.revokeUserSessions(revokeTarget.id);
      setRevokeTarget(null);
      await load();
      toast.success("Sessions revoked", revokeTarget.email);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Revoke failed";
      setRevokeError(message);
      toast.error("Revoke failed", message);
    } finally {
      setRevokeBusy(false);
    }
  }

  const totalPages = result
    ? Math.max(1, Math.ceil(result.total / result.page_size))
    : 1;

  const items = result?.items ?? [];
  const activeCount = items.filter((u) => u.status === "active").length;
  const bannedCount = items.filter((u) => u.status === "banned").length;
  const disabledCount = items.length - activeCount - bannedCount;

  const allOnPageSelected =
    items.length > 0 && items.every((u) => selected.has(u.id));

  function toggleAll() {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allOnPageSelected) {
        items.forEach((u) => next.delete(u.id));
      } else {
        items.forEach((u) => next.add(u.id));
      }
      return next;
    });
  }

  function toggleOne(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  async function handleBulk() {
    if (!bulkConfirm || selected.size === 0) return;
    const ids = Array.from(selected);
    setBulkBusy(true);
    setBulkError(null);
    try {
      if (bulkConfirm === "revoke") {
        const response = await api.bulkRevokeUserSessions(ids);
        toast.success(
          "Sessions revoked",
          `${response.revoked} session(s) across ${ids.length} user(s)`
        );
      } else {
        const status = bulkConfirm === "disable" ? "disabled" : "active";
        const response = await api.bulkSetUserStatus(ids, status);
        toast.success(
          status === "disabled" ? "Users disabled" : "Users enabled",
          `${response.updated} user(s) updated`
        );
      }
      setBulkConfirm(null);
      setSelected(new Set());
      await load();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Bulk action failed";
      setBulkError(message);
      toast.error("Bulk action failed", message);
    } finally {
      setBulkBusy(false);
    }
  }

  return (
    <div>
      <PageTitle
        title="Users"
        description="Licensed end users of the product."
      />
      {/* Bulk action bar */}
      {selected.size > 0 && (
        <div className="mb-4 flex flex-wrap items-center gap-2 rounded-xl border border-brand-200 bg-brand-50 px-4 py-3 dark:border-brand-500/30 dark:bg-brand-500/10">
          <span className="text-sm font-medium text-brand-700 dark:text-brand-300">
            {selected.size} selected
          </span>
          {canWrite && (
            <>
              <Button size="sm" variant="success" onClick={() => setBulkConfirm("enable")}>
                Enable
              </Button>
              <Button size="sm" variant="danger" onClick={() => setBulkConfirm("disable")}>
                Disable
              </Button>
              <Button size="sm" variant="warning" onClick={() => setBulkConfirm("revoke")}>
                Revoke sessions
              </Button>
            </>
          )}
          <button
            type="button"
            onClick={() => setSelected(new Set())}
            className="ml-auto rounded-lg px-2 py-1 text-sm font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
          >
            Clear
          </button>
        </div>
      )}
      {/* Page snapshot summary */}
      {result && (
        <div className="mb-4 flex flex-wrap gap-2">
          <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
            {result.total} total{statusFilter ? ` (${statusFilter})` : ""}
          </span>
          <span className="rounded-full bg-success-50 px-3 py-1 text-xs font-medium text-success-600 dark:bg-success-500/10 dark:text-success-400">
            {activeCount} active (this page)
          </span>
          <span className="rounded-full bg-warning-50 px-3 py-1 text-xs font-medium text-warning-600 dark:bg-warning-500/10 dark:text-warning-400">
            {disabledCount} disabled (this page)
          </span>
          <span className="rounded-full bg-error-50 px-3 py-1 text-xs font-medium text-error-600 dark:bg-error-500/10 dark:text-error-400">
            {bannedCount} banned (this page)
          </span>
        </div>
      )}
      <ConsoleSection
        title="User Directory"
        description={
          result ? `${result.total} user(s) total` : "Loading users"
        }
        actions={
          <div className="flex items-center gap-2">
            <input
              type="search"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="Type to filter by email..."
              className="h-10 w-56 rounded-lg border border-gray-300 bg-transparent px-3.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            />
            <select
              value={statusFilter}
              onChange={(e) => updateStatusFilter(e.target.value)}
              aria-label="Filter users by status"
              className="h-10 rounded-lg border border-gray-300 bg-transparent px-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            >
              <option value="">All statuses</option>
              <option value="active">Active</option>
              <option value="disabled">Disabled</option>
              <option value="banned">Banned</option>
            </select>
            {statusFilter !== "" && (
              <button
                type="button"
                onClick={() => updateStatusFilter("")}
                className="rounded-lg px-2 py-1 text-sm font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
              >
                Clear
              </button>
            )}
            <ExportCsvButton
              filename="users.csv"
              headers={["email", "status", "licenses", "devices", "active_sessions", "last_login_at", "created_at"]}
              rows={items.map((u) => [
                u.email,
                u.status,
                u.license_count,
                u.device_count,
                u.active_session_count,
                formatDateTime(u.last_login_at),
                formatDateTime(u.created_at),
              ])}
            />
            {canWrite && (
              <Button size="sm" onClick={openCreate}>
                Add user
              </Button>
            )}
          </div>
        }
      >
        {loading && !error ? (
          <TableSkeleton rows={6} cols={7} />
        ) : error ? (
          <ErrorNote message={error} />
        ) : !result || result.items.length === 0 ? (
          <EmptyState
            title="No users found"
            message={
              statusFilter === "banned"
                ? "No banned users match the current filters."
                : "Users appear here when they register, or add one manually."
            }
          />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
                  <th className="w-12 px-5 py-3">
                    <input
                      type="checkbox"
                      checked={allOnPageSelected}
                      onChange={toggleAll}
                      aria-label="Select all users on this page"
                      className="h-4 w-4 rounded border-gray-300 accent-brand-500 dark:border-gray-700"
                    />
                  </th>
                  <th className="px-5 py-3 font-medium">Email</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Licenses</th>
                  <th className="px-5 py-3 font-medium">Devices</th>
                  <th className="px-5 py-3 font-medium">Active Sessions</th>
                  <th className="px-5 py-3 font-medium">Last Login</th>
                  <th className="px-5 py-3 font-medium">Created</th>
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {result.items.map((user) => {
                  const actions: RowAction[] = [
                    {
                      label: "View profile",
                      href: `/users/${user.id}`,
                      tone: "info",
                    },
                  ];
                  if (canWrite) {
                    if (user.status === "banned") {
                      actions.push({
                        label: "Unban user",
                        tone: "success",
                        onClick: () => {
                          setStatusError(null);
                          setStatusTarget(user);
                        },
                      });
                    } else {
                      actions.push({
                        label:
                          user.status === "active"
                            ? "Disable user"
                            : "Enable user",
                        tone: user.status === "active" ? "danger" : "success",
                        onClick: () => {
                          setStatusError(null);
                          setStatusTarget(user);
                        },
                      });
                    }
                    actions.push(
                      {
                        label: "Revoke sessions",
                        tone: "danger",
                        disabled: user.active_session_count === 0,
                        onClick: () => {
                          setRevokeError(null);
                          setRevokeTarget(user);
                        },
                      },
                      {
                        label: "Reset password",
                        tone: "warning",
                        onClick: () => setResetTarget(user),
                      }
                    );
                  }
                  return (
                    <tr
                      key={user.id}
                      className={`hover:bg-gray-50 dark:hover:bg-white/[0.02] ${
                        selected.has(user.id) ? "bg-brand-50/60 dark:bg-brand-500/[0.06]" : ""
                      }`}
                    >
                      <td className="px-5 py-3.5">
                        <input
                          type="checkbox"
                          checked={selected.has(user.id)}
                          onChange={() => toggleOne(user.id)}
                          aria-label={`Select ${user.email}`}
                          className="h-4 w-4 rounded border-gray-300 accent-brand-500 dark:border-gray-700"
                        />
                      </td>
                      <td className="px-5 py-3.5">
                        <Link
                          href={`/users/${user.id}`}
                          className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400"
                        >
                          {user.email}
                        </Link>
                      </td>
                      <td className="px-5 py-3.5">
                        <StatusBadge status={user.status} />
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {user.license_count}
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {user.device_count}
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {user.active_session_count}
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {formatDateTime(user.last_login_at)}
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {formatDateTime(user.created_at)}
                      </td>
                      <td className="px-5 py-3.5 text-right">
                        <div className="flex justify-end">
                          <RowActions actions={actions} />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            <div className="flex justify-end border-t border-gray-200 px-5 py-4 dark:border-gray-800">
              <Pagination
                currentPage={result.page}
                totalPages={totalPages}
                onPageChange={(next) => setPage(Math.min(Math.max(next, 1), totalPages))}
              />
            </div>
          </>
        )}
      </ConsoleSection>

      <Modal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        className="max-w-md p-6"
      >
        <form onSubmit={handleCreate} className="space-y-5">
          <div>
            <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
              Add user
            </h3>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Creates an end-user account. The user can log in immediately with
              this password.
            </p>
          </div>
          <div>
            <Label>Email</Label>
            <input
              type="email"
              required
              value={createEmail}
              onChange={(e) => setCreateEmail(e.target.value)}
              placeholder="user@example.com"
              className={inputClasses}
            />
          </div>
          <div>
            <Label>Password</Label>
            <input
              type="password"
              required
              minLength={10}
              value={createPassword}
              onChange={(e) => setCreatePassword(e.target.value)}
              placeholder="At least 10 characters"
              className={inputClasses}
            />
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
              {creating ? "Creating..." : "Create user"}
            </Button>
          </div>
        </form>
      </Modal>

      <ConfirmModal
        isOpen={statusTarget !== null}
        title={
          statusTarget?.status === "banned"
            ? "Unban user"
            : statusTarget?.status === "active"
              ? "Disable user"
              : "Enable user"
        }
        message={
          statusTarget
            ? statusTarget.status === "banned"
              ? `Unban ${statusTarget.email}? The ban details will be cleared and the account returns to active.`
              : statusTarget.status === "active"
                ? `Disable ${statusTarget.email}? Their licenses stay intact but the account can no longer authenticate.`
                : `Enable ${statusTarget.email}? The account can authenticate again.`
            : ""
        }
        confirmLabel={
          statusTarget?.status === "banned"
            ? "Unban"
            : statusTarget?.status === "active"
              ? "Disable"
              : "Enable"
        }
        busy={statusBusy}
        error={statusError}
        onConfirm={handleStatusChange}
        onClose={() => setStatusTarget(null)}
      />

      <ConfirmModal
        isOpen={revokeTarget !== null}
        title="Revoke sessions"
        message={
          revokeTarget
            ? `Sign out all sessions for ${revokeTarget.email}?`
            : ""
        }
        confirmLabel="Revoke"
        busy={revokeBusy}
        error={revokeError}
        onConfirm={handleRevoke}
        onClose={() => setRevokeTarget(null)}
      />

      <ResetPasswordModal
        open={resetTarget !== null}
        userEmail={resetTarget?.email ?? ""}
        onClose={() => setResetTarget(null)}
        onReset={async (password) => {
          if (!resetTarget) return { tempPassword: null };
          const response = await api.resetUserPassword(
            resetTarget.id,
            password || undefined
          );
          await load();
          toast.success("Password reset", resetTarget.email);
          return { tempPassword: response.temp_password ?? null };
        }}
      />

      <ConfirmModal
        isOpen={bulkConfirm !== null}
        title={
          bulkConfirm === "revoke"
            ? "Revoke sessions"
            : bulkConfirm === "disable"
              ? "Disable users"
              : "Enable users"
        }
        message={
          bulkConfirm === "revoke"
            ? `Sign out all sessions for the ${selected.size} selected user(s)?`
            : bulkConfirm === "disable"
              ? `Disable the ${selected.size} selected user(s)? They will no longer be able to authenticate until re-enabled.`
              : `Enable the ${selected.size} selected user(s)?`
        }
        confirmLabel={
          bulkConfirm === "revoke"
            ? "Revoke"
            : bulkConfirm === "disable"
              ? "Disable"
              : "Enable"
        }
        busy={bulkBusy}
        error={bulkError}
        onConfirm={handleBulk}
        onClose={() => setBulkConfirm(null)}
      />
    </div>
  );
}
