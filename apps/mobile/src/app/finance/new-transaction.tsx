import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'expo-router';
import React, { useMemo, useState } from 'react';

import { errorMessage } from '@/api/client';
import { qk, useCategories } from '@/api/queries';
import { FinanceApi } from '@/api/services';
import { useAuth } from '@/auth/AuthContext';
import { CurrencyField } from '@/components/CurrencyField';
import { Banner, Button, Card, Field, Screen, Segmented, SelectField, Txt } from '@/components/ui';
import { categoryOptions } from '@/lib/categories';
import { amountPlaceholder, dateInputToRfc3339, parseMinor, todayInput } from '@/lib/format';

export default function NewTransactionScreen() {
  const router = useRouter();
  const qc = useQueryClient();
  const { user } = useAuth();
  const categories = useCategories();
  const [kind, setKind] = useState<'expense' | 'income'>('expense');
  const [amount, setAmount] = useState('');
  const [merchant, setMerchant] = useState('');
  const [description, setDescription] = useState('');
  const [categoryId, setCategoryId] = useState('');
  const [date, setDate] = useState(todayInput());
  const [currency, setCurrency] = useState('');
  const [error, setError] = useState<string | null>(null);

  const cur = currency || user?.currency || 'USD';

  const options = useMemo(
    () => [{ value: '', label: 'Auto-categorize' }, ...categoryOptions(categories.data?.categories)],
    [categories.data],
  );

  const create = useMutation({
    mutationFn: () => {
      const cents = parseMinor(amount, cur);
      if (cents === null || cents <= 0) throw new Error('Enter a positive amount');
      const occurredAt = dateInputToRfc3339(date);
      if (!occurredAt) throw new Error('Date must be in YYYY-MM-DD format');
      return FinanceApi.createTransaction({
        amountCents: kind === 'expense' ? -cents : cents,
        merchant: merchant.trim(),
        description: description.trim(),
        categoryId: categoryId || undefined,
        occurredAt,
        currency,
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.transactionsAll });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
      void qc.invalidateQueries({ queryKey: qk.recurring });
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
        <Field label="Amount" value={amount} onChangeText={setAmount} keyboardType="decimal-pad" placeholder={amountPlaceholder(cur)} autoFocus />
        <CurrencyField
          label="Currency"
          value={currency}
          onChange={setCurrency}
          allowEmpty
          emptyLabel={`Home currency (${user?.currency ?? 'USD'})`}
        />
        <Field label="Merchant" value={merchant} onChangeText={setMerchant} placeholder="e.g. Starbucks" />
        <Field label="Description" value={description} onChangeText={setDescription} placeholder="Optional" />
        <SelectField label="Category" value={categoryId} options={options} onChange={setCategoryId} searchable />
        <Field label="Date (YYYY-MM-DD)" value={date} onChangeText={setDate} placeholder="2026-01-31" autoCapitalize="none" />
        <Txt variant="small">Leave category on auto — the server picks one from the merchant and your rules.</Txt>
      </Card>
      <Button title="Add transaction" icon="checkmark" loading={create.isPending} onPress={() => { setError(null); create.mutate(); }} />
    </Screen>
  );
}
