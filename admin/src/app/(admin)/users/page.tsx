"use client";
import ConsoleSection, {
  ErrorNote,
  LoadingNote,
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

  const canWrite = hasPermission("users.write");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.users(page, search);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load users");
    } finally {
      setLoading(false);
    }
  }, [page, search]);

  useEffect(() => {
    load();
  }, [load]);

  // Live search: debounce keystrokes, then refetch from page 1.
  useEffect(() => {
    const timer = setTimeout(() => {
      setPage(1);
      setSearch(searchInput.trim());
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Open the create modal when arriving via /users?create=1 (sub sidebar).
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("create") === "1") {
      window.history.replaceState({}, "", "/users");
      openCreate();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
      await api.setUserStatus(statusTarget.id, next);
      setStatusTarget(null);
      await load();
      toast.success(next === "disabled" ? "User disabled" : "User enabled", statusTarget.email);
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
  const disabledCount = items.length - activeCount;

  return (
    <div>
      <PageTitle
        title="Users"
        description="Licensed end users of the product."
      />
      {/* Page snapshot summary */}
      {result && (
        <div className="mb-4 flex flex-wrap gap-2">
          <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
            {result.total} total
          </span>
          <span className="rounded-full bg-success-50 px-3 py-1 text-xs font-medium text-success-600 dark:bg-success-500/10 dark:text-success-400">
            {activeCount} active (this page)
          </span>
          <span className="rounded-full bg-warning-50 px-3 py-1 text-xs font-medium text-warning-600 dark:bg-warning-500/10 dark:text-warning-400">
            {disabledCount} disabled (this page)
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
            message="Users appear here when they register, or add one manually."
          />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
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
                    actions.push(
                      {
                        label:
                          user.status === "active"
                            ? "Disable user"
                            : "Enable user",
                        tone: user.status === "active" ? "danger" : "success",
                        onClick: () => {
                          setStatusError(null);
                          setStatusTarget(user);
                        },
                      },
                      {
                        label: "Revoke sessions",
                        tone: "danger",
                        disabled: user.active_session_count === 0,
                        onClick: () => {
                          setRevokeError(null);
                          setRevokeTarget(user);
                        },
                      }
                    );
                  }
                  return (
                    <tr
                      key={user.id}
                      className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                    >
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
          statusTarget?.status === "active" ? "Disable user" : "Enable user"
        }
        message={
          statusTarget
            ? statusTarget.status === "active"
              ? `Disable ${statusTarget.email}? Their licenses stay intact but the account can no longer authenticate.`
              : `Enable ${statusTarget.email}? The account can authenticate again.`
            : ""
        }
        confirmLabel={statusTarget?.status === "active" ? "Disable" : "Enable"}
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
    </div>
  );
}
