import Badge from "@/components/ui/badge/Badge";
import React from "react";

const statusColors: Record<
  string,
  "success" | "error" | "warning" | "light" | "info"
> = {
  active: "success",
  verified: "success",
  pending: "warning",
  locked: "warning",
  disabled: "error",
  banned: "error",
  revoked: "error",
  expired: "light",
};

export default function StatusBadge({ status }: { status: string }) {
  const color = statusColors[status] ?? "light";
  return (
    <Badge size="sm" color={color}>
      {status}
    </Badge>
  );
}
