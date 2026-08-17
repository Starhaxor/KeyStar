"use client";
import ActivityChart from "@/components/console/ActivityChart";
import CompositionChart from "@/components/console/CompositionChart";
import { CardSkeleton, Skeleton, TableSkeleton } from "@/components/common/Skeleton";
import ConsoleSection, {
  EmptyNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import StatCard from "@/components/console/StatCard";
import { api, formatDateTime } from "@/lib/api";
import type { DailyStat, Overview, TodayStats } from "@/lib/types";
import {
  AlertIcon,
  BoltIcon,
  BoxCubeIcon,
  DocsIcon,
  LockIcon,
  TimeIcon,
  UserCircleIcon,
  UserIcon,
} from "@/icons";
import React, { useEffect, useState } from "react";

export default function OverviewPage() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [stats, setStats] = useState<DailyStat[] | null>(null);
  const [today, setToday] = useState<TodayStats | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Fetch on mount and refresh every 60s so the dashboard behaves like a
  // live operations center. State updates only happen after the awaited
  // fetch resolves, never synchronously inside the effect.
  useEffect(() => {
    let disposed = false;
    async function refresh() {
      try {
        const [overviewResponse, statsResponse, todayResponse] =
          await Promise.all([
            api.overview(),
            api.overviewStats(),
            api.overviewToday(),
          ]);
        if (disposed) return;
        setError(null);
        setOverview(overviewResponse);
        setStats(statsResponse.days);
        setToday(todayResponse);
      } catch (err) {
        if (disposed) return;
        setError(
          err instanceof Error ? err.message : "Failed to load overview"
        );
      }
    }
    refresh();
    const timer = setInterval(refresh, 60_000);
    return () => {
      disposed = true;
      clearInterval(timer);
    };
  }, []);

  return (
    <div>
      <PageTitle
        title="Overview"
        description="Live snapshot of the StarLoader licensing system."
        actions={
          <span className="flex items-center gap-1.5 rounded-full border border-gray-200 px-3 py-1 text-xs font-medium text-gray-500 dark:border-gray-800 dark:text-gray-400">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-success-500" />
            Live · refreshes every 60s
          </span>
        }
      />
      {error && (
        <div className="mb-4 rounded-lg border border-error-500/30 bg-error-50 px-4 py-3 text-sm text-error-600 dark:bg-error-500/10 dark:text-error-500">
          {error}
        </div>
      )}
      {!overview && !error && (
        <div className="space-y-6">
          {[0, 1, 2].map((row) => (
            <div
              key={row}
              className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4"
            >
              {[0, 1, 2, 3].map((index) => (
                <div
                  key={index}
                  className="rounded-2xl border border-gray-200 bg-white p-5 shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]"
                >
                  <CardSkeleton />
                </div>
              ))}
            </div>
          ))}
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

          {today && (
            <>
              <div>
                <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-400">
                  Today
                </h2>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
                  <StatCard
                    label="Sessions Today"
                    value={today.logins_today}
                    icon={<TimeIcon />}
                  />
                  <StatCard
                    label="Activations Today"
                    value={today.activations_today}
                    icon={<BoltIcon />}
                  />
                  <StatCard
                    label="New Devices Today"
                    value={today.new_devices_today}
                    icon={<BoxCubeIcon />}
                  />
                  <StatCard
                    label="Admin Logins Today"
                    value={today.admin_logins_today}
                    icon={<UserIcon />}
                  />
                </div>
              </div>
              <div>
                <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-gray-400">
                  Watchlist
                </h2>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
                  <StatCard
                    label="Failed Logins Today"
                    value={today.failed_logins_today}
                    icon={<AlertIcon />}
                  />
                  <StatCard
                    label="Permission Denied Today"
                    value={today.permission_denied_today}
                    icon={<LockIcon />}
                  />
                  <StatCard
                    label="Banned Users"
                    value={today.banned_users}
                    icon={<UserCircleIcon />}
                  />
                  <StatCard
                    label="Expired Licenses"
                    value={today.expired_licenses}
                    icon={<DocsIcon />}
                  />
                </div>
              </div>
            </>
          )}

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
