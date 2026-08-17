"use client";
import ActivityChart from "@/components/console/ActivityChart";
import CompositionChart from "@/components/console/CompositionChart";
import { CardSkeleton, Skeleton, TableSkeleton } from "@/components/common/Skeleton";
import ConsoleSection, {
  EmptyNote,
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import StatCard from "@/components/console/StatCard";
import { api, formatDateTime } from "@/lib/api";
import type { DailyStat, Overview } from "@/lib/types";
import { BoxCubeIcon, DocsIcon, TimeIcon, UserCircleIcon } from "@/icons";
import React, { useCallback, useEffect, useState } from "react";

export default function OverviewPage() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [stats, setStats] = useState<DailyStat[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setError(null);
      const [overviewResponse, statsResponse] = await Promise.all([
        api.overview(),
        api.overviewStats(),
      ]);
      setOverview(overviewResponse);
      setStats(statsResponse.days);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load overview");
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div>
      <PageTitle
        title="Overview"
        description="Live snapshot of the StarLoader licensing system."
      />
      {error && (
        <div className="mb-4 rounded-lg border border-error-500/30 bg-error-50 px-4 py-3 text-sm text-error-600 dark:bg-error-500/10 dark:text-error-500">
          {error}
        </div>
      )}
      {!overview && !error && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            {[0, 1, 2, 3].map((index) => (
              <div
                key={index}
                className="rounded-2xl border border-gray-200 bg-white p-5 shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]"
              >
                <CardSkeleton />
              </div>
            ))}
          </div>
          <div className="rounded-2xl border border-gray-200 bg-white shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
            <TableSkeleton rows={5} cols={4} />
          </div>
        </div>
      )}
      {overview && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard
              label="Total Users"
              value={overview.total_users}
              icon={<UserCircleIcon />}
            />
            <StatCard
              label="Active Licenses"
              value={overview.active_licenses}
              icon={<DocsIcon />}
            />
            <StatCard
              label="Active Devices"
              value={overview.active_devices}
              icon={<BoxCubeIcon />}
            />
            <StatCard
              label="Active Sessions"
              value={overview.active_sessions}
              icon={<TimeIcon />}
            />
          </div>

          <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
            <div className="xl:col-span-2">
              <ConsoleSection
                title="Activity"
                description="Licenses, devices, sessions and admin logins over the last 14 days."
              >
                {!stats ? (
                  <div className="p-5">
                    <Skeleton className="h-64 w-full" />
                  </div>
                ) : stats.length === 0 ? (
                  <EmptyNote message="No activity data yet." />
                ) : (
                  <ActivityChart stats={stats} />
                )}
              </ConsoleSection>
            </div>
            <ConsoleSection
              title="Composition"
              description="Share of activity over the last 14 days."
            >
              {!stats ? (
                <div className="p-5">
                  <Skeleton className="h-64 w-full" />
                </div>
              ) : stats.length === 0 ? (
                <EmptyNote message="No activity data yet." />
              ) : (
                <CompositionChart stats={stats} />
              )}
            </ConsoleSection>
          </div>

          <ConsoleSection
            title="Recent Admin Activity"
            description="Latest events from the audit log."
          >
            {overview.recent_audit.length === 0 ? (
              <EmptyNote message="No admin activity recorded yet." />
            ) : (
              <table className="w-full text-left text-sm">
                <thead className="border-b border-gray-200 dark:border-gray-800">
                  <tr className="text-xs uppercase text-gray-400">
                    <th className="px-5 py-3 font-medium">Action</th>
                    <th className="px-5 py-3 font-medium">Actor</th>
                    <th className="px-5 py-3 font-medium">Resource</th>
                    <th className="px-5 py-3 font-medium">When</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                  {overview.recent_audit.map((entry) => (
                    <tr
                      key={entry.id}
                      className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                    >
                      <td className="px-5 py-3 font-medium text-gray-700 dark:text-gray-300">
                        {entry.action}
                      </td>
                      <td className="px-5 py-3 text-gray-500 dark:text-gray-400">
                        {entry.actor_email || "—"}
                      </td>
                      <td className="px-5 py-3 text-gray-500 dark:text-gray-400">
                        {entry.resource_type
                          ? `${entry.resource_type}${
                              entry.resource_id ? ` · ${entry.resource_id}` : ""
                            }`
                          : "—"}
                      </td>
                      <td className="px-5 py-3 text-gray-500 dark:text-gray-400">
                        {formatDateTime(entry.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </ConsoleSection>
        </div>
      )}
    </div>
  );
}
