export default function Loading() {
  return (
    <div className="space-y-6" aria-busy="true" aria-label="Loading console page">
      <div className="h-8 w-48 animate-pulse rounded bg-gray-200 dark:bg-gray-800" />
      <div className="h-56 animate-pulse rounded-2xl border border-gray-200 bg-gray-100 dark:border-gray-800 dark:bg-white/[0.03]" />
    </div>
  );
}
