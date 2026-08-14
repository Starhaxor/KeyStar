import React from "react";

interface StatCardProps {
  label: string;
  value: number | string;
  icon: React.ReactNode;
}

export default function StatCard({ label, value, icon }: StatCardProps) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-5 shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
      <div className="flex items-center justify-between">
        <div>
          <span className="block text-sm text-gray-500 dark:text-gray-400">
            {label}
          </span>
          <span className="mt-1 block text-2xl font-semibold text-gray-800 dark:text-white/90">
            {value}
          </span>
        </div>
        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-brand-50 text-brand-500 dark:bg-brand-500/15 dark:text-brand-400">
          {icon}
        </div>
      </div>
    </div>
  );
}
