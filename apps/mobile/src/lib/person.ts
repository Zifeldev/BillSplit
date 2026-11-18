import type { Href } from 'expo-router';

import { formatMoney } from './format';

/** Route to the shared account with a person; the name pre-fills the header while loading. */
export function personHref(userId: string, name?: string): Href {
  const q = name ? `?name=${encodeURIComponent(name)}` : '';
  return `/person/${encodeURIComponent(userId)}${q}` as Href;
}

/** First name for friendly copy ("Bob" from "Bob Stone"). */
export function firstName(name: string | undefined, fallback = 'them'): string {
  return (name || '').trim().split(/\s+/)[0] || fallback;
}

/** "Bob owes you" / "You owe Bob" / "All settled" for a net (>0 — they owe you). */
export function netPhrase(netCents: number, name: string): string {
  if (netCents > 0) return `${name} owes you`;
  if (netCents < 0) return `You owe ${name}`;
  return 'All settled';
}

/** Friendly reminder text for the share sheet. */
export function reminderText(name: string, netCents: number, currency: string, groupNames: string[]): string {
  const amount = formatMoney(Math.abs(netCents), currency);
  const where =
    groupNames.length === 0
      ? ''
      : groupNames.length === 1
        ? ` for ${groupNames[0]}`
        : ` across ${groupNames.slice(0, -1).join(', ')} and ${groupNames[groupNames.length - 1]}`;
  return `Hey ${firstName(name, 'there')}! Quick friendly nudge from Billsplit: you owe me ${amount}${where}. No rush — settle up whenever it suits you`;
}
