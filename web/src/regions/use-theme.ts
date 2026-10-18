import { useCallback, useEffect, useState } from "react";
import type { Theme } from "@/components/arc/theme-switch/theme-switch";

const themeOf = (): Theme =>
  document.documentElement.dataset.theme === "light" ? "light" : "dark";

/**
 * Arc drives dark mode with data-theme on <html>. The attribute is read after mount
 * (a browser value, so not during first paint), then kept in React state.
 */
export function useTheme() {
  const [theme, setThemeState] = useState<Theme>("dark");

  useEffect(() => setThemeState(themeOf()), []);

  const setTheme = useCallback((next: Theme) => {
    document.documentElement.dataset.theme = next;
    setThemeState(next);
  }, []);

  return { theme, setTheme };
}
