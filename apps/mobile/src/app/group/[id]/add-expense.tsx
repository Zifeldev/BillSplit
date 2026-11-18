import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useGroup } from '@/api/queries';
import { ReceiptApi } from '@/api/services';
import type { ExpenseParticipant, GroupMember, SplitType } from '@/api/types';
import { useMyId } from '@/auth/AuthContext';
import { CurrencyField } from '@/components/CurrencyField';
import {
  Avatar,
  Banner,
  Button,
  Card,
  ErrorView,
  Field,
  LoadingView,
  Row,
  Screen,
  SectionHeader,
  Segmented,
  SelectField,
  Stepper,
  Txt,
} from '@/components/ui';
import { amountPlaceholder, dateInputToRfc3339, formatMoney, minorToInput, parseMinor, todayInput } from '@/lib/format';
import { spacing, useTheme } from '@/theme';

type Mode = 'equal' | 'percent' | 'exact' | 'shares';
const SPLIT_TYPE: Record<Mode, SplitType> = {
  equal: 'SPLIT_TYPE_EQUAL',
  percent: 'SPLIT_TYPE_PERCENT',
  exact: 'SPLIT_TYPE_EXACT',
  shares: 'SPLIT_TYPE_SHARES',
};

/** "33.33" → 3333 basis points; null if invalid (max 2 decimals). */
function parsePercentBp(input: string): number | null {
  const s = input.trim().replace(',', '.');
  if (!s) return 0;
  if (!/^\d*(\.\d{0,2})?$/.test(s) || s === '.') return null;
  return Math.round(Number(s) * 100);
}

const bpToInput = (bp: number) => (bp % 100 === 0 ? String(bp / 100) : (bp / 100).toFixed(2));

/** Spreads `total` across `n` integer parts, remainder to the first ones. */
function spread(total: number, n: number): number[] {
  if (n <= 0) return [];
  const base = Math.floor(total / n);
  const rem = total - base * n;
  return Array.from({ length: n }, (_, i) => base + (i < rem ? 1 : 0));
}

function ExpenseForm({ groupId, members, groupCurrency, groupName }: {
  groupId: string;
  members: GroupMember[];
  groupCurrency: string;
  groupName: string;
}) {
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const myId = useMyId();

  const [description, setDescription] = useState('');
  const [amount, setAmount] = useState('');
  const [currency, setCurrency] = useState('');
  const [paidBy, setPaidBy] = useState(myId);
  const [date, setDate] = useState(todayInput());
  const [mode, setMode] = useState<Mode>('equal');
  const [included, setIncluded] = useState<Set<string>>(() => new Set(members.map((m) => m.userId)));
  const [percent, setPercent] = useState<Record<string, string>>({});
  const [exact, setExact] = useState<Record<string, string>>({});
  const [shares, setShares] = useState<Record<string, number>>(() =>
    Object.fromEntries(members.map((m) => [m.userId, 1])),
  );
  const [error, setError] = useState<string | null>(null);

  const cur = currency || groupCurrency;
  const amountCents = parseMinor(amount, cur);
  const name = (m: GroupMember) => (m.userId === myId ? 'You' : m.name || m.email);

  // Live validation of the per-member values.
  const check = ((): { ok: boolean; message: string } => {
    if (mode === 'equal') {
      const n = included.size;
      return { ok: n > 0, message: n > 0 ? '' : 'Select at least one person' };
    }
    if (mode === 'percent') {
      let sum = 0;
      for (const m of members) {
        const bp = parsePercentBp(percent[m.userId] ?? '');
        if (bp === null) return { ok: false, message: `Invalid percent for ${name(m)}` };
        sum += bp;
      }
      const left = 10000 - sum;
      return left === 0
        ? { ok: true, message: 'Percentages add up to 100%' }
        : { ok: false, message: left > 0 ? `${bpToInput(left)}% left to assign` : `${bpToInput(-left)}% over 100%` };
    }
    if (mode === 'exact') {
      let sum = 0;
      for (const m of members) {
        const raw = exact[m.userId] ?? '';
        const v = raw.trim() ? parseMinor(raw, cur) : 0;
        if (v === null || v < 0) return { ok: false, message: `Invalid amount for ${name(m)}` };
        sum += v;
      }
      if (amountCents === null || amountCents <= 0) return { ok: false, message: 'Enter the total amount first' };
      const left = amountCents - sum;
      return left === 0
        ? { ok: true, message: `Amounts add up to ${formatMoney(amountCents, cur)}` }
        : {
            ok: false,
            message: left > 0 ? `${formatMoney(left, cur)} left to assign` : `${formatMoney(-left, cur)} over the total`,
          };
    }
    const total = members.reduce((s, m) => s + (shares[m.userId] ?? 0), 0);
    return { ok: total > 0, message: total > 0 ? `${total} shares in total` : 'Give at least one share' };
  })();

  const fillEvenly = () => {
    if (mode === 'percent') {
      const parts = spread(10000, members.length);
      setPercent(Object.fromEntries(members.map((m, i) => [m.userId, bpToInput(parts[i])])));
    } else if (mode === 'exact' && amountCents && amountCents > 0) {
      const parts = spread(amountCents, members.length);
      setExact(Object.fromEntries(members.map((m, i) => [m.userId, minorToInput(parts[i], cur)])));
    }
  };

  const participants = (): ExpenseParticipant[] => {
    switch (mode) {
      case 'equal':
        // Empty with EQUAL means "all members".
        return included.size === members.length ? [] : [...included].map((userId) => ({ userId, value: 0 }));
      case 'percent':
        return members
          .map((m) => ({ userId: m.userId, value: parsePercentBp(percent[m.userId] ?? '') ?? 0 }))
          .filter((p) => p.value > 0);
      case 'exact':
        return members
          .map((m) => ({ userId: m.userId, value: parseMinor(exact[m.userId] ?? '', cur) ?? 0 }))
          .filter((p) => p.value > 0);
      case 'shares':
        return members.map((m) => ({ userId: m.userId, value: shares[m.userId] ?? 0 })).filter((p) => p.value > 0);
    }
  };

  const create = useMutation({
    mutationFn: () => {
      if (!description.trim()) throw new Error('Enter a description');
      if (amountCents === null || amountCents <= 0) throw new Error('Enter a positive amount');
      if (!check.ok) throw new Error(check.message);
      const occurredAt = date.trim() ? dateInputToRfc3339(date) : null;
      if (date.trim() && !occurredAt) throw new Error('Date must be in YYYY-MM-DD format');
      return ReceiptApi.createExpense({
        groupId,
        description: description.trim(),
        amountCents,
        currency,
        paidBy: paidBy === myId ? '' : paidBy,
        splitType: SPLIT_TYPE[mode],
        participants: participants(),
        occurredAt: occurredAt ?? undefined,
      });
    },
    onSuccess: (split) => {
      void qc.invalidateQueries({ queryKey: qk.receipts(groupId) });
      void qc.invalidateQueries({ queryKey: qk.groupBalances(groupId) });
      void qc.invalidateQueries({ queryKey: qk.myBalances });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
      void qc.invalidateQueries({ queryKey: qk.transactionsAll });
      if (split.receiptId) {
        qc.setQueryData(qk.split(split.receiptId), split);
        router.replace(`/receipt/${split.receiptId}`);
      } else {
        router.back();
      }
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const payerOptions = members.map((m) => ({ value: m.userId, label: name(m), hint: m.email }));

  return (
    <Screen>
      <Stack.Screen options={{ title: `Add expense · ${groupName}` }} />
      {error ? <Banner kind="error" message={error} /> : null}
      <Card>
        <Field label="Description" value={description} onChangeText={setDescription} placeholder="Rent, taxi, utilities…" />
        <Row gap={spacing.sm} style={styles.top}>
          <Field
            style={styles.flex1}
            label="Amount"
            value={amount}
            onChangeText={setAmount}
            keyboardType="decimal-pad"
            placeholder={amountPlaceholder(cur)}
            error={amount.trim() && amountCents === null ? 'Invalid amount' : null}
          />
          <View style={styles.flex1}>
            <CurrencyField
              value={currency}
              onChange={setCurrency}
              allowEmpty
              emptyLabel={`Group (${groupCurrency})`}
            />
          </View>
        </Row>
        <SelectField label="Paid by" value={paidBy} options={payerOptions} onChange={setPaidBy} />
        <Field label="Date (YYYY-MM-DD)" value={date} onChangeText={setDate} placeholder="2026-01-31" autoCapitalize="none" />
      </Card>

      <SectionHeader
        title="Split"
        action={mode === 'percent' || mode === 'exact' ? 'Split evenly' : undefined}
        onAction={fillEvenly}
      />
      <Segmented
        options={[
          { value: 'equal', label: 'Equal' },
          { value: 'percent', label: '%' },
          { value: 'exact', label: 'Exact' },
          { value: 'shares', label: 'Shares' },
        ]}
        value={mode}
        onChange={setMode}
      />
      <Card>
        {members.map((m) => {
          const on = included.has(m.userId);
          let right: React.ReactNode = null;
          if (mode === 'equal') {
            right = (
              <Pressable
                hitSlop={8}
                onPress={() =>
                  setIncluded((prev) => {
                    const next = new Set(prev);
                    if (next.has(m.userId)) next.delete(m.userId);
                    else next.add(m.userId);
                    return next;
                  })
                }>
                <Icon name={on ? 'checkmark-circle' : 'ellipse-outline'} size={26} color={on ? c.primary : c.textMuted} />
              </Pressable>
            );
          } else if (mode === 'percent') {
            right = (
              <Row gap={4}>
                <Field
                  style={styles.valueInput}
                  value={percent[m.userId] ?? ''}
                  onChangeText={(v) => setPercent((p) => ({ ...p, [m.userId]: v }))}
                  keyboardType="decimal-pad"
                  placeholder="0"
                />
                <Txt variant="muted">%</Txt>
              </Row>
            );
          } else if (mode === 'exact') {
            right = (
              <Field
                style={styles.valueInputWide}
                value={exact[m.userId] ?? ''}
                onChangeText={(v) => setExact((p) => ({ ...p, [m.userId]: v }))}
                keyboardType="decimal-pad"
                placeholder={amountPlaceholder(cur)}
              />
            );
          } else {
            right = (
              <Stepper
                value={shares[m.userId] ?? 0}
                onChange={(v) => setShares((p) => ({ ...p, [m.userId]: v }))}
              />
            );
          }
          let hint = '';
          if (amountCents && amountCents > 0) {
            if (mode === 'equal' && on && included.size > 0) {
              hint = `≈ ${formatMoney(Math.floor(amountCents / included.size), cur)}`;
            } else if (mode === 'percent') {
              const bp = parsePercentBp(percent[m.userId] ?? '') ?? 0;
              if (bp > 0) hint = `≈ ${formatMoney(Math.round((amountCents * bp) / 10000), cur)}`;
            } else if (mode === 'shares') {
              const total = members.reduce((s, x) => s + (shares[x.userId] ?? 0), 0);
              const mine = shares[m.userId] ?? 0;
              if (total > 0 && mine > 0) hint = `≈ ${formatMoney(Math.round((amountCents * mine) / total), cur)}`;
            }
          }
          return (
            <Row key={m.userId} gap={spacing.md} style={styles.memberRow}>
              <Avatar name={m.name} id={m.userId} size={32} />
              <View style={styles.flex1}>
                <Txt weight="600" numberOfLines={1}>
                  {name(m)}
                </Txt>
                {hint ? <Txt variant="small">{hint}</Txt> : null}
              </View>
              {right}
            </Row>
          );
        })}
        {check.message ? (
          <Row gap={6}>
            <Icon
              name={check.ok ? 'checkmark-circle' : 'alert-circle'}
              size={16}
              color={check.ok ? c.positive : c.warning}
            />
            <Txt variant="small" color={check.ok ? c.positive : c.warning}>
              {check.message}
            </Txt>
          </Row>
        ) : null}
      </Card>
      <Txt variant="small">The expense is added to group balances right away.</Txt>

      <Button
        title="Add expense"
        icon="checkmark"
        loading={create.isPending}
        disabled={!check.ok || !description.trim() || !amountCents || amountCents <= 0}
        onPress={() => {
          setError(null);
          create.mutate();
        }}
      />
    </Screen>
  );
}

export default function AddExpenseScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const groupId = String(id ?? '');
  const group = useGroup(groupId);
  if (!group.data) {
    return (
      <Screen>
        {group.error ? <ErrorView error={group.error} onRetry={() => void group.refetch()} /> : <LoadingView />}
      </Screen>
    );
  }
  return (
    <ExpenseForm
      groupId={groupId}
      members={group.data.members ?? []}
      groupCurrency={group.data.currency}
      groupName={group.data.name}
    />
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  top: { alignItems: 'flex-start' },
  memberRow: { paddingVertical: 6 },
  valueInput: { width: 76 },
  valueInputWide: { width: 110 },
});
