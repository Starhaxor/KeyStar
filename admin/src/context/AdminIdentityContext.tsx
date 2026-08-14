"use client";
import { api } from "@/lib/api";
import type { AdminIdentity } from "@/lib/types";
import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";

// AdminIdentityContext loads GET /v1/admin/me once per console session and
// exposes the caller's role, permission set and MFA state to layout, sidebar
// and pages so the UI can gate itself on the same rules the backend enforces.

interface AdminIdentityContextValue {
  identity: AdminIdentity | null;
  loading: boolean;
  hasPermission: (permission: string) => boolean;
  refresh: () => Promise<void>;
}

const AdminIdentityContext = createContext<AdminIdentityContextValue>({
  identity: null,
  loading: true,
  hasPermission: () => false,
  refresh: async () => {},
});

export function AdminIdentityProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [identity, setIdentity] = useState<AdminIdentity | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const response = await api.me();
      setIdentity(response);
    } catch {
      // Auth failures redirect via the API client; keep the last state here.
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const value = useMemo<AdminIdentityContextValue>(
    () => ({
      identity,
      loading,
      hasPermission: (permission: string) =>
        Boolean(identity?.permissions?.includes(permission)),
      refresh,
    }),
    [identity, loading, refresh]
  );

  return (
    <AdminIdentityContext.Provider value={value}>
      {children}
    </AdminIdentityContext.Provider>
  );
}

export function useAdminIdentity() {
  return useContext(AdminIdentityContext);
}
