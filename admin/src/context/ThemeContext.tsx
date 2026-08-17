"use client";

import type React from "react";
import { createContext, useState, useContext, useEffect } from "react";

export type Theme = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

type ThemeContextType = {
  theme: Theme;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
};

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

function getSystemTheme(): ResolvedTheme {
  if (typeof window === "undefined") return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

export const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [theme, setThemeState] = useState<Theme>("system");
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>("light");
  const [isInitialized, setIsInitialized] = useState(false);

  useEffect(() => {
    const saved = localStorage.getItem("theme") as Theme | null;
    const initial: Theme =
      saved === "light" || saved === "dark" ? saved : "system";
    setThemeState(initial);
    setResolvedTheme(initial === "system" ? getSystemTheme() : initial);
    setIsInitialized(true);
  }, []);

  useEffect(() => {
    if (!isInitialized) return;
    if (theme === "system") {
      const media = window.matchMedia("(prefers-color-scheme: dark)");
      const update = () => setResolvedTheme(media.matches ? "dark" : "light");
      update();
      media.addEventListener("change", update);
      return () => media.removeEventListener("change", update);
    }
    setResolvedTheme(theme);
  }, [theme, isInitialized]);

  useEffect(() => {
    if (!isInitialized) return;
    localStorage.setItem("theme", theme);
    document.documentElement.classList.toggle(
      "dark",
      resolvedTheme === "dark"
    );
  }, [theme, resolvedTheme, isInitialized]);

  const setTheme = (next: Theme) => setThemeState(next);

  const toggleTheme = () => {
    // Plain light<->dark flip based on what is currently shown.
    setThemeState((prev) => {
      if (prev === "system") return resolvedTheme === "dark" ? "light" : "dark";
      return prev === "dark" ? "light" : "dark";
    });
  };

  return (
    <ThemeContext.Provider
      value={{ theme, resolvedTheme, setTheme, toggleTheme }}
    >
      {children}
    </ThemeContext.Provider>
  );
};

export const useTheme = () => {
  const context = useContext(ThemeContext);
  if (context === undefined) {
    throw new Error("useTheme must be used within a ThemeProvider");
  }
  return context;
};
