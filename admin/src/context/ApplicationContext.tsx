"use client";

import { api, applicationCookieName, readCookie } from "@/lib/api";
import type { Application } from "@/lib/types";
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

type ApplicationContextValue = {
  applications: Application[];
  selectedApplicationID: string | null;
  loading: boolean;
  selectApplication: (applicationID: string) => void;
};

const ApplicationContext = createContext<ApplicationContextValue>({ applications: [], selectedApplicationID: null, loading: true, selectApplication: () => {} });

export function ApplicationProvider({ children }: { children: React.ReactNode }) {
  const [applications, setApplications] = useState<Application[]>([]);
  const [selectedApplicationID, setSelectedApplicationID] = useState<string | null>(() => readCookie(applicationCookieName));
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    void api.applications().then((response) => {
      setApplications(response.items);
      if (!selectedApplicationID && response.items.length === 1) setSelectedApplicationID(response.items[0].id);
    }).catch(() => setApplications([])).finally(() => setLoading(false));
  }, [selectedApplicationID]);

  const selectApplication = useCallback((applicationID: string) => {
    document.cookie = `${applicationCookieName}=${encodeURIComponent(applicationID)}; Path=/; SameSite=Lax`;
    setSelectedApplicationID(applicationID);
    window.location.reload();
  }, []);

  const value = useMemo(() => ({ applications, selectedApplicationID, loading, selectApplication }), [applications, selectedApplicationID, loading, selectApplication]);
  return <ApplicationContext.Provider value={value}>{children}</ApplicationContext.Provider>;
}

export function useApplication() { return useContext(ApplicationContext); }
