const formatters = new Map<string, Intl.NumberFormat>();

function formatter(currency: string): Intl.NumberFormat | null {
  const code = (currency || 'USD').toUpperCase();
  let f = formatters.get(code);
  if (!f) {
    try {
      f = new Intl.NumberFormat(undefined, { style: 'currency', currency: code });
    } catch {
      return null;
    }
    formatters.set(code, f);
  }
  return f;
}

const digitsCache = new Map<string, number>();

/**
 * Number of minor-unit digits for a currency (2 for USD, 0 for JPY/KRW, 3 for KWD).
 * Source of truth: Intl's resolved maximumFractionDigits; falls back to 2.
 */
export function minorDigits(currency: string): number {
  const code = (currency || 'USD').toUpperCase();
  const cached = digitsCache.get(code);
  if (cached !== undefined) return cached;
  let d = 2;
  try {
    const resolved = new Intl.NumberFormat(undefined, { style: 'currency', currency: code }).resolvedOptions();
    if (typeof resolved.maximumFractionDigits === 'number') d = resolved.maximumFractionDigits;
  } catch {
    d = 2;
  }
  digitsCache.set(code, d);
  return d;
}

/** Minor units → major units as a number (1234 USD → 12.34, 1234 JPY → 1234). */
export function minorToMajor(minor: number, currency: string): number {
  return (Number(minor) || 0) / 10 ** minorDigits(currency);
}

/** Minor units → plain decimal string with '.' ("12.34"), e.g. for payment links. */
export function minorToDecimal(minor: number, currency: string): string {
  return minorToMajor(minor, currency).toFixed(minorDigits(currency));
}

const wholeFormatters = new Map<string, Intl.NumberFormat | null>();

/**
 * Formats an amount in minor units of `currency`. `whole` drops the fraction
 * (big headline numbers: "KZT 2,191,507" instead of a truncated "KZT 2,191,507.3…").
 */
export function formatMoney(minor: number, currency: string, whole = false): string {
  const value = minorToMajor(minor, currency);
  if (whole) {
    const code = (currency || 'USD').toUpperCase();
    if (!wholeFormatters.has(code)) {
      try {
        wholeFormatters.set(code, new Intl.NumberFormat(undefined, { style: 'currency', currency: code, maximumFractionDigits: 0, minimumFractionDigits: 0 }));
      } catch {
        wholeFormatters.set(code, null);
      }
    }
    const wf = wholeFormatters.get(code);
    return wf ? wf.format(Math.round(value)) : `${Math.round(value)} ${currency}`;
  }
  const f = formatter(currency);
  if (!f) return `${value.toFixed(minorDigits(currency))} ${currency}`;
  return f.format(value);
}

/** Formats with an explicit "+" for positive values. */
export function formatSigned(minor: number, currency: string, whole = false): string {
  const s = formatMoney(Math.abs(minor), currency, whole);
  if (minor > 0) return `+${s}`;
  if (minor < 0) return `−${s}`;
  return s;
}

/**
 * Parses user input like "12.50", "12,5", "1 234.00" into integer minor units of
 * `currency`. Rejects more fraction digits than the currency has. Returns null if invalid.
 */
export function parseMinor(input: string, currency: string): number | null {
  const digits = minorDigits(currency);
  const cleaned = input.replace(/[\s\u00a0]/g, '').replace(',', '.');
  if (!cleaned) return null;
  const re = digits > 0 ? new RegExp(`^-?\\d*(\\.\\d{0,${digits}})?$`) : /^-?\d*\.?$/;
  if (!re.test(cleaned) || cleaned === '-' || cleaned === '.' || cleaned === '-.') return null;
  const n = Number(cleaned.endsWith('.') ? cleaned.slice(0, -1) : cleaned);
  if (!Number.isFinite(n)) return null;
  return Math.round(n * 10 ** digits);
}

/** Minor units → editable string ("12.50" for USD, "1250" for JPY). */
export function minorToInput(minor: number, currency: string): string {
  if (!minor) return '';
  return minorToDecimal(minor, currency);
}

/** Placeholder for amount inputs ("0.00" / "0"). */
export function amountPlaceholder(currency: string): string {
  const d = minorDigits(currency);
  return d > 0 ? `0.${'0'.repeat(d)}` : '0';
}

export function formatPercent(ratio: number, digits = 0): string {
  return `${((Number(ratio) || 0) * 100).toFixed(digits)}%`;
}

export function formatDate(ts: string | null | undefined): string {
  if (!ts) return '';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return '';
  // Calendar dates (receipt purchase date, bank statement date) arrive as UTC
  // midnight; rendering them in a timezone west of UTC would show the previous day.
  const dateOnly = d.getUTCHours() === 0 && d.getUTCMinutes() === 0 && d.getUTCSeconds() === 0 && d.getUTCMilliseconds() === 0;
  return d.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    ...(dateOnly ? { timeZone: 'UTC' } : {}),
  });
}

export function monthLabel(year: number, month: number): string {
  return new Date(year, month - 1, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
}

export function shortMonth(yyyyMm: string): string {
  const [y, m] = yyyyMm.split('-').map(Number);
  if (!y || !m) return yyyyMm;
  return new Date(y, m - 1, 1).toLocaleDateString(undefined, { month: 'short' });
}

export function initials(name: string | undefined, fallback = '?'): string {
  const parts = (name || '').trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return fallback;
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

/** YYYY-MM-DD → RFC3339 at local noon (avoids TZ day shifts). Returns null if invalid. */
export function dateInputToRfc3339(input: string): string | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(input.trim());
  if (!m) return null;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), 12, 0, 0);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

export function todayInput(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export function tsToDateInput(ts: string | null | undefined): string {
  if (!ts) return '';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return '';
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export const COMMON_CURRENCIES = [
  'USD',
  'EUR',
  'GBP',
  'CAD',
  'AUD',
  'CHF',
  'JPY',
  'PLN',
  'SEK',
  'NOK',
  'CZK',
  'TRY',
  'KZT',
  'GEL',
  'RUB',
];

const symbols = new Map<string, string>();

/** Currency symbol as Intl renders it ("$", "₸", "€"); falls back to the code. */
export function currencySymbol(currency: string): string {
  const code = (currency || 'USD').toUpperCase();
  let s = symbols.get(code);
  if (s === undefined) {
    // Hermes (iOS/Android) has no `formatToParts`, so take what is left of "$0.00" without the number.
    s = formatter(code)?.format(0).replace(/[\d.,\s\u00A0\u202F]/g, '') || code;
    symbols.set(code, s);
  }
  return s;
}

/** Short money for chart axes: "$1.2k", "₸3.4M" (minor units in). */
export function compactMoney(minor: number, currency: string): string {
  const v = minorToMajor(minor, currency);
  const a = Math.abs(v);
  const f = (x: number, suffix: string) => (x >= 100 ? x.toFixed(0) : x.toFixed(1).replace(/\.0$/, '')) + suffix;
  const body = a >= 1e9 ? f(a / 1e9, 'B') : a >= 1e6 ? f(a / 1e6, 'M') : a >= 1e3 ? f(a / 1e3, 'k') : a.toFixed(0);
  const sym = currencySymbol(currency);
  return `${v < 0 ? '−' : ''}${sym}${/^[A-Za-z]+$/.test(sym) ? ' ' : ''}${body}`;
}

/** Market price in MAJOR units with precision that fits its size (0.0412, 91.49, 81 254). */
export function formatPrice(value: number, currency: string): string {
  const digits = value >= 1000 ? 0 : value >= 1 ? 2 : 4;
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: (currency || 'USD').toUpperCase(),
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(value);
  } catch {
    return `${value.toFixed(digits)} ${currency}`;
  }
}

/** Signed percentage from a PERCENT value (1.234 → "+1.23%"). */
export function formatChange(pct: number, digits = 2): string {
  const v = Number(pct) || 0;
  const s = Math.abs(v).toFixed(digits);
  return v > 0.005 ? `+${s}%` : v < -0.005 ? `−${s}%` : `${(0).toFixed(digits)}%`;
}

/** "2026-10-05" → "Oct 5" (date-only strings, no time zone shift). */
export function shortDay(ymd: string): string {
  const [y, m, d] = ymd.split('-').map(Number);
  if (!y || !m || !d) return ymd;
  return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });
}

/** "2026-10-05" → "Oct 2026". */
export function shortMonthYear(ymd: string): string {
  const [y, m] = ymd.split('-').map(Number);
  if (!y || !m) return ymd;
  return new Date(Date.UTC(y, m - 1, 1)).toLocaleDateString(undefined, { month: 'short', year: 'numeric', timeZone: 'UTC' });
}

/** Major units → minor units (12.34 USD → 1234). */
export function majorToMinor(major: number, currency: string): number {
  return Math.round((Number(major) || 0) * 10 ** minorDigits(currency));
}
