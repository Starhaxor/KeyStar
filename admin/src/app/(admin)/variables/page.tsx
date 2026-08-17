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
import type { Variable } from "@/lib/types";
import { ListIcon } from "@/icons";
import React, { useCallback, useEffect, useMemo, useState } from "react";

const fieldClasses =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

type EditorState =
  | { mode: "create" }
  | { mode: "edit"; variable: Variable }
  | null;

export default function VariablesPage() {
  const { hasPermission } = useAdminIdentity();
  const toast = useToast();
  const [variables, setVariables] = useState<Variable[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState("");

  const [editor, setEditor] = useState<EditorState>(null);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [description, setDescription] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<Variable | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const canWrite = hasPermission("admins.write");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.variables();
      setVariables(response.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load variables");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return variables;
    return variables.filter(
      (variable) =>
        variable.key.toLowerCase().includes(q) ||
        variable.value.toLowerCase().includes(q) ||
        variable.description.toLowerCase().includes(q)
    );
  }, [variables, filter]);

  function openCreate() {
    setKey("");
    setValue("");
    setDescription("");
    setSaveError(null);
    setEditor({ mode: "create" });
  }

  function openEdit(variable: Variable) {
    setKey(variable.key);
    setValue(variable.value);
    setDescription(variable.description);
    setSaveError(null);
    setEditor({ mode: "edit", variable });
  }

  async function handleSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editor) return;
    setSaving(true);
    setSaveError(null);
    try {
      if (editor.mode === "create") {
        await api.createVariable(key.trim(), value, description.trim());
        toast.success("Variable created", key.trim());
      } else {
        await api.updateVariable(editor.variable.id, value, description.trim());
        toast.success("Variable updated", key.trim());
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
      await api.deleteVariable(deleteTarget.id);
      setDeleteTarget(null);
      await load();
      toast.success("Variable deleted", deleteTarget.key);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Delete failed";
      setDeleteError(message);
      toast.error("Delete failed", message);
    } finally {
      setDeleteBusy(false);
    }
  }

  return (
    <div>
      <PageTitle
        title="Variables"
        description="KeyAuth-style key-value store for app settings, links and messages."
        actions={
          canWrite ? (
            <Button size="sm" onClick={openCreate}>
              Create variable
            </Button>
          ) : undefined
        }
      />
      <ConsoleSection
        title="Variables"
        description={`${variables.length} variable(s)`}
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
        ) : variables.length === 0 ? (
          <EmptyState
            icon={<ListIcon />}
            title="No variables yet"
            message="Store app-wide key-value settings here (download links, announcements, feature flags)."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={<ListIcon />}
            title="No matching variables"
            message={`Nothing matches “${filter}”.`}
          />
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 dark:border-gray-800">
              <tr className="text-xs uppercase text-gray-400">
                <th className="px-5 py-3 font-medium">Key</th>
                <th className="px-5 py-3 font-medium">Value</th>
                <th className="px-5 py-3 font-medium">Description</th>
                <th className="px-5 py-3 font-medium">Updated</th>
                {canWrite && <th className="px-5 py-3 font-medium"></th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {filtered.map((variable) => (
                <tr
                  key={variable.id}
                  className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                >
                  <td className="px-5 py-3.5">
                    <span className="font-mono text-sm font-medium text-gray-800 dark:text-white/90">
                      {variable.key}
                    </span>
                  </td>
                  <td className="max-w-[320px] px-5 py-3.5">
                    <span className="block truncate text-gray-500 dark:text-gray-400">
                      {variable.value || "—"}
                    </span>
                  </td>
                  <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                    {variable.description || "—"}
                  </td>
                  <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                    {new Date(variable.updated_at).toLocaleString()}
                  </td>
                  {canWrite && (
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex justify-end">
                        <RowActions
                          actions={
                            [
                              {
                                label: "Edit",
                                tone: "info",
                                onClick: () => openEdit(variable),
                              },
                              {
                                label: "Delete",
                                tone: "danger",
                                onClick: () => {
                                  setDeleteError(null);
                                  setDeleteTarget(variable);
                                },
                              },
                            ] as RowAction[]
                          }
                        />
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </ConsoleSection>

      <Modal
        isOpen={editor !== null}
        onClose={() => !saving && setEditor(null)}
        className="max-w-md p-6"
      >
        {editor && (
          <form onSubmit={handleSave} className="space-y-5">
            <div>
              <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
                {editor.mode === "create"
                  ? "Create variable"
                  : `Edit ${editor.variable.key}`}
              </h3>
              <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                Variables are consumed by the client to configure the app
                without redeploying.
              </p>
            </div>
            {editor.mode === "create" && (
              <div>
                <Label>Key *</Label>
                <input
                  type="text"
                  required
                  pattern="[a-z][a-z0-9_.-]{0,63}"
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  placeholder="download_url"
                  className={fieldClasses}
                />
                <p className="mt-1 text-xs text-gray-400">
                  Lowercase letters, digits, dots, dashes or underscores (max 64).
                </p>
              </div>
            )}
            <div>
              <Label>Value</Label>
              <textarea
                value={value}
                onChange={(e) => setValue(e.target.value)}
                rows={3}
                maxLength={10000}
                placeholder="https://..."
                className="w-full resize-y rounded-lg border border-gray-300 bg-transparent px-4 py-3 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
              />
            </div>
            <div>
              <Label>Description</Label>
              <input
                type="text"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                maxLength={500}
                placeholder="What is this variable for?"
                className={fieldClasses}
              />
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
                    ? "Create variable"
                    : "Save changes"}
              </Button>
            </div>
          </form>
        )}
      </Modal>

      <ConfirmModal
        isOpen={deleteTarget !== null}
        title="Delete variable"
        message={
          deleteTarget
            ? `Delete the variable “${deleteTarget.key}”? Clients reading this key will no longer receive a value. This cannot be undone.`
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
