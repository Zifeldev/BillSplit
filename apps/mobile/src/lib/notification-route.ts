import type { Href } from 'expo-router';

/**
 * Where to go when a notification (inbox row or push) is tapped.
 * `data` keys come from the server verbatim (snake_case); camelCase is accepted too.
 */
export function notificationHref(data: Record<string, unknown> | null | undefined): Href {
  const pick = (...keys: string[]): string => {
    for (const k of keys) {
      const v = data?.[k];
      if (typeof v === 'string' && v) return v;
    }
    return '';
  };
  const receiptId = pick('receipt_id', 'receiptId');
  if (receiptId) return `/receipt/${encodeURIComponent(receiptId)}`;
  const groupId = pick('group_id', 'groupId');
  if (groupId) return `/group/${encodeURIComponent(groupId)}`;
  return '/friends';
}
