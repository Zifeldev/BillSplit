import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import { Stack, useRouter } from 'expo-router';
import React, { useMemo, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { qk, useCategories } from '@/api/queries';
import { FinanceApi } from '@/api/services';
import type { Transaction } from '@/api/types';
import {
  Button,
  Card,
  EmptyView,
  ErrorView,
  IconButton,
  ListRow,
  IconTile,
  SkeletonList,
  Money,
  Row,
  Screen,
  SelectField,
  Txt,
} from '@/components/ui';
import { categoryColor, categoryMeta, categoryOptions } from '@/lib/categories';
import { formatDate, monthLabel } from '@/lib/format';
import { spacing, useTheme } from '@/theme';

const PAGE = 50;

export default function TransactionsScreen() {
  const router = useRouter();
  const { c, dark } = useTheme();
  const now = new Date();
  const [ym, setYm] = useState<{ year: number; month: number } | null>({ year: now.getFullYear(), month: now.getMonth() + 1 });
  const [categoryId, setCategoryId] = useState('');
  const qc = useQueryClient();
  const categories = useCategories();

  const filter = useMemo(() => {
    if (!ym) return { categoryId };
    const from = new Date(ym.year, ym.month - 1, 1).toISOString();
    const to = new Date(ym.year, ym.month, 1).toISOString();
    return { from, to, categoryId };
  }, [ym, categoryId]);

  const txs = useInfiniteQuery({
    queryKey: qk.transactions(filter),
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      FinanceApi.listTransactions({
        ...filter,
        categoryId: filter.categoryId || undefined,
        limit: PAGE,
        offset: pageParam,
      }),
    getNextPageParam: (last, all) =>
      (last.transactions ?? []).length < PAGE ? undefined : all.reduce((s, p) => s + (p.transactions ?? []).length, 0),
  });

  const catById = useMemo(() => new Map((categories.data?.categories ?? []).map((cat) => [cat.id, cat])), [categories.data]);
  const options = useMemo(() => categoryOptions(categories.data?.categories), [categories.data]);
  const list = (txs.data?.pages ?? []).flatMap((p) => p.transactions ?? []);

  const shift = (delta: number) =>
    setYm((p) => {
      const base = p ?? { year: now.getFullYear(), month: now.getMonth() + 1 };
      const d = new Date(base.year, base.month - 1 + delta, 1);
      return { year: d.getFullYear(), month: d.getMonth() + 1 };
    });

  return (
    <Screen refreshing={txs.isRefetching && !txs.isFetchingNextPage} onRefresh={() => void txs.refetch()}>
      <Stack.Screen
        options={{
          headerRight: () => <IconButton icon="add-circle" color={c.primary} size={28} label="Add transaction" onPress={() => router.push('/finance/new-transaction')} />,
        }}
      />
      <Card>
        <Row>
          <IconButton icon="chevron-back" onPress={() => shift(-1)} label="Previous month" />
          <Txt variant="h3" style={styles.center}>
            {ym ? monthLabel(ym.year, ym.month) : 'All time'}
          </Txt>
          <IconButton icon="chevron-forward" onPress={() => shift(1)} label="Next month" />
        </Row>
        <Row gap={spacing.sm}>
          <View style={styles.flex1}>
            <SelectField
              value={categoryId}
              options={[{ value: '', label: 'All categories' }, ...options]}
              onChange={setCategoryId}
              searchable
            />
          </View>
          <Button
            title={ym ? 'All time' : 'By month'}
            small
            variant="ghost"
            onPress={() => setYm(ym ? null : { year: now.getFullYear(), month: now.getMonth() + 1 })}
          />
        </Row>
      </Card>

      {txs.data ? (
        list.length === 0 ? (
          <Card>
            <EmptyView
              icon="list"
              title="No transactions"
              message="Add one manually or import a bank statement."
              action={<Button title="Add transaction" icon="add" small onPress={() => router.push('/finance/new-transaction')} />}
            />
          </Card>
        ) : (
          <Card>
            {list.map((t) => {
              const cat = catById.get(t.categoryId);
              return (
                <ListRow
                  key={t.id}
                  onPress={() => {
                    // There is no GetTransaction RPC: hand the row to the edit screen via the cache.
                    qc.setQueryData<Transaction>(qk.transaction(t.id), t);
                    router.push(`/finance/edit-transaction?id=${encodeURIComponent(t.id)}`);
                  }}
                  left={
                    <IconTile
                      icon={t.categoryId ? categoryMeta(t.categoryId).icon : t.amountCents >= 0 ? 'cash' : 'receipt'}
                      color={t.categoryId ? categoryColor(t.categoryId, dark) : t.amountCents >= 0 ? c.positive : c.textMuted}
                      size={42}
                    />
                  }
                  title={t.merchant || t.description || 'Transaction'}
                  subtitle={`${cat?.name ?? 'Uncategorized'} · ${formatDate(t.occurredAt)}`}
                  right={<Money cents={t.amountCents} currency={t.currency} signed colored />}
                />
              );
            })}
            {txs.hasNextPage ? (
              <Button
                title="Load more"
                variant="ghost"
                small
                loading={txs.isFetchingNextPage}
                onPress={() => void txs.fetchNextPage()}
              />
            ) : null}
          </Card>
        )
      ) : txs.error ? (
        <ErrorView error={txs.error} onRetry={() => void txs.refetch()} />
      ) : (
        <SkeletonList rows={5} />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  center: { flex: 1, textAlign: 'center' },
});
