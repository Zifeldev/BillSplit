/**
 * Investment math for the calculators. All money is in MINOR units.
 *
 * The deposit model mirrors the finance service (internal/finance/deposit.go):
 * interest accrues monthly at rate/12 on the balance, joins the principal every
 * capitalization period, and the monthly top-up arrives at the start of each
 * following month (not in the last one).
 */
import type { IconName } from '@/components/Icon';
import type { Capitalization, InvestmentKind } from '@/api/types';

export const CAPITALIZATIONS: { value: Capitalization; label: string; every: number }[] = [
  { value: 'CAPITALIZATION_MONTHLY', label: 'Monthly', every: 1 },
  { value: 'CAPITALIZATION_QUARTERLY', label: 'Quarterly', every: 3 },
  { value: 'CAPITALIZATION_YEARLY', label: 'Yearly', every: 12 },
  { value: 'CAPITALIZATION_NONE', label: 'None', every: 0 },
];

function capEvery(c: Capitalization): number {
  return CAPITALIZATIONS.find((x) => x.value === c)?.every ?? 1;
}

/** Effective annual yield (percent) of a nominal rate with capitalization. */
export function effectiveRate(ratePct: number, cap: Capitalization): number {
  const every = capEvery(cap);
  if (every === 0 || ratePct <= 0) return ratePct;
  const periods = 12 / every;
  return (Math.pow(1 + ratePct / 100 / periods, periods) - 1) * 100;
}

export interface DepositInput {
  amount: number;
  ratePct: number;
  months: number;
  cap: Capitalization;
  topUp: number;
  /** Withholding tax on interest, percent (KZ deposits: 0; US: your bracket). */
  taxPct: number;
}

export interface DepositRow {
  month: number;
  value: number;
  contributed: number;
}

export interface DepositResult {
  rows: DepositRow[]; // month 0..months
  final: number;
  contributed: number;
  interest: number;
  tax: number;
  net: number;
  effective: number;
}

export function depositSchedule(inp: DepositInput): DepositResult {
  const months = Math.max(0, Math.min(600, Math.round(inp.months)));
  const r = inp.ratePct / 100 / 12;
  const every = capEvery(inp.cap);
  let balance = inp.amount;
  let accrued = 0;
  let contributed = inp.amount;
  const rows: DepositRow[] = [{ month: 0, value: balance, contributed }];
  for (let k = 1; k <= months; k++) {
    accrued += balance * r;
    if (every > 0 && k % every === 0) {
      balance += accrued;
      accrued = 0;
    }
    rows.push({ month: k, value: balance + accrued, contributed });
    if (k < months) {
      balance += inp.topUp;
      contributed += inp.topUp;
    }
  }
  const final = balance + accrued;
  const interest = final - contributed;
  const tax = Math.max(0, interest) * (Math.max(0, inp.taxPct) / 100);
  return { rows, final, contributed, interest, tax, net: final - tax, effective: effectiveRate(inp.ratePct, inp.cap) };
}

export interface CompoundInput {
  initial: number;
  monthly: number;
  returnPct: number;
  years: number;
  inflationPct: number;
  /** Annual fund fee / expense ratio, percent. */
  feePct: number;
}

export interface CompoundRow {
  year: number;
  value: number;
  contributed: number;
  real: number; // value in today's money
}

/** Monthly compounding, contribution at the end of each month; fee reduces the return. */
export function compoundSchedule(inp: CompoundInput): { rows: CompoundRow[]; final: number; contributed: number; growth: number; real: number } {
  const years = Math.max(0, Math.min(60, Math.round(inp.years)));
  const r = (inp.returnPct - inp.feePct) / 100 / 12;
  let v = inp.initial;
  let contributed = inp.initial;
  const rows: CompoundRow[] = [{ year: 0, value: v, contributed, real: v }];
  for (let y = 1; y <= years; y++) {
    for (let m = 0; m < 12; m++) {
      v = v * (1 + r) + inp.monthly;
      contributed += inp.monthly;
    }
    rows.push({ year: y, value: v, contributed, real: v / Math.pow(1 + inp.inflationPct / 100, y) });
  }
  const last = rows[rows.length - 1];
  return { rows, final: last.value, contributed: last.contributed, growth: last.value - last.contributed, real: last.real };
}

/** Monthly contribution needed to reach `target` in `years` at `returnPct` (end-of-month payments). */
export function monthlyForTarget(target: number, initial: number, returnPct: number, years: number): number {
  const n = Math.round(years * 12);
  if (n <= 0) return Math.max(0, target - initial);
  const r = returnPct / 100 / 12;
  const grown = initial * Math.pow(1 + r, n);
  const left = target - grown;
  if (left <= 0) return 0;
  return r === 0 ? left / n : (left * r) / (Math.pow(1 + r, n) - 1);
}

export type LoanType = 'annuity' | 'differentiated';

export interface LoanResult {
  balances: number[]; // remaining principal, month 0..n
  payments: number[]; // payment of month 1..n
  first: number;
  last: number;
  totalPaid: number;
  interest: number;
}

export function loanSchedule(amount: number, ratePct: number, months: number, type: LoanType): LoanResult {
  const n = Math.max(1, Math.min(600, Math.round(months)));
  const r = ratePct / 100 / 12;
  const annuity = r === 0 ? amount / n : (amount * r) / (1 - Math.pow(1 + r, -n));
  let bal = amount;
  const balances = [bal];
  const payments: number[] = [];
  for (let k = 1; k <= n; k++) {
    const interest = bal * r;
    const principal = type === 'annuity' ? annuity - interest : amount / n;
    payments.push(principal + interest);
    bal = Math.max(0, bal - principal);
    balances.push(bal);
  }
  const totalPaid = payments.reduce((s, x) => s + x, 0);
  return { balances, payments, first: payments[0], last: payments[payments.length - 1], totalPaid, interest: totalPaid - amount };
}

// ---------- investment kinds ----------

export const KINDS: { value: InvestmentKind; label: string; icon: IconName; market: boolean }[] = [
  { value: 'INVESTMENT_KIND_DEPOSIT', label: 'Deposit', icon: 'business', market: false },
  { value: 'INVESTMENT_KIND_STOCK', label: 'Stock', icon: 'trending-up', market: true },
  { value: 'INVESTMENT_KIND_ETF', label: 'ETF / fund', icon: 'pie-chart', market: true },
  { value: 'INVESTMENT_KIND_CRYPTO', label: 'Crypto', icon: 'logo-bitcoin', market: true },
  { value: 'INVESTMENT_KIND_BOND', label: 'Bond', icon: 'document-text', market: false },
  { value: 'INVESTMENT_KIND_METAL', label: 'Metal', icon: 'diamond', market: true },
  { value: 'INVESTMENT_KIND_OTHER', label: 'Other', icon: 'cube', market: false },
];

export function kindMeta(k: InvestmentKind) {
  return KINDS.find((x) => x.value === k) ?? KINDS[KINDS.length - 1];
}

/** Ticker suggestions per kind for the add form. */
export const SUGGESTED: Partial<Record<InvestmentKind, { symbol: string; name: string }[]>> = {
  INVESTMENT_KIND_STOCK: [
    { symbol: 'KSPI', name: 'Kaspi.kz' },
    { symbol: 'AAPL', name: 'Apple' },
    { symbol: 'NVDA', name: 'NVIDIA' },
    { symbol: 'MSFT', name: 'Microsoft' },
  ],
  INVESTMENT_KIND_ETF: [
    { symbol: 'VOO', name: 'Vanguard S&P 500' },
    { symbol: 'QQQ', name: 'Invesco Nasdaq 100' },
    { symbol: 'VT', name: 'Vanguard Total World' },
  ],
  INVESTMENT_KIND_CRYPTO: [
    { symbol: 'BTC-USD', name: 'Bitcoin' },
    { symbol: 'ETH-USD', name: 'Ethereum' },
    { symbol: 'SOL-USD', name: 'Solana' },
  ],
  INVESTMENT_KIND_METAL: [
    { symbol: 'GC=F', name: 'Gold (oz)' },
    { symbol: 'SI=F', name: 'Silver (oz)' },
  ],
};

/**
 * Allocation colors per kind, fixed order (validated with the dataviz palette
 * checker for CVD separation in both themes): deposit, stock, crypto, ETF,
 * bond, other, metal.
 */
const KIND_COLORS: Record<InvestmentKind, [light: string, dark: string]> = {
  INVESTMENT_KIND_DEPOSIT: ['#14966A', '#1BAF7A'],
  INVESTMENT_KIND_STOCK: ['#5B5FE8', '#6E72F2'],
  INVESTMENT_KIND_CRYPTO: ['#C27C05', '#BA800A'],
  INVESTMENT_KIND_ETF: ['#0B8AC9', '#1E9FE0'],
  INVESTMENT_KIND_BOND: ['#D93D77', '#E5508A'],
  INVESTMENT_KIND_OTHER: ['#8B5CF6', '#9B6CF5'],
  INVESTMENT_KIND_METAL: ['#B4652F', '#C0703A'],
  INVESTMENT_KIND_UNSPECIFIED: ['#8B5CF6', '#9B6CF5'],
};

export function kindColor(k: InvestmentKind, dark: boolean): string {
  return KIND_COLORS[k][dark ? 1 : 0];
}

/** Best guess of the investment kind from a Yahoo ticker. */
export function guessKind(symbol: string): InvestmentKind {
  if (/-USD$|-EUR$|-USDT$/.test(symbol)) return 'INVESTMENT_KIND_CRYPTO';
  if (/^(GC|SI|PL|PA)=F$/.test(symbol)) return 'INVESTMENT_KIND_METAL';
  return 'INVESTMENT_KIND_STOCK';
}
