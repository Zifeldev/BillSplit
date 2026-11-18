import { useMutation, useQueryClient } from '@tanstack/react-query';
import React, { useMemo, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useBudgets, useCategories, useDashboard } from '@/api/queries';
import { FinanceApi } from '@/api/services';
import type { Category } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { ProgressBar } from '@/components/charts';
import { Banner, Button, Card, EmptyView, ErrorView, Field, HeroCard, IconTile, ListRow, Row, Screen, SkeletonList, Txt } from '@/components/ui';
import { categoryColor, categoryMeta } from '@/lib/categories';
import { amountPlaceholder, formatMoney, minorToInput, parseMinor } from '@/lib/format';
import { radius, spacing, useTheme } from '@/theme';

function BudgetEditor({
  category,
  limitCents,
  currency,
  onDone,
}: {
  category: Category;
  limitCents: number;
  currency: string;
  onDone: () => void;
}) {
  const qc = useQueryClient();
  const [value, setValue] = useState(minorToInput(limitCents, currency));
  const [error, setError] = useState<string | null>(null);
  const onSuccess = () => {
    void qc.invalidateQueries({ queryKey: qk.budgets });
    void qc.invalidateQueries({ queryKey: qk.dashboardAll });
    onDone();
  };
  const set = useMutation({
    mutationFn: (cents: number) => FinanceApi.setBudget({ categoryId: category.id, limitCents: cents }),
    onSuccess,
    onError: (e) => setError(errorMessage(e)),
  });
  const del = useMutation({
    mutationFn: () => FinanceApi.deleteBudget({ categoryId: category.id }),
    onSuccess,
    onError: (e) => setError(errorMessage(e)),
  });
  return (
    <View style={styles.editor}>
      {error ? <Banner kind="error" message={error} /> : null}
      <Row gap={spacing.sm}>
        <Field style={styles.flex1} value={value} onChangeText={setValue} keyboardType="decimal-pad" placeholder={`Monthly limit, ${amountPlaceholder(currency)}`} autoFocus />
        <Button
          title="Save"
          small
          loading={set.isPending}
          onPress={() => {
            const cents = parseMinor(value, currency);
            if (cents === null || cents <= 0) {
              setError('Enter a positive amount');
              return;
            }
            set.mutate(cents);
          }}
        />
        {limitCents > 0 ? (
          <Button title="Remove" small variant="danger" loading={del.isPending} onPress={() => del.mutate()} />
        ) : null}
      </Row>
    </View>
  );
}

export default function BudgetsScreen() {
  const { c, dark } = useTheme();
  const { user } = useAuth();
  const now = new Date();
  const categories = useCategories();
  const budgets = useBudgets();
  const dash = useDashboard(now.getFullYear(), now.getMonth() + 1);
  const [editing, setEditing] = useState<string | null>(null);
  const cur = dash.data?.currency || user?.currency || 'USD';

  const limitByCat = useMemo(
    () => new Map((budgets.data?.budgets ?? []).map((b) => [b.categoryId, b.limitCents])),
    [budgets.data],
  );
  const spentByCat = useMemo(
    () => new Map((dash.data?.byCategory ?? []).map((s) => [s.categoryId, s.spentCents])),
    [dash.data],
  );
  const cats = (categories.data?.categories ?? []).filter((x) => x.kind !== 'CATEGORY_KIND_INCOME');
  const totalLimit = [...limitByCat.values()].reduce((a, b) => a + b, 0);

  const loading = categories.isLoading || budgets.isLoading;
  const error = categories.error || budgets.error;

  return (
    <Screen
      refreshing={budgets.isRefetching || dash.isRefetching}
      onRefresh={() => {
        void budgets.refetch();
        void categories.refetch();
        void dash.refetch();
      }}>
      <HeroCard>
        <Txt variant="label" color="rgba(255,255,255,0.85)">
          Total monthly budget
        </Txt>
        <Txt variant="hero" color="#FFFFFF">
          {formatMoney(totalLimit, cur)}
        </Txt>
        <Txt variant="small" color="rgba(255,255,255,0.88)">
          Spending shown for the current month, in {cur}. Tap a category to set its limit.
        </Txt>
      </HeroCard>
      {error ? (
        <ErrorView error={error} onRetry={() => { void categories.refetch(); void budgets.refetch(); }} />
      ) : loading ? (
        <SkeletonList rows={5} />
      ) : cats.length === 0 ? (
        <EmptyView icon="pricetag" title="No categories" message="Categories come from the server." />
      ) : (
        <Card>
          {cats.map((cat) => {
            const limit = limitByCat.get(cat.id) ?? 0;
            const spent = spentByCat.get(cat.id) ?? 0;
            return (
              <View key={cat.id} style={[styles.row, { borderColor: c.border }]}>
                <ListRow
                  onPress={() => setEditing(editing === cat.id ? null : cat.id)}
                  left={<IconTile icon={categoryMeta(cat.id).icon} color={categoryColor(cat.id, dark)} size={42} />}
                  title={cat.name}
                  subtitle={limit > 0 ? `${formatMoney(spent, cur)} of ${formatMoney(limit, cur)}` : `${formatMoney(spent, cur)} spent · no budget`}
                  right={limit > 0 ? <Txt weight="700">{formatMoney(limit, cur)}</Txt> : <Txt variant="small" color={c.primary}>Set</Txt>}
                />
                {limit > 0 ? <ProgressBar value={spent / limit} color={categoryColor(cat.id, dark)} height={10} /> : null}
                {editing === cat.id ? (
                  <BudgetEditor category={cat} limitCents={limit} currency={cur} onDone={() => setEditing(null)} />
                ) : null}
              </View>
            );
          })}
        </Card>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  row: { paddingBottom: spacing.sm },
  editor: { gap: spacing.sm, paddingTop: spacing.sm, borderRadius: radius.md },
});
