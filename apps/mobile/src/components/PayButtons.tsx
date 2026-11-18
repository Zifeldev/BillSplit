import React from 'react';

import type { PaymentHandles } from '@/api/types';
import { confirm } from '@/lib/dialogs';
import { formatMoney } from '@/lib/format';
import { availablePayMethods, PAY_LABEL, PayMethod, startPayment } from '@/lib/payments';
import { spacing } from '@/theme';

import { Button, IconName, Row } from './ui';

const ICON: Record<PayMethod, IconName> = {
  venmo: 'logo-venmo',
  paypal: 'logo-paypal',
  zelle: 'copy-outline',
};

/**
 * "Pay with Venmo / PayPal / Zelle" buttons for a debt you owe. After the
 * payment app opens, asks "Mark as paid?" and calls `onMarkPaid` on yes.
 */
export function PayButtons({
  handles,
  amountCents,
  currency,
  note,
  recipientName,
  onMarkPaid,
}: {
  handles: PaymentHandles | null | undefined;
  amountCents: number;
  currency: string;
  note: string;
  recipientName: string;
  onMarkPaid?: () => void;
}) {
  const methods = availablePayMethods(handles);
  if (!handles || methods.length === 0 || amountCents <= 0) return null;
  return (
    <Row gap={spacing.sm} style={{ flexWrap: 'wrap' }}>
      {methods.map((m) => (
        <Button
          key={m}
          small
          variant="ghost"
          icon={ICON[m]}
          title={PAY_LABEL[m]}
          onPress={async () => {
            const opened = await startPayment(m, handles, amountCents, currency, note);
            if (!opened || !onMarkPaid) return;
            const ok = await confirm(
              'Mark as paid?',
              `Did you pay ${recipientName} ${formatMoney(amountCents, currency)} via ${PAY_LABEL[m]}?`,
              'Mark as paid',
            );
            if (ok) onMarkPaid();
          }}
        />
      ))}
    </Row>
  );
}
