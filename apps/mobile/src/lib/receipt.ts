import type { ReceiptStatus } from '@/api/types';

export function statusLabel(s: ReceiptStatus): { label: string; tone: 'muted' | 'warn' | 'ok' | 'bad' } {
  switch (s) {
    case 'RECEIPT_STATUS_PENDING':
      return { label: 'Processing', tone: 'warn' };
    case 'RECEIPT_STATUS_PARSED':
      return { label: 'Open', tone: 'ok' };
    case 'RECEIPT_STATUS_FAILED':
      return { label: 'Needs review', tone: 'bad' };
    case 'RECEIPT_STATUS_FINALIZED':
      return { label: 'Finalized', tone: 'muted' };
    default:
      return { label: 'Unknown', tone: 'muted' };
  }
}
