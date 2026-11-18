import * as Clipboard from 'expo-clipboard';
import * as Linking from 'expo-linking';
import { Platform } from 'react-native';

import type { PaymentHandles } from '@/api/types';

import { notify } from './dialogs';
import { minorToDecimal } from './format';

export type PayMethod = 'venmo' | 'paypal' | 'zelle';

export function availablePayMethods(h: PaymentHandles | null | undefined): PayMethod[] {
  if (!h) return [];
  const out: PayMethod[] = [];
  if (h.venmo?.trim()) out.push('venmo');
  if (h.paypal?.trim()) out.push('paypal');
  if (h.zelle?.trim()) out.push('zelle');
  return out;
}

export const PAY_LABEL: Record<PayMethod, string> = { venmo: 'Venmo', paypal: 'PayPal', zelle: 'Zelle' };

const clean = (s: string) => s.trim().replace(/^@/, '');

export function venmoUrls(user: string, amountMinor: number, currency: string, note: string) {
  const q = `txn=pay&amount=${encodeURIComponent(minorToDecimal(amountMinor, currency))}&note=${encodeURIComponent(note)}`;
  const u = encodeURIComponent(clean(user));
  return {
    app: `venmo://paycharge?txn=pay&recipients=${u}&amount=${encodeURIComponent(
      minorToDecimal(amountMinor, currency),
    )}&note=${encodeURIComponent(note)}`,
    web: `https://venmo.com/${u}?${q}`,
  };
}

export function paypalUrl(user: string, amountMinor: number, currency: string): string {
  return `https://paypal.me/${encodeURIComponent(clean(user))}/${minorToDecimal(amountMinor, currency)}${currency.toUpperCase()}`;
}

async function openWeb(url: string): Promise<void> {
  if (Platform.OS === 'web') {
    if (typeof window !== 'undefined') window.open(url, '_blank', 'noopener');
    return;
  }
  await Linking.openURL(url);
}

/**
 * Opens the payment app/site for `method`. Returns false when nothing could be
 * opened (the caller then should not ask "Mark as paid?").
 */
export async function startPayment(
  method: PayMethod,
  handles: PaymentHandles,
  amountMinor: number,
  currency: string,
  note: string,
): Promise<boolean> {
  try {
    if (method === 'venmo') {
      const urls = venmoUrls(handles.venmo, amountMinor, currency, note);
      if (Platform.OS !== 'web') {
        try {
          await Linking.openURL(urls.app);
          return true;
        } catch {
          // Venmo app not installed — fall back to the website.
        }
      }
      await openWeb(urls.web);
      return true;
    }
    if (method === 'paypal') {
      await openWeb(paypalUrl(handles.paypal, amountMinor, currency));
      return true;
    }
    const zelle = handles.zelle.trim();
    await Clipboard.setStringAsync(zelle);
    notify(
      'Zelle handle copied',
      `${zelle}\n\nOpen your bank app → Zelle → Send, paste the handle and send ${minorToDecimal(amountMinor, currency)} ${currency}.`,
    );
    return true;
  } catch (err) {
    notify('Could not open payment', err instanceof Error ? err.message : String(err));
    return false;
  }
}
