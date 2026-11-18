import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useMemo, useState } from 'react';

import { errorMessage } from '@/api/client';
import { qk, useCategories } from '@/api/queries';
import { FinanceApi } from '@/api/services';
import type { Transaction, UpdateTransactionRequest } from '@/api/types';
import { CurrencyField } from '@/components/CurrencyField';
import {
  Banner,
  Button,
  Card,
  EmptyView,
  Field,
  Screen,
  Segmented,
  SelectField,
  ToggleRow,
  Txt,
} from '@/components/ui';
import { categoryOptions } from '@/lib/categories';
import { confirm } from '@/lib/dialogs';
import {
  amountPlaceholder,
  dateInputToRfc3339,
  minorToInput,
  parseMinor,
  tsToDateInput,
} from '@/lib/format';

function EditForm({ tx }: { tx: Transaction }) {
  const router = useRouter();
  const qc = useQueryClient();
  const categories = useCategories();
  const [kind, setKind] = useState<'expense' | 'income'>(tx.amountCents < 0 ? 'expense' : 'income');
  const [amount, setAmount] = useState(minorToInput(Math.abs(tx.amountCents), tx.currency));
  const [currency, setCurrency] = useState(tx.currency);
  const [merchant, setMerchant] = useState(tx.merchant);
  const [description, setDescription] = useState(tx.description);
  const [categoryId, setCategoryId] = useState(tx.categoryId);
  const [date, setDate] = useState(tsToDateInput(tx.occurredAt));
  const [remember, setRemember] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const options = useMemo(() => categoryOptions(categories.data?.categories), [categories.data]);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: qk.transactionsAll });
    void qc.invalidateQueries({ queryKey: qk.dashboardAll });
    void qc.invalidateQueries({ queryKey: qk.recurring });
    void qc.invalidateQueries({ queryKey: qk.budgets });
  };

  const save = useMutation({
    mutationFn: async () => {
      const abs = parseMinor(amount, currency);
      if (abs === null || abs <= 0) throw new Error('Enter a positive amount');
      const signed = kind === 'expense' ? -abs : abs;
      const req: UpdateTransactionRequest = { transactionId: tx.id };
      if (signed !== tx.amountCents || currency !== tx.currency) {
        req.amountCents = signed;
        req.currency = currency;
      }
      if (merchant.trim() !== tx.merchant) req.merchant = merchant.trim();
      if (description.trim() !== tx.description) req.description = description.trim();
      const categoryChanged = categoryId !== tx.categoryId;
      if (categoryChanged && !remember) req.categoryId = categoryId;
      if (date.trim() !== tsToDateInput(tx.occurredAt)) {
        const occurredAt = dateInputToRfc3339(date);
        if (!occurredAt) throw new Error('Date must be in YYYY-MM-DD format');
        req.occurredAt = occurredAt;
      }
      let updated = tx;
      if (Object.keys(req).length > 1) updated = await FinanceApi.updateTransaction(req);
      if (categoryChanged && remember) {
        // UpdateTransactionCategory can also remember merchant → category for future imports.
        updated = await FinanceApi.updateTransactionCategory({
          transactionId: tx.id,
          categoryId,
          rememberRule: true,
        });
      }
      return updated;
    },
    onSuccess: (updated) => {
      qc.setQueryData(qk.transaction(tx.id), updated);
      invalidate();
      router.back();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const del = useMutation({
    mutationFn: () => FinanceApi.deleteTransaction({ transactionId: tx.id }),
    onSuccess: () => {
      qc.removeQueries({ queryKey: qk.transaction(tx.id) });
      invalidate();
      router.back();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  return (
    <Screen>
      <Segmented
        options={[
          { value: 'expense', label: 'Expense' },
          { value: 'income', label: 'Income' },
        ]}
        value={kind}
        onChange={setKind}
      />
      {error ? <Banner kind="error" message={error} /> : null}
      <Card>
        <Field
          label="Amount"
          value={amount}
          onChangeText={setAmount}
          keyboardType="decimal-pad"
          placeholder={amountPlaceholder(currency)}
        />
        <CurrencyField label="Currency" value={currency} onChange={setCurrency} />
        <Field label="Merchant" value={merchant} onChangeText={setMerchant} placeholder="e.g. Starbucks" />
        <Field label="Description" value={description} onChangeText={setDescription} placeholder="Optional" />
        <SelectField label="Category" value={categoryId} options={options} onChange={setCategoryId} searchable />
        {categoryId !== tx.categoryId ? (
          <ToggleRow
            label="Remember for this merchant"
            hint={merchant.trim() ? `Future "${merchant.trim()}" transactions get this category` : undefined}
            value={remember}
            onChange={setRemember}
          />
        ) : null}
        <Field label="Date (YYYY-MM-DD)" value={date} onChangeText={setDate} placeholder="2026-01-31" autoCapitalize="none" />
        <Txt variant="small">Source: {tx.source || 'manual'}</Txt>
      </Card>
      <Button
        title="Save changes"
        icon="checkmark"
        loading={save.isPending}
        onPress={() => {
          setError(null);
          save.mutate();
        }}
      />
      <Button
        title="Delete transaction"
        icon="trash-outline"
        variant="danger"
        loading={del.isPending}
        onPress={async () => {
          const ok = await confirm('Delete transaction', 'This cannot be undone.', 'Delete', true);
          if (ok) del.mutate();
        }}
      />
    </Screen>
  );
}

export default function EditTransactionScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const txId = String(id ?? '');
  // Filled by the transactions list before navigating (there is no GetTransaction RPC).
  const tx = useQuery<Transaction | null>({
    queryKey: qk.transaction(txId),
    queryFn: () => null,
    enabled: false,
    initialData: null,
  });
  if (!tx.data) {
    return (
      <Screen>
        <EmptyView
          icon="list-outline"
          title="Transaction not loaded"
          message="Open the transaction from the list to edit it."
          action={<Button title="Go to transactions" small onPress={() => router.replace('/finance/transactions')} />}
        />
      </Screen>
    );
  }
  return <EditForm key={tx.data.id} tx={tx.data} />;
}
