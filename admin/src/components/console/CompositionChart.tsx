"use client";
import type { DailyStat } from "@/lib/types";
import dynamic from "next/dynamic";
import React, { useMemo } from "react";

const Chart = dynamic(() => import("react-apexcharts"), { ssr: false });

export default function CompositionChart({ stats }: { stats: DailyStat[] }) {
  const { series, labels } = useMemo(() => {
    const totals = {
      Licenses: stats.reduce((sum, stat) => sum + stat.licenses_created, 0),
      Devices: stats.reduce((sum, stat) => sum + stat.devices_registered, 0),
      Sessions: stats.reduce((sum, stat) => sum + stat.sessions_created, 0),
      "Admin logins": stats.reduce((sum, stat) => sum + stat.admin_logins, 0),
    };
    return {
      series: Object.values(totals),
      labels: Object.keys(totals),
    };
  }, [stats]);

  const options = useMemo(
    () => ({
      chart: {
        type: "donut" as const,
        fontFamily: "inherit",
        foreColor: "#98a2b3",
        toolbar: { show: false },
      },
      labels,
      colors: ["#16a34a", "#0ba5ec", "#f79009", "#7a5af8"],
      stroke: { width: 0 },
      dataLabels: { enabled: false },
      legend: {
        position: "bottom" as const,
        fontSize: "12px",
        markers: { size: 5 },
        itemMargin: { horizontal: 8, vertical: 4 },
      },
      plotOptions: {
        pie: {
          donut: {
            size: "72%",
            labels: {
              show: true,
              name: { fontSize: "12px", color: "#98a2b3" },
              value: {
                fontSize: "20px",
                fontWeight: 600,
                color: "var(--color-gray-800)",
              },
              total: {
                show: true,
                label: "14-day total",
                fontSize: "12px",
                color: "#98a2b3",
              },
            },
          },
        },
      },
    }),
    [labels]
  );

  return (
    <div className="p-5">
      <Chart type="donut" height={320} width="100%" series={series} options={options} />
    </div>
  );
}
