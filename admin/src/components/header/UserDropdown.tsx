"use client";
import { api } from "@/lib/api";
import { Dropdown } from "@/components/ui/dropdown/Dropdown";
import { DropdownItem } from "@/components/ui/dropdown/DropdownItem";
import { Modal } from "@/components/ui/modal";
import { ChevronDownIcon, LockIcon, UserCircleIcon } from "@/icons";
import Link from "next/link";
import React, { useEffect, useState } from "react";

function initialsFor(email: string): string {
  const local = email.split("@")[0] ?? "";
  return local.slice(0, 2).toUpperCase() || "A";
}

export default function UserDropdown() {
  const [email, setEmail] = useState<string>("");
  const [isOpen, setIsOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [signingOut, setSigningOut] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api
      .me()
      .then((identity) => {
        if (!cancelled) setEmail(identity.email);
      })
      .catch(() => {
        // The 401 handler in the API client redirects to /signin.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleSignOut() {
    if (signingOut) return;
    setSigningOut(true);
    try {
      await api.logout();
    } catch {
      // Even if the backend call fails, drop the local session.
    }
    window.location.assign("/signin");
  }

  return (
    <>
      <div className="relative">
        <button
          className="dropdown-toggle flex items-center gap-2 rounded-lg px-2 py-1.5 transition-colors hover:bg-gray-100 dark:hover:bg-white/5"
          onClick={() => setIsOpen((prev) => !prev)}
          aria-label="Account menu"
        >
          <span className="flex h-9 w-9 items-center justify-center rounded-full bg-brand-500 text-sm font-semibold text-white">
            {initialsFor(email)}
          </span>
          <span className="hidden text-left sm:block">
            <span className="block max-w-[160px] truncate text-sm font-medium text-gray-800 dark:text-white/90">
              {email || "Administrator"}
            </span>
            <span className="block text-xs text-gray-400">Admin</span>
          </span>
          <ChevronDownIcon
            className={`hidden text-gray-400 transition-transform sm:block ${
              isOpen ? "rotate-180" : ""
            }`}
          />
        </button>

        <Dropdown isOpen={isOpen} onClose={() => setIsOpen(false)}>
          <div className="w-60 p-2">
            <div className="mb-2 flex items-center gap-3 rounded-lg border-b border-gray-100 px-3 pb-3 dark:border-gray-800">
              <span className="flex h-10 w-10 items-center justify-center rounded-full bg-brand-500 text-sm font-semibold text-white">
                {initialsFor(email)}
              </span>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-gray-800 dark:text-white/90">
                  {email || "Administrator"}
                </p>
                <p className="text-xs text-gray-400">Owner · Admin</p>
              </div>
            </div>
            <DropdownItem
              tag="a"
              href="/profile"
              onClick={() => setIsOpen(false)}
              baseClassName="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-white/5"
            >
              <UserCircleIcon />
              Profile
            </DropdownItem>
            <DropdownItem
              tag="a"
              href="/security"
              onClick={() => setIsOpen(false)}
              baseClassName="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-white/5"
            >
              <LockIcon />
              Security
            </DropdownItem>
            <div className="my-1 border-t border-gray-100 dark:border-gray-800" />
            <DropdownItem
              onClick={() => {
                setIsOpen(false);
                setConfirmOpen(true);
              }}
              baseClassName="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-error-600 hover:bg-error-50 dark:text-error-500 dark:hover:bg-error-500/10"
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                xmlns="http://www.w3.org/2000/svg"
              >
                <path
                  fillRule="evenodd"
                  clipRule="evenodd"
                  d="M15.1007 19.247C14.6865 19.247 14.3507 18.9112 14.3507 18.497L14.3507 14.245H12.8507V18.497C12.8507 19.7396 13.8581 20.747 15.1007 20.747H18.5007C19.7434 20.747 20.7507 19.7396 20.7507 18.497L20.7507 5.49609C20.7507 4.25345 19.7433 3.24609 18.5007 3.24609H15.1007C13.8581 3.24609 12.8507 4.25345 12.8507 5.49609V9.74501L14.3507 9.74501V5.49609C14.3507 5.08188 14.6865 4.74609 15.1007 4.74609L18.5007 4.74609C18.9149 4.74609 19.2507 5.08188 19.2507 5.49609L19.2507 18.497C19.2507 18.9112 18.9149 19.247 18.5007 19.247H15.1007ZM3.25073 11.9984C3.25073 12.2144 3.34204 12.4091 3.48817 12.546L8.09483 17.1556C8.38763 17.4485 8.86251 17.4487 9.15549 17.1559C9.44848 17.8631 9.44863 17.3882 9.15583 17.0952L5.81116 13.7484L16.0007 13.7484C16.4149 13.7484 16.7507 13.4127 16.7507 12.9984C16.7507 12.5842 16.4149 12.2484 16.0007 12.2484L5.81528 12.2484L9.15585 8.90554C9.44864 8.61255 9.44847 8.13767 9.15547 7.84488C8.86248 7.55209 8.3876 7.55226 8.09481 7.84525L3.52309 12.4202C3.35673 12.5577 3.25073 12.7657 3.25073 12.9984Z"
                  fill="currentColor"
                />
              </svg>
              Sign out
            </DropdownItem>
          </div>
        </Dropdown>
      </div>

      <Modal
        isOpen={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        showCloseButton={false}
        className="max-w-md p-6"
      >
        <div className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-error-50 dark:bg-error-500/10">
            <svg
              width="24"
              height="24"
              viewBox="0 0 24 24"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
              className="text-error-600 dark:text-error-500"
            >
              <path
                fillRule="evenodd"
                clipRule="evenodd"
                d="M15.1007 19.247C14.6865 19.247 14.3507 18.9112 14.3507 18.497L14.3507 14.245H12.8507V18.497C12.8507 19.7396 13.8581 20.747 15.1007 20.747H18.5007C19.7434 20.747 20.7507 19.7396 20.7507 18.497L20.7507 5.49609C20.7507 4.25345 19.7433 3.24609 18.5007 3.24609H15.1007C13.8581 3.24609 12.8507 4.25345 12.8507 5.49609V9.74501L14.3507 9.74501V5.49609C14.3507 5.08188 14.6865 4.74609 15.1007 4.74609L18.5007 4.74609C18.9149 4.74609 19.2507 5.08188 19.2507 5.49609L19.2507 18.497C19.2507 18.9112 18.9149 19.247 18.5007 19.247H15.1007ZM3.25073 11.9984C3.25073 12.2144 3.34204 12.4091 3.48817 12.546L8.09483 17.1556C8.38763 17.4485 8.86251 17.4487 9.15549 17.1559C9.44848 17.8631 9.44863 17.3882 9.15583 17.0952L5.81116 13.7484L16.0007 13.7484C16.4149 13.7484 16.7507 13.4127 16.7507 12.9984C16.7507 12.5842 16.4149 12.2484 16.0007 12.2484L5.81528 12.2484L9.15585 8.90554C9.44864 8.61255 9.44847 8.13767 9.15547 7.84488C8.86248 7.55209 8.3876 7.55226 8.09481 7.84525L3.52309 12.4202C3.35673 12.5577 3.25073 12.7657 3.25073 12.9984Z"
                fill="currentColor"
              />
            </svg>
          </div>
          <h3 className="mb-1 text-lg font-semibold text-gray-800 dark:text-white/90">
            Sign out?
          </h3>
          <p className="mb-5 text-sm text-gray-500 dark:text-gray-400">
            You will need to sign in again to access the admin console.
          </p>
          <div className="flex gap-3">
            <button
              onClick={() => setConfirmOpen(false)}
              className="flex-1 rounded-lg border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5"
            >
              Cancel
            </button>
            <button
              onClick={handleSignOut}
              disabled={signingOut}
              className="flex-1 rounded-lg bg-error-500 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-error-600 disabled:opacity-50"
            >
              {signingOut ? "Signing out…" : "Sign out"}
            </button>
          </div>
        </div>
      </Modal>
    </>
  );
}
