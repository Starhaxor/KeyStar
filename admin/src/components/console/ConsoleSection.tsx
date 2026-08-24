import React from "react";

interface ConsoleSectionProps {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}

export default function ConsoleSection({
  title,
  description,
  actions,
  children,
}: ConsoleSectionProps) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-gray-800">
        <div>
          <h2 className="text-base font-semibold text-gray-800 dark:text-white/90">
            {title}
          </h2>
          {description && (
            <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
              {description}
            </p>
          )}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      <div
        data-testid="console-section-content"
        className="max-w-full overflow-x-auto"
      >
        {children}
      </div>
    </div>
  );
}

export function TableCard({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">
      <div
        data-testid="table-card-content"
        className="max-w-full overflow-x-auto"
      >
        {children}
      </div>
    </div>
  );
}

export function PageTitle({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 className="text-xl font-semibold text-gray-800 dark:text-white/90">
          {title}
        </h1>
        {description && (
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {description}
          </p>
        )}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}

export function LoadingNote() {
  return (
    <p className="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      Loading...
    </p>
  );
}

export function EmptyNote({ message }: { message: string }) {
  return (
    <p className="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {message}
    </p>
  );
}

export function ErrorNote({ message }: { message: string }) {
  return (
    <p className="px-5 py-8 text-center text-sm text-error-500">{message}</p>
  );
}
