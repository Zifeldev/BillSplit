import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useNetWorth } from '@/api/queries';
import { FinanceApi } from '@/api/services';
import type { Account, AccountKind } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { CurrencyField } from '@/components/CurrencyField';
import { Banner, Button, Card, Chip, Field, Row, Screen, Txt } from '@/components/ui';
import { confirm } from '@/lib/dialogs';
import { amountPlaceholder, minorToInput, parseMinor } from '@/lib/format';
import { ACCOUNT_KINDS, accountMeta } from '@/lib/networth';


function Form({ acc }: { acc: Account | null }) {
  const router = useRouter();
  const qc = useQueryClient();
  const { user } = useAuth();
  const [kind, setKind] = useState<AccountKind>(acc?.kind ?? 'ACCOUNT_KIND_CARD');
  const [name, setName] = useState(acc?.name ?? '');
  const [currency, setCurrency] = useState(acc?.currency ?? user?.currency ?? 'USD');
  const [balance, setBalance] = useState(acc ? minorToInput(acc.balanceCents, acc.currency) : '');
  const [error, setError] = useState<string | null>(null);
  const meta = accountMeta(kind);
  const liability = kind === 'ACCOUNT_KIND_CREDIT_CARD' || kind === 'ACCOUNT_KIND_LOAN';

  const done = () => {
    void qc.invalidateQueries({ queryKey: qk.netWorth });
    void qc.invalidateQueries({ queryKey: qk.forecast });
    router.back();
  };
  const save = useMutation({
    mutationFn: () => {
      if (!name.trim()) throw new Error('Name is required');
      const b = balance.trim() ? parseMinor(balance.replace('-', ''), currency) : 0;
      if (b === null || b < 0) throw new Error('Enter the balance');
      return FinanceApi.setAccount({ id: acc?.id, name: name.trim(), kind, currency, balanceCents: b });
    },
    onSuccess: done,
    onError: (e) => setError(errorMessage(e)),
  });
  const del = useMutation({
    mutationFn: () => FinanceApi.deleteAccount({ id: acc?.id ?? '' }),
    onSuccess: done,
    onError: (e) => setError(errorMessage(e)),
  });

  return (
    <>
      <Stack.Screen options={{ title: acc ? 'Edit account' : 'New account' }} />
      {error ? <Banner kind="error" message={error} /> : null}
      <Card>
        <Txt variant="label">Type</Txt>
        <Row gap={6} style={styles.wrap}>
          {ACCOUNT_KINDS.map((k) => (
            <Chip key={k.value} label={k.label} icon={k.icon} active={kind === k.value} onPress={() => setKind(k.value)} />
          ))}
        </Row>
        <Txt variant="small">{meta.hint}</Txt>
      </Card>
      <Card>
        <Field label="Name" value={name} onChangeText={setName} placeholder={liability ? 'Kaspi Red' : kind === 'ACCOUNT_KIND_PROPERTY' ? 'Car' : 'Kaspi Gold'} />
        <Field
          label={liability ? 'Amount owed' : kind === 'ACCOUNT_KIND_PROPERTY' ? 'Estimated value' : 'Balance'}
          value={balance}
          onChangeText={setBalance}
          keyboardType="decimal-pad"
          placeholder={amountPlaceholder(currency)}
        />
        <CurrencyField label="Currency" value={currency} onChange={setCurrency} />
        {liability ? <Txt variant="small">Counts against your net worth.</Txt> : null}
      </Card>
      <Button title="Save" icon="checkmark" loading={save.isPending} onPress={() => { setError(null); save.mutate(); }} />
      {acc ? (
        <Button
          title="Delete"
          icon="trash-outline"
          variant="ghost"
          loading={del.isPending}
          onPress={async () => {
            if (await confirm('Delete account?', `${acc.name} will be removed from your net worth.`, 'Delete', true)) del.mutate();
          }}
        />
      ) : null}
    </>
  );
}

export default function AccountScreen() {
  const { id } = useLocalSearchParams<{ id?: string }>();
  const nw = useNetWorth();
  const acc = id ? nw.data?.accounts.find((a) => a.id === id) ?? null : null;
  if (id && !nw.data) {
    return (
      <Screen>
        <Txt variant="muted">Loading…</Txt>
      </Screen>
    );
  }
  return (
    <Screen>
      <Form key={acc?.id ?? 'new'} acc={acc} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  wrap: { flexWrap: 'wrap' },
});
