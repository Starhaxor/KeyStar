import React from "react";

export default function EmptyState({
  icon,
  title,
  message,
  action,
}: {
  icon?: React.ReactNode;
  title: string;
  message?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-12 text-center">
      {icon && (
        <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 text-gray-400 dark:bg-white/[0.05] dark:text-gray-500">
          {icon}
        </div>
      )}
      <p className="text-sm font-medium text-gray-800 dark:text-white/90">
        {title}
      </p>
      {message && (
        <p className="mt-1 max-w-sm text-sm text-gray-500 dark:text-gray-400">
          {message}
        </p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
