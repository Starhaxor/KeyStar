"use client";

import { api, applicationCookieName, readCookie } from "@/lib/api";
import { nextSelectedApplicationID } from "@/lib/applicationSelection";
import type { Application } from "@/lib/types";
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

type ApplicationContextValue = {
  applications: Application[];
  selectedApplicationID: string | null;
  loading: boolean;
  selectApplication: (applicationID: string) => void;
  refresh: () => Promise<void>;
};

const ApplicationContext = createContext<ApplicationContextValue>({ applications: [], selectedApplicationID: null, loading: true, selectApplication: () => {}, refresh: async () => {} });

export function ApplicationProvider({ children }: { children: React.ReactNode }) {
  const [applications, setApplications] = useState<Application[]>([]);
  const [selectedApplicationID, setSelectedApplicationID] = useState<string | null>(() => readCookie(applicationCookieName));
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.applications();
      setApplications(response.items);
      setSelectedApplicationID((currentID) => {
        const nextID = nextSelectedApplicationID(currentID, response.items);
        if (nextID !== currentID) {
          if (nextID) {
            document.cookie = `${applicationCookieName}=${encodeURIComponent(nextID)}; Path=/; SameSite=Lax`;
          } else {
            document.cookie = `${applicationCookieName}=; Max-Age=0; Path=/; SameSite=Lax`;
          }
        }
        return nextID;
      });
    } catch {
      setApplications([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  const selectApplication = useCallback((applicationID: string) => {
    document.cookie = `${applicationCookieName}=${encodeURIComponent(applicationID)}; Path=/; SameSite=Lax`;
    setSelectedApplicationID(applicationID);
    window.location.reload();
  }, []);

  const value = useMemo(() => ({ applications, selectedApplicationID, loading, selectApplication, refresh }), [applications, selectedApplicationID, loading, selectApplication, refresh]);
  return <ApplicationContext.Provider value={value}>{children}</ApplicationContext.Provider>;
}

export function useApplication() { return useContext(ApplicationContext); }
