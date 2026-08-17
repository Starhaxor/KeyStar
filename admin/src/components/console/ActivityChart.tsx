"use client";
import type { DailyStat } from "@/lib/types";
import dynamic from "next/dynamic";
import React, { useMemo } from "react";

const Chart = dynamic(() => import("react-apexcharts"), { ssr: false });

function dayLabel(day: string): string {
  const date = new Date(`${day}T00:00:00Z`);
  if (Number.isNaN(date.getTime())) return day;
  return date.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

export default function ActivityChart({ stats }: { stats: DailyStat[] }) {
  const series = useMemo(
    () => [
      {
        name: "Licenses",
        data: stats.map((stat) => stat.licenses_created),
      },
      {
        name: "Devices",
        data: stats.map((stat) => stat.devices_registered),
      },
      {
        name: "Sessions",
        data: stats.map((stat) => stat.sessions_created),
      },
      {
        name: "Admin logins",
        data: stats.map((stat) => stat.admin_logins),
      },
    ],
    [stats]
  );

  const categories = useMemo(() => stats.map((stat) => dayLabel(stat.day)), [stats]);

  const options = useMemo(
    () => ({
      chart: {
        type: "area" as const,
        toolbar: { show: false },
        fontFamily: "inherit",
        foreColor: "#98a2b3",
        zoom: { enabled: false },
      },
      colors: ["#465fff", "#12b76a", "#f79009", "#7a5af8"],
      dataLabels: { enabled: false },
      stroke: { curve: "smooth" as const, width: 2 },
      fill: {
        type: "gradient",
        gradient: {
          opacityFrom: 0.35,
          opacityTo: 0.05,
        },
      },
      grid: {
        borderColor: "var(--color-gray-200)",
        strokeDashArray: 4,
        padding: { left: 8, right: 8 },
      },
      xaxis: {
        categories,
        axisBorder: { show: false },
        axisTicks: { show: false },
        labels: { style: { colors: "#98a2b3", fontSize: "12px" } },
      },
      yaxis: {
        min: 0,
        forceNiceScale: true,
        labels: { style: { colors: "#98a2b3", fontSize: "12px" } },
      },
      legend: {
        position: "top" as const,
        horizontalAlign: "right" as const,
        fontSize: "12px",
        markers: { size: 5 },
        itemMargin: { horizontal: 8 },
      },
      tooltip: { enabled: true },
    }),
    [categories]
  );

  return (
    <div className="p-5">
      <Chart
        type="area"
        height={320}
        width="100%"
        series={series}
        options={options}
      />
    </div>
  );
}
