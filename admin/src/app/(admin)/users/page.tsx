"use client";
import ConsoleSection, {
  EmptyNote,
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import StatusBadge from "@/components/console/StatusBadge";
import Label from "@/components/form/Label";
import Pagination from "@/components/tables/Pagination";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { TableSkeleton } from "@/components/common/Skeleton";
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

  function handleSearch(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPage(1);
    setSearch(searchInput.trim());
  }

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

  const totalPages = result
    ? Math.max(1, Math.ceil(result.total / result.page_size))
    : 1;

  return (
    <div>
      <PageTitle
        title="Users"
        description="Licensed end users of the product."
      />
      <ConsoleSection
        title="User Directory"
        description={
          result ? `${result.total} user(s) total` : "Loading users"
        }
        actions={
          <div className="flex items-center gap-2">
            <form onSubmit={handleSearch} className="flex items-center gap-2">
              <input
                type="search"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                placeholder="Search by email..."
                className="h-10 w-56 rounded-lg border border-gray-300 bg-transparent px-3.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
              />
              <button
                type="submit"
                className="h-10 rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600"
              >
                Search
              </button>
            </form>
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
          <EmptyNote message="No users found." />
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
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {result.items.map((user) => (
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
                  </tr>
                ))}
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
    </div>
  );
}
