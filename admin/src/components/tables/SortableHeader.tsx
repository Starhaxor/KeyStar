import type { SortState } from "@/lib/tableSort";

// Clickable table header cell that reflects and cycles the active sort.
export default function SortableHeader<K extends string>({
  label,
  sortKey,
  sort,
  onToggle,
  className = "",
}: {
  label: string;
  sortKey: K;
  sort: SortState<K>;
  onToggle: (key: K) => void;
  className?: string;
}) {
  const active = sort?.key === sortKey;
  const direction = active ? sort.direction : null;
  return (
    <th
      scope="col"
      aria-sort={
        direction === "asc"
          ? "ascending"
          : direction === "desc"
            ? "descending"
            : undefined
      }
      className={`px-5 py-3 font-medium ${className}`}
    >
      <button
        type="button"
        onClick={() => onToggle(sortKey)}
        className="group inline-flex items-center gap-1 uppercase transition-colors hover:text-gray-600 dark:hover:text-gray-300"
      >
        {label}
        <span
          aria-hidden="true"
          className={`inline-flex flex-col leading-none transition-colors ${
            active
              ? "text-brand-500 dark:text-brand-400"
              : "text-gray-300 group-hover:text-gray-400 dark:text-gray-600 dark:group-hover:text-gray-500"
          }`}
        >
          <svg
            width="8"
            height="5"
            viewBox="0 0 8 5"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            className={direction === "desc" ? "opacity-30" : ""}
          >
            <path
              d="M4 0L7.4641 4.5H0.535898L4 0Z"
              fill="currentColor"
            />
          </svg>
          <svg
            width="8"
            height="5"
            viewBox="0 0 8 5"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            className={direction === "asc" ? "opacity-30" : ""}
          >
            <path d="M4 5L0.535898 0.5H7.4641L4 5Z" fill="currentColor" />
          </svg>
        </span>
      </button>
    </th>
  );
}
