"use client";

export default function Error({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <section className="rounded-2xl border border-error-200 bg-error-50 p-6 text-error-800 dark:border-error-500/30 dark:bg-error-500/10 dark:text-error-200" role="alert">
      <h1 className="text-lg font-semibold">This console page could not be loaded.</h1>
      <p className="mt-2 text-sm">Retry the page. If this keeps happening, check your connection and contact a KeyStar administrator.</p>
      <button type="button" className="mt-4 rounded-lg bg-error-600 px-4 py-2 text-sm font-medium text-white hover:bg-error-700 focus:outline-none focus:ring-2 focus:ring-error-500 focus:ring-offset-2" onClick={reset}>
        Retry
      </button>
    </section>
  );
}
