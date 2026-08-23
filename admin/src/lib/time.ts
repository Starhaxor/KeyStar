// Shared date/time formatting for console tables. `formatDateTime` renders
// full locale timestamps; `formatRelativeTime` renders compact "5m ago"
// labels so operators can scan activity without parsing absolute dates.

export function formatDateTime(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString();
}

function relativeSpan(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d`;
  if (days < 30) return `${Math.floor(days / 7)}w`;
  if (days < 365) return `${Math.floor(days / 30)}mo`;
  return `${Math.floor(days / 365)}y`;
}

export function formatRelativeTime(
  value: string | null | undefined,
  now: Date = new Date()
): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  const diffSeconds = (now.getTime() - date.getTime()) / 1000;
  if (Math.abs(diffSeconds) < 60) return "just now";
  const span = relativeSpan(Math.abs(diffSeconds));
  return diffSeconds >= 0 ? `${span} ago` : `in ${span}`;
}
