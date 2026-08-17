"use client";
import React, { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import Link from "next/link";

export type RowAction = {
  label: string;
  icon?: React.ReactNode;
  onClick?: () => void;
  href?: string;
  danger?: boolean;
  disabled?: boolean;
};

// Three-dot row action menu. Rendered through a portal with fixed
// positioning so it is never clipped by the table's overflow container.
export default function RowActions({ actions }: { actions: RowAction[] }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; right: number } | null>(null);
  const [mounted, setMounted] = useState(false);

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => {
      const target = event.target as HTMLElement;
      if (!target.closest("[data-row-actions]")) setOpen(false);
    };
    const dismiss = () => setOpen(false);
    document.addEventListener("mousedown", close);
    document.addEventListener("scroll", dismiss, true);
    window.addEventListener("resize", dismiss);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("scroll", dismiss, true);
      window.removeEventListener("resize", dismiss);
    };
  }, [open]);

  function toggle(event: React.MouseEvent) {
    event.stopPropagation();
    if (!open) {
      const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
      setPos({
        top: rect.bottom + 6,
        right: Math.max(8, window.innerWidth - rect.right),
      });
    }
    setOpen((prev) => !prev);
  }

  const menu =
    open && pos && mounted
      ? createPortal(
          <div
            data-row-actions
            style={{ top: pos.top, right: pos.right }}
            className="fixed z-50 w-48 rounded-xl border border-gray-200 bg-white p-1.5 shadow-theme-lg dark:border-gray-800 dark:bg-gray-dark"
          >
            {actions.map((action, index) => {
              const classes = `flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                action.danger
                  ? "text-error-600 hover:bg-error-50 dark:text-error-500 dark:hover:bg-error-500/10"
                  : "text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-white/5"
              } ${action.disabled ? "pointer-events-none opacity-40" : ""}`;
              if (action.href) {
                return (
                  <Link
                    key={index}
                    href={action.href}
                    onClick={() => setOpen(false)}
                    className={classes}
                  >
                    {action.icon}
                    {action.label}
                  </Link>
                );
              }
              return (
                <button
                  key={index}
                  type="button"
                  disabled={action.disabled}
                  onClick={() => {
                    setOpen(false);
                    action.onClick?.();
                  }}
                  className={classes}
                >
                  {action.icon}
                  {action.label}
                </button>
              );
            })}
          </div>,
          document.body
        )
      : null;

  return (
    <div data-row-actions className="relative">
      <button
        type="button"
        aria-label="Row actions"
        onClick={toggle}
        className="flex h-8 w-8 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-white/5 dark:hover:text-white"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          <circle cx="8" cy="2.5" r="1.5" />
          <circle cx="8" cy="8" r="1.5" />
          <circle cx="8" cy="13.5" r="1.5" />
        </svg>
      </button>
      {menu}
    </div>
  );
}
