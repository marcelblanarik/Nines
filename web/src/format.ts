/** Formats a fraction as a percentage the way the CLI does: 3 significant digits of unavailability, at least 2 decimals. */
export function pct(f: number): string {
  const u = (1 - f) * 100;
  const decimals = u > 0 ? Math.min(9, Math.max(2, 2 - Math.floor(Math.log10(u)))) : 2;
  let s = (f * 100).toFixed(decimals);
  while (s.endsWith("0") && s[s.length - 3] !== ".") s = s.slice(0, -1);
  return s + "%";
}

/** Downtime a fractional availability allows over a window of `days`. */
export function downtime(avail: number, days: number): string {
  const m = (1 - avail) * days * 24 * 60;
  if (m >= 100) return `${m.toFixed(0)} min`;
  if (m >= 1) return `${m.toFixed(1)} min`;
  return `${m.toFixed(2)} min`;
}

// The model only accepts availabilities above 50%, so the slider starts at 50.1.
const MIN_PERCENT = 50.1;
const STEP_PERCENT = 0.1;
const MAX_PERCENT = 99.999;
// Whole 0.1 steps end at 99.9; one more position jumps to the 99.999 maximum.
const LAST_STEP_PERCENT = 99.9;
const LAST_POSITION = Math.round((LAST_STEP_PERCENT - MIN_PERCENT) / STEP_PERCENT) + 1;

/** Slider positions are whole numbers: each is a 0.1 step, and the last one is 99.999%. */
export const SLIDER = { min: 0, max: LAST_POSITION, step: 1 } as const;

export function percentFromPosition(position: number): number {
  if (position >= LAST_POSITION) return MAX_PERCENT;
  return Number((MIN_PERCENT + Math.max(0, position) * STEP_PERCENT).toFixed(1));
}

/** The nearest slider position; anything at or above 99.95% sits on the 99.999% end. */
export function positionFromPercent(percent: number): number {
  if (percent >= (LAST_STEP_PERCENT + MAX_PERCENT) / 2) return LAST_POSITION;
  return Math.min(LAST_POSITION - 1, Math.max(0, Math.round((percent - MIN_PERCENT) / STEP_PERCENT)));
}
