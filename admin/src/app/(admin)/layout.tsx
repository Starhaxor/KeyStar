"use client";

import { useSidebar } from "@/context/SidebarContext";
import {
  AdminIdentityProvider,
  useAdminIdentity,
} from "@/context/AdminIdentityContext";
import AppHeader from "@/layout/AppHeader";
import { ApplicationProvider } from "@/context/ApplicationContext";
import AppSidebar from "@/layout/AppSidebar";
import Backdrop from "@/layout/Backdrop";
import CommandPalette from "@/components/common/CommandPalette";
import Link from "next/link";
import React from "react";

function MfaEnrollmentBanner() {
  const { identity, loading } = useAdminIdentity();
  if (loading || !identity || identity.mfa_enrolled) return null;
  return (
    <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-warning-500/30 bg-warning-50 px-4 py-3 dark:bg-warning-500/[0.05]">
      <p className="text-sm text-gray-700 dark:text-gray-300">
        Two-factor authentication is required before you can access the
        console.
      </p>
      <Link
        href="/security"
        className="rounded-lg bg-warning-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-warning-600"
      >
        Set up MFA
      </Link>
    </div>
  );
}

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { isExpanded, isHovered, isMobileOpen } = useSidebar();

  // Dynamic class for main content margin based on sidebar state
  const mainContentMargin = isMobileOpen
    ? "ml-0"
    : isExpanded || isHovered
    ? "lg:ml-[290px]"
    : "lg:ml-[90px]";

  return (
    <AdminIdentityProvider>
      <ApplicationProvider>
      <div className="min-h-screen xl:flex">
        {/* Sidebar and Backdrop */}
        <AppSidebar />
        <Backdrop />
        {/* Main Content Area */}
        <div
          className={`flex-1 transition-all  duration-300 ease-in-out ${mainContentMargin}`}
        >
          {/* Header */}
          <AppHeader />
          {/* Page Content */}
          <div className="p-4 mx-auto max-w-(--breakpoint-2xl) md:p-6">
            <MfaEnrollmentBanner />
            {children}
          </div>
        </div>
      </div>
      <CommandPalette />
      </ApplicationProvider>
    </AdminIdentityProvider>
  );
}
