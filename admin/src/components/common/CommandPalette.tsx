"use client";

import { useRouter } from "next/navigation";
import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { isSidebarItemVisible, sidebarSections } from "@/layout/sidebarNavigation";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useTheme } from "@/context/ThemeContext";
import { SearchIcon } from "@/icons";

// Opens the palette from anywhere via Ctrl/⌘+K or by dispatching this DOM
// event — the header search button uses it so both triggers share one state.
export const COMMAND_PALETTE_EVENT = "keystar:command-palette";

type PaletteEntry = {
  id: string;
  label: string;
  group: string;
  href?: string;
  action?: () => void;
  keywords?: string;
};

function isMac(): boolean {
  if (typeof navigator === "undefined") return false;
  return /mac|iPod|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
}

export default function CommandPalette() {
  const router = useRouter();
  const { hasPermission } = useAdminIdentity();
  const { resolvedTheme, toggleTheme } = useTheme();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const close = useCallback(() => {
    setOpen(false);
    setQuery("");
    setActiveIndex(0);
  }, []);

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((prev) => !prev);
      }
    }
    function onOpenEvent() {
      setOpen(true);
    }
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener(COMMAND_PALETTE_EVENT, onOpenEvent);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener(COMMAND_PALETTE_EVENT, onOpenEvent);
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    // Focus after paint so the caret lands reliably once the panel mounts.
    const timer = setTimeout(() => inputRef.current?.focus(), 0);
    document.body.style.overflow = "hidden";
    return () => {
      clearTimeout(timer);
      document.body.style.overflow = "";
    };
  }, [open]);

  const entries = useMemo<PaletteEntry[]>(() => {
    const navigation: PaletteEntry[] = [];
    for (const section of sidebarSections) {
      for (const item of section.items) {
        if (!isSidebarItemVisible(item, hasPermission)) continue;
        if (
          item.children?.some(
            (child) => child.permission && !hasPermission(child.permission)
          )
        ) {
          continue;
        }
        navigation.push({
          id: `nav:${item.path}`,
          label: item.name,
          group: section.name,
          href: item.path,
          keywords: `${section.name} ${item.children?.map((c) => c.label).join(" ") ?? ""}`,
        });
      }
    }
    return [
      ...navigation,
      {
        id: "action:theme",
        label:
          resolvedTheme === "dark"
            ? "Switch to light theme"
            : "Switch to dark theme",
        group: "Actions",
        action: toggleTheme,
        keywords: "appearance mode dark light",
      },
    ];
  }, [hasPermission, resolvedTheme, toggleTheme]);

  const results = useMemo<PaletteEntry[]>(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return entries;
    return entries.filter((entry) =>
      `${entry.label} ${entry.group} ${entry.keywords ?? ""}`
        .toLowerCase()
        .includes(needle)
    );
  }, [entries, query]);

  useEffect(() => {
    const container = listRef.current;
    const active = container?.children[activeIndex] as HTMLElement | undefined;
    active?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);

  const runEntry = useCallback(
    (entry: PaletteEntry | undefined) => {
      if (!entry) return;
      close();
      if (entry.href) router.push(entry.href);
      else entry.action?.();
    },
    [close, router]
  );

  function handleInputKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((prev) => Math.min(prev + 1, Math.max(results.length - 1, 0)));
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((prev) => Math.max(prev - 1, 0));
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      runEntry(results[activeIndex]);
    }
  }

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[999999] flex items-start justify-center px-4 pt-[12vh]"
      role="dialog"
      aria-modal="true"
      aria-label="Command palette"
    >
      <button
        type="button"
        aria-label="Close command palette"
        onClick={close}
        className="absolute inset-0 h-full w-full cursor-default bg-gray-950/40 backdrop-blur-[2px] dark:bg-black/60"
        tabIndex={-1}
      />
      <div className="relative w-full max-w-xl overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-theme-xl dark:border-gray-700 dark:bg-gray-900">
        <div className="flex items-center gap-3 border-b border-gray-100 px-4 py-3 dark:border-gray-800">
          <span className="text-gray-400 dark:text-gray-500">
            <SearchIcon width={18} height={18} />
          </span>
          <input
            ref={inputRef}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setActiveIndex(0);
            }}
            onKeyDown={handleInputKeyDown}
            placeholder="Search pages and actions..."
            role="combobox"
            aria-expanded="true"
            aria-controls="command-palette-list"
            aria-activedescendant={
              results[activeIndex]
                ? `command-palette-option-${activeIndex}`
                : undefined
            }
            className="w-full bg-transparent text-sm text-gray-800 placeholder:text-gray-400 focus:outline-none dark:text-white/90 dark:placeholder:text-gray-500"
          />
          <kbd className="hidden rounded-md border border-gray-200 px-1.5 py-0.5 text-xs font-medium text-gray-400 sm:block dark:border-gray-700 dark:text-gray-500">
            Esc
          </kbd>
        </div>

        <div
          ref={listRef}
          id="command-palette-list"
          role="listbox"
          aria-label="Commands"
          className="custom-scrollbar max-h-80 overflow-y-auto p-2"
        >
          {results.length === 0 ? (
            <p className="px-3 py-6 text-center text-sm text-gray-500 dark:text-gray-400">
              No matches for “{query}”
            </p>
          ) : (
            results.map((entry, index) => {
              const showGroup =
                index === 0 || results[index - 1].group !== entry.group;
              return (
                <React.Fragment key={entry.id}>
                  {showGroup && (
                    <p className="px-3 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-gray-400 first:pt-1 dark:text-gray-500">
                      {entry.group}
                    </p>
                  )}
                  <div
                    id={`command-palette-option-${index}`}
                    role="option"
                    aria-selected={index === activeIndex}
                    onMouseMove={() => setActiveIndex(index)}
                    onClick={() => runEntry(entry)}
                    className={`flex cursor-pointer items-center justify-between gap-3 rounded-lg px-3 py-2.5 text-sm font-medium ${
                      index === activeIndex
                        ? "bg-brand-50 text-brand-600 dark:bg-brand-500/[0.12] dark:text-brand-400"
                        : "text-gray-700 dark:text-gray-300"
                    }`}
                  >
                    <span className="truncate">{entry.label}</span>
                    {entry.href && (
                      <span
                        aria-hidden="true"
                        className="shrink-0 text-xs font-normal text-gray-400 dark:text-gray-500"
                      >
                        Jump
                      </span>
                    )}
                  </div>
                </React.Fragment>
              );
            })
          )}
        </div>

        <div className="flex items-center gap-4 border-t border-gray-100 px-4 py-2.5 text-xs text-gray-400 dark:border-gray-800 dark:text-gray-500">
          <span>↑↓ navigate</span>
          <span>↵ open</span>
          <span>esc close</span>
          <span className="ml-auto hidden sm:block">
            {isMac() ? "⌘K" : "Ctrl K"}
          </span>
        </div>
      </div>
    </div>
  );
}
