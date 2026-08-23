import { formatDateTime, formatRelativeTime } from "@/lib/time";

// Compact relative timestamp with the absolute value exposed through the
// native title tooltip — scannable in dense tables without losing precision.
export default function TimeAgo({
  value,
  className = "",
}: {
  value: string | null | undefined;
  className?: string;
}) {
  return (
    <span
      title={value ? formatDateTime(value) : undefined}
      className={className}
    >
      {formatRelativeTime(value)}
    </span>
  );
}
