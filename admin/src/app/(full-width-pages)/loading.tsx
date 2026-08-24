export default function Loading() {
  return (
    <main className="flex min-h-screen items-center justify-center p-6" aria-busy="true" aria-label="Loading page">
      <div className="h-40 w-full max-w-md animate-pulse rounded-2xl bg-gray-100 dark:bg-white/[0.03]" />
    </main>
  );
}
