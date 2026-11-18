import type { IconName } from '@/components/ui';

export interface Step {
  title: string;
  body: string;
}

export interface Feature {
  icon: IconName;
  title: string;
  body: string;
}

/** The real order of a split, so the cards carry their step number. */
export const STEPS: Step[] = [
  {
    title: 'Scan the receipt',
    body: 'Take a photo or upload a PDF. Items, tax and tip are read for you — or type the amount in yourself.',
  },
  {
    title: 'Everyone claims their items',
    body: 'Friends tap what they had and see each other’s choices appear live. Shared dishes split between whoever shared them.',
  },
  {
    title: 'Settle up',
    body: 'billsplit works out the fewest transfers that clear every debt. Pay back with Venmo, PayPal or Zelle.',
  },
];

export const FEATURES: Feature[] = [
  {
    icon: 'git-compare-outline',
    title: 'Split any way',
    body: 'Equally, by exact amounts, by percentage, by shares or item by item.',
  },
  {
    icon: 'scan-outline',
    title: 'Receipt scanning',
    body: 'Snap a photo and assign each line to the people who had it.',
  },
  {
    icon: 'people-outline',
    title: 'Groups and trips',
    body: 'Keep a running tab for flatmates and trips, with one balance per friend across all of them.',
  },
  {
    icon: 'globe-outline',
    title: 'Any currency',
    body: 'Log a bill in the local currency. Debts convert at the rate on the day you paid.',
  },
  {
    icon: 'pulse-outline',
    title: 'Fewer payments',
    body: 'Debts inside a group are simplified so everyone settles in the fewest steps.',
  },
  {
    icon: 'wallet-outline',
    title: 'Your own money too',
    body: 'Import bank statements, set budgets and savings goals, and see where the month went.',
  },
];
