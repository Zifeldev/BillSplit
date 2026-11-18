import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { useRecurring } from '@/api/queries';
import type { RecurringExpense, RecurringPeriod } from '@/api/types';
import {
  Card,
  EmptyView,
  ErrorView,
  IconTile,
  SkeletonList,
  Money,
  Row,
  Screen,
  SectionHeader,
  Segmented,
  Txt,
} from '@/components/ui';
import { categoryColor, categoryMeta } from '@/lib/categories';
import { formatDate, formatMoney } from '@/lib/format';
import { radius, spacing, useTheme } from '@/theme';

const PERIOD: Record<RecurringPeriod, string> = {
  RECURRING_PERIOD_UNSPECIFIED: '',
  RECURRING_PERIOD_WEEKLY: 'weekly',
  RECURRING_PERIOD_MONTHLY: 'monthly',
  RECURRING_PERIOD_YEARLY: 'yearly',
};

function daysUntil(ts: string | null): number | null {
  if (!ts) return null;
  const t = new Date(ts).getTime();
  if (Number.isNaN(t)) return null;
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return Math.round((t - today.getTime()) / 86_400_000);
}

function nextLabel(ts: string | null): string {
  const d = daysUntil(ts);
  if (d === null) return '';
  if (d < 0) return `expected ${formatDate(ts)} (overdue)`;
  if (d === 0) return 'next charge today';
  if (d === 1) return 'next charge tomorrow';
  if (d <= 7) return `next charge in ${d} days`;
  return `next charge ${formatDate(ts)}`;
}

function RecurringRow({ item, currency }: { item: RecurringExpense; currency: string }) {
  const { c, dark } = useTheme();
  const cur = item.currency || currency;
  const soon = (daysUntil(item.nextExpectedAt) ?? 99) <= 3;
  return (
    <View
      style={[
        styles.row,
        { borderColor: item.isSubscription ? c.primary : c.border },
        item.isSubscription && { backgroundColor: c.primarySoft },
      ]}>
      <Row gap={spacing.md}>
        <IconTile icon={categoryMeta(item.categoryId).icon} color={categoryColor(item.categoryId, dark)} size={36} />
        <View style={styles.flex1}>
          <Row gap={6}>
            <Txt weight="700" numberOfLines={1} style={styles.shrink}>
              {item.merchant || 'Unknown merchant'}
            </Txt>
            {item.isSubscription ? (
              <View style={[styles.tag, { backgroundColor: c.accent }]}>
                <Txt variant="small" weight="700" color={c.primaryText}>
                  Subscription
                </Txt>
              </View>
            ) : null}
          </Row>
          <Txt variant="small" color={soon ? c.warning : undefined}>
            {[PERIOD[item.period], nextLabel(item.nextExpectedAt)].filter(Boolean).join(' · ')}
          </Txt>
        </View>
        <View style={styles.right}>
          <Money cents={item.averageCents} currency={cur} />
          <Txt variant="small">{formatMoney(item.yearlyCostCents, cur)}/yr</Txt>
        </View>
      </Row>
      <Txt variant="small">
        Seen {item.occurrences} {item.occurrences === 1 ? 'time' : 'times'}
        {item.lastChargedAt ? ` · last ${formatDate(item.lastChargedAt)}` : ''}
      </Txt>
    </View>
  );
}

export default function RecurringScreen() {
  const { c } = useTheme();
  const q = useRecurring();
  const [filter, setFilter] = useState<'all' | 'subs'>('all');

  const d = q.data;
  const cur = d?.currency || 'USD';
  const items = [...(d?.items ?? [])].sort((a, b) => {
    if (a.isSubscription !== b.isSubscription) return a.isSubscription ? -1 : 1;
    return b.yearlyCostCents - a.yearlyCostCents;
  });
  const shown = filter === 'subs' ? items.filter((i) => i.isSubscription) : items;
  const subs = items.filter((i) => i.isSubscription);
  const subsYearly = subs.reduce((s, i) => s + i.yearlyCostCents, 0);
  const totalYearly = items.reduce((s, i) => s + i.yearlyCostCents, 0);

  return (
    <Screen refreshing={q.isRefetching} onRefresh={() => void q.refetch()}>
      {d ? (
        <>
          <Card style={{ backgroundColor: c.accent, borderColor: c.accent }}>
            <Txt variant="label" color={c.primaryText} style={styles.dim}>
              Recurring per month
            </Txt>
            <Money cents={d.monthlyTotalCents} currency={cur} variant="big" style={{ color: c.primaryText }} />
            <Txt color={c.primaryText} style={styles.dim}>
              {formatMoney(totalYearly, cur)} per year · {subs.length} {subs.length === 1 ? 'subscription' : 'subscriptions'} (
              {formatMoney(subsYearly, cur)}/yr)
            </Txt>
          </Card>

          {items.length === 0 ? (
            <Card>
              <EmptyView
                icon="repeat"
                title="No recurring charges found"
                message="Import a few months of bank statements — subscriptions and regular bills are detected automatically."
              />
            </Card>
          ) : (
            <>
              <Segmented
                options={[
                  { value: 'all', label: `All (${items.length})` },
                  { value: 'subs', label: `Subscriptions (${subs.length})` },
                ]}
                value={filter}
                onChange={setFilter}
              />
              <SectionHeader title="Sorted by yearly cost" />
              {shown.map((item, i) => (
                <RecurringRow
                  key={`${item.merchant}-${item.categoryId}-${i}`}
                  item={item}
                  currency={cur}
                />
              ))}
              <Txt variant="small">Amounts are averages converted to your home currency ({cur}).</Txt>
            </>
          )}
        </>
      ) : q.error ? (
        <ErrorView error={q.error} onRetry={() => void q.refetch()} />
      ) : (
        <SkeletonList rows={4} />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  shrink: { flexShrink: 1 },
  right: { alignItems: 'flex-end', gap: 2 },
  dim: { opacity: 0.85 },
  row: { borderWidth: 1, borderRadius: radius.lg, padding: spacing.md, gap: spacing.sm },
  tag: { paddingHorizontal: 6, paddingVertical: 1, borderRadius: radius.pill },
});
