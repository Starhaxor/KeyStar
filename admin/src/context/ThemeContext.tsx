"use client";

import type React from "react";
import { createContext, useContext, useEffect, useState, useSyncExternalStore } from "react";

export type Theme = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

type ThemeContextType = {
  theme: Theme;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
};

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

function readStoredTheme(): Theme {
  const saved = localStorage.getItem("theme");
  return saved === "light" || saved === "dark" ? saved : "system";
}

function getServerTheme(): Theme { return "system"; }
function getServerSystemTheme(): ResolvedTheme { return "light"; }

function subscribeToStoredTheme(onStoreChange: () => void) {
  const onStorage = (event: StorageEvent) => {
    if (event.key === "theme") onStoreChange();
  };
  window.addEventListener("storage", onStorage);
  return () => window.removeEventListener("storage", onStorage);
}

function getSystemTheme(): ResolvedTheme {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function subscribeToSystemTheme(onStoreChange: () => void) {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  media.addEventListener("change", onStoreChange);
  return () => media.removeEventListener("change", onStoreChange);
}

export const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  // The server snapshot is also used for the first browser render, preventing
  // localStorage from changing the icon tree while React hydrates it.
  const storedTheme = useSyncExternalStore(subscribeToStoredTheme, readStoredTheme, getServerTheme);
  const systemTheme = useSyncExternalStore(subscribeToSystemTheme, getSystemTheme, getServerSystemTheme);
  const [pendingTheme, setPendingTheme] = useState<Theme | null>(null);
  const theme = pendingTheme ?? storedTheme;
  const resolvedTheme = theme === "system" ? systemTheme : theme;

  useEffect(() => {
    document.documentElement.classList.toggle("dark", resolvedTheme === "dark");
  }, [resolvedTheme]);

  const setTheme = (next: Theme) => {
    localStorage.setItem("theme", next);
    setPendingTheme(next);
  };

  const toggleTheme = () => {
    setTheme(theme === "system" ? (resolvedTheme === "dark" ? "light" : "dark") : theme === "dark" ? "light" : "dark");
  };

  return <ThemeContext.Provider value={{ theme, resolvedTheme, setTheme, toggleTheme }}>{children}</ThemeContext.Provider>;
};

export const useTheme = () => {
  const context = useContext(ThemeContext);
  if (context === undefined) throw new Error("useTheme must be used within a ThemeProvider");
  return context;
};
