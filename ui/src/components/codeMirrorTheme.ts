/**
 * @module React hook that syncs CodeMirror theme with document-level dark mode preference.
 */
import { useEffect, useState } from "react";

function readThemeClass(): "light" | "dark" {
  if (
    typeof document !== "undefined" &&
    document.documentElement.classList.contains("dark")
  ) {
    return "dark";
  }
  return "light";
}

/**
 * Monitors the document root's class attribute for theme changes and syncs the CodeMirror theme accordingly. Observes the element for mutations and updates reactively.
 * @returns The current theme: "light" or "dark".
 */
export function useCodeMirrorTheme(): "light" | "dark" {
  const [theme, setTheme] = useState<"light" | "dark">(() => readThemeClass());

  useEffect(() => {
    if (typeof document === "undefined") return;

    const root = document.documentElement;
    const observer = new MutationObserver(() => {
      setTheme(readThemeClass());
    });
    observer.observe(root, { attributes: true, attributeFilter: ["class"] });

    return () => observer.disconnect();
  }, []);

  return theme;
}
