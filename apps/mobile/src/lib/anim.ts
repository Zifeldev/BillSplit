import { useEffect, useState } from 'react';

const easeOutCubic = (t: number) => 1 - Math.pow(1 - t, 3);

/**
 * 0 → 1 eased progress that runs once when `key` changes (cheap JS-driven
 * animation used for SVG charts and counters; works on native and web).
 */
export function useMountProgress(key: unknown = 0, duration = 700, enabled = true): number {
  const [p, setP] = useState(enabled ? 0 : 1);
  useEffect(() => {
    if (!enabled) return;
    let raf = 0;
    let start: number | null = null;
    const step = (ts: number) => {
      if (start === null) start = ts;
      const t = Math.min(1, (ts - start) / duration);
      setP(easeOutCubic(t));
      if (t < 1) raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [key, duration, enabled]);
  return enabled ? p : 1;
}
