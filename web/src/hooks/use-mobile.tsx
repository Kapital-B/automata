import * as React from "react";

const MOBILE_BREAKPOINT = 768;
/** Matches Tailwind `lg:` (1024px) — layout stacks below this width. */
const LG_BREAKPOINT = 1024;

export function useIsMobile() {
  const [isMobile, setIsMobile] = React.useState<boolean | undefined>(undefined);

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`);
    const onChange = () => {
      setIsMobile(window.innerWidth < MOBILE_BREAKPOINT);
    };
    mql.addEventListener("change", onChange);
    setIsMobile(window.innerWidth < MOBILE_BREAKPOINT);
    return () => mql.removeEventListener("change", onChange);
  }, []);

  return !!isMobile;
}

export function useIsBelowLg() {
  const [isBelow, setIsBelow] = React.useState<boolean>(() =>
    typeof window !== "undefined" ? window.matchMedia(`(max-width: ${LG_BREAKPOINT - 1}px)`).matches : false,
  );

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${LG_BREAKPOINT - 1}px)`);
    const onChange = () => {
      setIsBelow(mql.matches);
    };
    mql.addEventListener("change", onChange);
    setIsBelow(mql.matches);
    return () => mql.removeEventListener("change", onChange);
  }, []);

  return isBelow;
}
