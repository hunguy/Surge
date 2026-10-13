import { useEffect, useState } from "react";

/**
 * Tracks one media query. The first render always answers false, so the initial
 * paint is the same on every pass; the real value lands in an effect.
 */
export function useMediaQuery(next: string): boolean {
  const [matches, setMatches] = useState(false);

  useEffect(() => {
    const list = window.matchMedia(next);
    const update = () => setMatches(list.matches);
    update();
    list.addEventListener("change", update);
    return () => list.removeEventListener("change", update);
  }, [next]);

  return matches;
}

/** Phone widths get the bottom sheet, wider surfaces get the dialog. */
export const useIsPhone = () => useMediaQuery("(max-width: 767px)");
