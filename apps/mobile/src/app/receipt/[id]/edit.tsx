import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useReceipt } from '@/api/queries';
import { ReceiptApi } from '@/api/services';
import type { ItemInput, Receipt } from '@/api/types';
import { CurrencyField } from '@/components/CurrencyField';
import {
  Banner,
  Button,
  Card,
  ErrorView,
  Field,
  IconButton,
  SkeletonList,
  Row,
  Screen,
  SectionHeader,
  Txt,
} from '@/components/ui';
import { amountPlaceholder, formatMoney, minorToInput, parseMinor } from '@/lib/format';
import { useTheme } from '@/theme';

interface DraftItem {
  key: string;
  name: string;
  qty: string;
  price: string;
  /** As printed on the receipt; shown as a hint only (not editable). */
  raw?: string;
}

let seq = 0;
const blank = (): DraftItem => ({ key: `n${++seq}`, name: '', qty: '1', price: '' });

function EditForm({ receipt }: { receipt: Receipt }) {
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const [store, setStore] = useState(receipt.store);
  const [tax, setTax] = useState(minorToInput(receipt.taxCents, receipt.currency));
  const [tip, setTip] = useState(minorToInput(receipt.tipCents, receipt.currency));
  const [currency, setCurrency] = useState(receipt.currency);
  const [items, setItems] = useState<DraftItem[]>(() =>
    (receipt.items ?? []).length > 0
      ? receipt.items.map((it) => ({
          key: it.id,
          name: it.name,
          qty: String(it.quantity || 1),
          price: minorToInput(it.priceCents, receipt.currency),
          raw: it.rawName && it.rawName.trim().toLowerCase() !== it.name.trim().toLowerCase() ? it.rawName.trim() : undefined,
        }))
      : [blank()],
  );
  const [error, setError] = useState<string | null>(null);

  const cur = currency || receipt.currency || 'USD';

  const update = (key: string, patch: Partial<DraftItem>) =>
    setItems((prev) => prev.map((it) => (it.key === key ? { ...it, ...patch } : it)));

  const validate = (): ItemInput[] | string => {
    const out: ItemInput[] = [];
    for (const it of items) {
      if (!it.name.trim() && !it.price.trim()) continue;
      const price = parseMinor(it.price, cur);
      const qty = Number.parseInt(it.qty || '1', 10);
      if (!it.name.trim()) return 'Every item needs a name';
      if (price === null || price < 0) return `Invalid price for "${it.name}"`;
      if (!Number.isFinite(qty) || qty < 1) return `Invalid quantity for "${it.name}"`;
      out.push({ name: it.name.trim(), quantity: qty, priceCents: price });
    }
    if (out.length === 0) return 'Add at least one item';
    return out;
  };

  const save = useMutation({
    mutationFn: () => {
      const list = validate();
      if (typeof list === 'string') throw new Error(list);
      const taxCents = parseMinor(tax, cur);
      const tipCents = parseMinor(tip, cur);
      if (tax.trim() && taxCents === null) throw new Error('Invalid tax');
      if (tip.trim() && tipCents === null) throw new Error('Invalid tip');
      return ReceiptApi.updateItems({
        receiptId: receipt.id,
        items: list,
        store: store.trim(),
        taxCents: taxCents ?? 0,
        tipCents: tipCents ?? 0,
        currency,
      });
    },
    onSuccess: (updated) => {
      qc.setQueryData(qk.receipt(receipt.id), updated);
      void qc.invalidateQueries({ queryKey: qk.split(receipt.id) });
      void qc.invalidateQueries({ queryKey: qk.receipts(receipt.groupId) });
      router.back();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const subtotal = items.reduce((s, it) => s + (parseMinor(it.price, cur) ?? 0), 0);
  const total = subtotal + (parseMinor(tax, cur) ?? 0) + (parseMinor(tip, cur) ?? 0);

  return (
    <Screen>
      {error ? <Banner kind="error" message={error} /> : null}
      <Banner kind="info" message="Saving replaces all items. Existing claims on changed items may be reset." />
      <Card>
        <Field label="Store" value={store} onChangeText={setStore} placeholder="Store" />
        <CurrencyField label="Currency" value={currency} onChange={setCurrency} />
      </Card>
      <SectionHeader title="Items" action="+ Add item" onAction={() => setItems((p) => [...p, blank()])} />
      <Card>
        <Row gap={6}>
          <Txt variant="label" style={styles.flex3}>
            Name
          </Txt>
          <Txt variant="label" style={styles.qty}>
            Qty
          </Txt>
          <Txt variant="label" style={styles.flex1}>
            Line total
          </Txt>
          <Txt variant="label" style={styles.spacer}>
            {' '}
          </Txt>
        </Row>
        {items.map((it) => (
          <View key={it.key} style={styles.itemBlock}>
          <Row gap={6}>
            <Field style={styles.flex3} placeholder="Item" value={it.name} onChangeText={(v) => update(it.key, { name: v })} />
            <Field
              style={styles.qty}
              keyboardType="number-pad"
              value={it.qty}
              onChangeText={(v) => update(it.key, { qty: v.replace(/\D/g, '') })}
            />
            <Field
              style={styles.flex1}
              keyboardType="decimal-pad"
              placeholder={amountPlaceholder(cur)}
              value={it.price}
              onChangeText={(v) => update(it.key, { price: v })}
            />
            <IconButton
              icon="trash-outline"
              color={c.danger}
              label="Remove"
              onPress={() => setItems((p) => (p.length > 1 ? p.filter((x) => x.key !== it.key) : [blank()]))}
            />
          </Row>
          {it.raw ? (
            <Txt variant="small" style={styles.raw} numberOfLines={1}>
              On receipt: {it.raw}
            </Txt>
          ) : null}
          </View>
        ))}
      </Card>
      <Card>
        <Row gap={8}>
          <Field style={styles.flex1} label="Tax" keyboardType="decimal-pad" placeholder={amountPlaceholder(cur)} value={tax} onChangeText={setTax} />
          <Field style={styles.flex1} label="Tip" keyboardType="decimal-pad" placeholder={amountPlaceholder(cur)} value={tip} onChangeText={setTip} />
        </Row>
        <Row>
          <Txt variant="muted" style={styles.flex1}>
            Total
          </Txt>
          <Txt weight="800">{formatMoney(total, cur)}</Txt>
        </Row>
      </Card>
      <Button title="Save items" icon="checkmark" loading={save.isPending} onPress={() => { setError(null); save.mutate(); }} />
    </Screen>
  );
}

export default function EditReceiptScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const receipt = useReceipt(String(id ?? ''));
  if (!receipt.data) {
    return (
      <Screen>
        {receipt.error ? <ErrorView error={receipt.error} onRetry={() => void receipt.refetch()} /> : <SkeletonList rows={4} />}
      </Screen>
    );
  }
  return <EditForm receipt={receipt.data} />;
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  flex3: { flex: 3 },
  qty: { width: 56 },
  spacer: { width: 44 },
  itemBlock: { gap: 2 },
  raw: { marginLeft: 4, fontFamily: 'monospace', fontSize: 11.5 },
});
