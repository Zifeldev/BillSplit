import { Icon } from '@/components/Icon';
import { useRouter, type Href } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { useDashboard, useForecast, useNetWorth } from '@/api/queries';
import type { Dashboard, MonthForecast } from '@/api/types';
import { Masonry, type MasonryBlock } from '@/components/Masonry';
import { Donut, LegendRow, LineChart, ProgressBar, Ring, Sparkline } from '@/components/charts';
import {
  Button,
  Card,
  EmptyView,
  ErrorView,
  FadeIn,
  GlassPill,
  HeroCard,
  IconButton,
  IconName,
  IconTile,
  Money,
  Row,
  Screen,
  SectionHeader,
  Skeleton,
  SkeletonList,
  Txt,
} from '@/components/ui';
import { categoryColor, categoryMeta } from '@/lib/categories';
import { compactMoney, formatMoney, formatPercent, formatSigned, monthLabel, shortDay, shortMonth } from '@/lib/format';
import { tapFeedback } from '@/lib/haptics';
import { radius, spacing, useTheme } from '@/theme';

const WHITE = '#FFFFFF';
const WHITE_SOFT = 'rgba(255,255,255,0.86)';

function Shortcut({ icon, label, onPress }: { icon: IconName; label: string; onPress: () => void }) {
  const { c } = useTheme();
  return (
    <Pressable
      accessibilityRole="button"
      onPress={() => {
        tapFeedback();
        onPress();
      }}
      style={({ pressed }) => [styles.shortcut, pressed && { opacity: 0.75, transform: [{ scale: 0.96 }] }]}>
      <IconTile icon={icon} color={c.primary} size={48} />
      <Txt variant="small" color={c.text} weight="600" numberOfLines={1}>
        {label}
      </Txt>
    </Pressable>
  );
}

const SHORTCUTS: { icon: IconName; label: string; href: Href }[] = [
  { icon: 'list', label: 'Activity', href: '/finance/transactions' },
  { icon: 'cloud-upload', label: 'Import', href: '/finance/import' },
  { icon: 'wallet', label: 'Budgets', href: '/finance/budgets' },
  { icon: 'flag', label: 'Goals', href: '/finance/goals' },
  { icon: 'trending-up', label: 'Invest', href: '/finance/investments' },
  { icon: 'repeat', label: 'Subs', href: '/finance/recurring' },
];

const TOP_SLICES = 5;

/** The donut hole fits ~10 characters: switch to "KZT 538k" for longer amounts. */
function donutValue(minor: number, cur: string): string {
  const full = formatMoney(minor, cur, true);
  return full.length > 10 ? compactMoney(minor, cur) : full;
}

/** The API returns only days with spending; a line needs the zero days too. */
function fillDays(days: Dashboard['daily']): Dashboard['daily'] {
  if (days.length === 0) return days;
  const by = new Map(days.map((x) => [x.date, x.spentCents]));
  const [y, m] = days[0].date.split('-').map(Number);
  const last = Math.max(...days.map((x) => Number(x.date.slice(8, 10))));
  const out: Dashboard['daily'] = [];
  for (let day = 1; day <= last; day++) {
    const date = `${y}-${String(m).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
    out.push({ date, spentCents: by.get(date) ?? 0 });
  }
  return out;
}

function SpendingDonut({ d }: { d: Dashboard }) {
  const { c, dark } = useTheme();
  const [active, setActive] = useState<string | null>(null);
  const cur = d.currency || 'USD';
  const cats = [...(d.byCategory ?? [])].filter((x) => x.spentCents > 0).sort((a, b) => b.spentCents - a.spentCents);
  if (cats.length === 0) return null;
  const top = cats.slice(0, TOP_SLICES);
  const rest = cats.slice(TOP_SLICES);
  const slices = top.map((x) => ({
    key: x.categoryId || x.name,
    label: x.name || 'Uncategorized',
    value: x.spentCents,
    color: categoryColor(x.categoryId, dark),
    id: x.categoryId,
  }));
  if (rest.length > 0) {
    slices.push({
      key: '__other',
      label: `Other (${rest.length})`,
      value: rest.reduce((s, x) => s + x.spentCents, 0),
      color: c.chartMuted,
      id: '',
    });
  }
  const total = slices.reduce((s, x) => s + x.value, 0);
  const sel = slices.find((s) => s.key === active);
  return (
    <Card>
      <View style={styles.donutWrap}>
        <Donut
          slices={slices}
          active={active}
          centerTop={sel ? sel.label : 'Spent'}
          centerValue={donutValue(sel ? sel.value : total, cur)}
        />
      </View>
      <View>
        {slices.map((s) => {
          const meta = categoryMeta(s.id);
          return (
            <LegendRow
              key={s.key}
              color={s.color}
              label={s.label}
              value={formatMoney(s.value, cur)}
              share={formatPercent(total > 0 ? s.value / total : 0)}
              active={active === s.key}
              onPress={() => {
                tapFeedback();
                setActive(active === s.key ? null : s.key);
              }}
              icon={
                s.key === '__other' ? (
                  <IconTile icon="ellipsis-horizontal" color={c.textMuted} size={32} />
                ) : (
                  <IconTile icon={meta.icon} color={s.color} size={32} solid />
                )
              }
            />
          );
        })}
      </View>
    </Card>
  );
}

/** Rough block heights for column balancing on desktop (see `Masonry`). */
const WEIGHT = {
  summary: 470,
  forecast: 430,
  insightBase: 40,
  insightRow: 44,
  donut: 500,
  chart: 320,
  listBase: 80,
  budgetRow: 74,
  rule: 330,
  merchantRow: 64,
  splitting: 150,
  empty: 360,
  footer: 70,
} as const;

/** The month's sections as layout blocks; sections without data are left out. */
function useDashboardBlocks(d: Dashboard | undefined): MasonryBlock[] {
  const { c, dark } = useTheme();
  const router = useRouter();
  if (!d) return [];
  const cur = d.currency || 'USD';
  const rule = d.ruleBreakdown ?? { needs: 0, wants: 0, savings: 0 };
  const budgeted = [...(d.byCategory ?? [])]
    .filter((x) => x.budgetCents > 0)
    .sort((a, b) => b.budgetUsed - a.budgetUsed);
  const daily = fillDays(d.daily ?? []);
  const dailyAvg = daily.length ? daily.reduce((s, x) => s + x.spentCents, 0) / daily.length : 0;
  const months = d.lastSixMonths ?? [];
  const fmt = (v: number) => formatMoney(v, cur);
  const empty = d.incomeCents === 0 && d.expenseCents === 0 && (d.byCategory ?? []).length === 0;

  if (empty) {
    const node = (
      <Card>
        <EmptyView
          icon="stats-chart"
          title="Nothing tracked this month"
          message="Import a bank statement (CSV, OFX or PDF) or add a transaction to light up your dashboard."
          action={
            <Row gap={spacing.sm}>
              <Button title="Import" icon="cloud-upload-outline" small onPress={() => router.push('/finance/import')} />
              <Button
                title="Add"
                icon="add"
                small
                variant="secondary"
                onPress={() => router.push('/finance/new-transaction')}
              />
            </Row>
          }
        />
      </Card>
    );
    return [{ key: 'empty', weight: WEIGHT.empty, node }];
  }

  const insights = d.insights ?? [];
  const merchants = d.topMerchants ?? [];
  const blocks: (MasonryBlock | null)[] = [
    insights.length > 0
      ? {
          key: 'insights',
          weight: WEIGHT.insightBase + insights.length * WEIGHT.insightRow,
          node: (
            <FadeIn>
              <Card style={{ backgroundColor: c.warningSoft, borderColor: c.warningSoft }}>
                {insights.map((s, i) => (
                  <Row key={i} gap={10} style={styles.insight}>
                    <Icon name="bulb" size={20} color={c.warning} weight="duotone" />
                    <Txt style={styles.flex1} color={c.text}>
                      {s}
                    </Txt>
                  </Row>
                ))}
              </Card>
            </FadeIn>
          ),
        }
      : null,

    (d.byCategory ?? []).some((x) => x.spentCents > 0)
      ? {
          key: 'donut',
          weight: WEIGHT.donut,
          node: (
            <>
              <SectionHeader title="Where your money went" />
              <SpendingDonut d={d} />
            </>
          ),
        }
      : null,

    daily.length > 0
      ? {
          key: 'daily',
          weight: WEIGHT.chart,
          node: (
            <>
              <SectionHeader title="Daily spending" />
              <Card>
                <LineChart
                  series={[{ name: 'Spent', color: c.primary, values: daily.map((x) => x.spentCents), area: true }]}
                  labels={daily.map((x) => x.date)}
                  formatLabel={shortDay}
                  formatValue={fmt}
                  formatAxis={(v) => compactMoney(v, cur)}
                  refLine={dailyAvg}
                  curve="linear"
                  summary={`avg ${fmt(Math.round(dailyAvg))}/day (dashed line)`}
                  ticks={5}
                />
              </Card>
            </>
          ),
        }
      : null,

    budgeted.length > 0
      ? {
          key: 'budgets',
          weight: WEIGHT.listBase + budgeted.length * WEIGHT.budgetRow,
          node: (
            <>
              <SectionHeader title="Budgets" action="Manage" onAction={() => router.push('/finance/budgets')} />
              <Card>
                {budgeted.map((cat) => {
                  const meta = categoryMeta(cat.categoryId);
                  const col = categoryColor(cat.categoryId, dark);
                  const over = cat.budgetUsed > 1;
                  return (
                    <View key={cat.categoryId || cat.name} style={styles.catRow}>
                      <Row gap={10}>
                        <IconTile icon={meta.icon} color={col} size={32} />
                        <Txt weight="600" style={styles.flex1} numberOfLines={1}>
                          {cat.name || 'Uncategorized'}
                        </Txt>
                        <Txt variant="small" color={over ? c.danger : c.textMuted} weight="700">
                          {fmt(cat.spentCents)} / {fmt(cat.budgetCents)}
                        </Txt>
                      </Row>
                      <ProgressBar value={cat.budgetUsed} color={col} height={10} />
                      {over ? (
                        <Txt variant="small" color={c.danger}>
                          Over by {fmt(cat.spentCents - cat.budgetCents)}
                        </Txt>
                      ) : null}
                    </View>
                  );
                })}
              </Card>
            </>
          ),
        }
      : null,

    {
      key: 'rule',
      weight: WEIGHT.rule,
      node: (
        <>
          <SectionHeader title="50 / 30 / 20 rule" />
          <Card>
            {(
              [
                {
                  key: 'needs',
                  label: 'Needs',
                  value: rule.needs,
                  target: 0.5,
                  color: c.needs,
                  icon: 'home' as IconName,
                },
                {
                  key: 'wants',
                  label: 'Wants',
                  value: rule.wants,
                  target: 0.3,
                  color: c.wants,
                  icon: 'sparkles' as IconName,
                },
                {
                  key: 'savings',
                  label: 'Savings',
                  value: rule.savings,
                  target: 0.2,
                  color: c.savings,
                  icon: 'leaf' as IconName,
                },
              ] as const
            ).map((r) => (
              <View key={r.key} style={styles.ruleRow}>
                <Row gap={10}>
                  <IconTile icon={r.icon} color={r.color} size={32} />
                  <Txt weight="600" style={styles.flex1}>
                    {r.label}
                  </Txt>
                  <Txt variant="small">
                    <Txt variant="small" weight="800" color={c.text}>
                      {formatPercent(r.value)}
                    </Txt>{' '}
                    · target {formatPercent(r.target)}
                  </Txt>
                </Row>
                <ProgressBar value={r.value} color={r.color} marker={r.target} height={10} />
              </View>
            ))}
            <Txt variant="small">Share of income; the tick marks the target.</Txt>
          </Card>
        </>
      ),
    },

    months.length > 0
      ? {
          key: 'months',
          weight: WEIGHT.chart,
          node: (
            <>
              <SectionHeader title="Last 6 months" />
              <Card>
                <LineChart
                  series={[
                    { name: 'Income', color: c.savings, values: months.map((m) => m.incomeCents), area: true },
                    { name: 'Expenses', color: c.wants, values: months.map((m) => m.expenseCents) },
                  ]}
                  labels={months.map((m) => m.month)}
                  formatLabel={shortMonth}
                  formatValue={fmt}
                  formatAxis={(v) => compactMoney(v, cur)}
                  ticks={months.length}
                />
              </Card>
            </>
          ),
        }
      : null,

    merchants.length > 0
      ? {
          key: 'merchants',
          weight: WEIGHT.listBase + merchants.length * WEIGHT.merchantRow,
          node: (
            <>
              <SectionHeader title="Top merchants" />
              <Card>
                {merchants.map((m, i) => (
                  <Row key={`${m.merchant}-${i}`} style={styles.merchant} gap={spacing.md}>
                    <View style={[styles.rank, { backgroundColor: i === 0 ? c.primarySoft : c.cardAlt }]}>
                      <Txt variant="small" weight="800" color={i === 0 ? c.primary : c.textMuted}>
                        {i + 1}
                      </Txt>
                    </View>
                    <View style={styles.flex1}>
                      <Txt weight="600" numberOfLines={1}>
                        {m.merchant || 'Unknown'}
                      </Txt>
                      <Txt variant="small">
                        {m.count} {m.count === 1 ? 'payment' : 'payments'}
                      </Txt>
                    </View>
                    <Txt weight="700">{fmt(m.spentCents)}</Txt>
                  </Row>
                ))}
              </Card>
            </>
          ),
        }
      : null,

    {
      key: 'splitting',
      weight: WEIGHT.splitting,
      node: (
        <>
          <SectionHeader title="Bill splitting" />
          <Row gap={spacing.sm}>
            <Card style={styles.flex1}>
              <Row gap={6}>
                <Icon name="arrow-down-circle" size={18} color={c.positive} />
                <Txt variant="small" weight="600">
                  Owed to you
                </Txt>
              </Row>
              <Money cents={d.owedToMeCents} currency={cur} variant="h2" color={c.positive} />
            </Card>
            <Card style={styles.flex1}>
              <Row gap={6}>
                <Icon name="arrow-up-circle" size={18} color={d.iOweCents > 0 ? c.negative : c.textMuted} />
                <Txt variant="small" weight="600">
                  You owe
                </Txt>
              </Row>
              <Money cents={d.iOweCents} currency={cur} variant="h2" color={d.iOweCents > 0 ? c.negative : undefined} />
            </Card>
          </Row>
        </>
      ),
    },
  ];
  return blocks.filter((b): b is MasonryBlock => b !== null);
}

function NetWorthCard() {
  const router = useRouter();
  const { c } = useTheme();
  const q = useNetWorth();
  const nw = q.data;
  if (!nw) return null;
  const empty = nw.accounts.length === 0 && nw.investmentsCents === 0;
  const hist = nw.history.map((h) => h.netCents);
  return (
    <Card onPress={() => router.push('/finance/net-worth')}>
      <Row gap={spacing.md}>
        <IconTile icon="stats-chart" color={c.primary} size={44} />
        <View style={styles.flex1}>
          <Txt variant="label">Net worth</Txt>
          {empty ? (
            <Txt variant="small">Add cards, cash and loans to see everything you own in one number.</Txt>
          ) : (
            <>
              <Money cents={nw.netCents} currency={nw.currency} variant="h2" />
              <Txt
                variant="small"
                color={nw.hasChange30d ? (nw.change30dCents >= 0 ? c.positive : c.negative) : undefined}>
                {nw.hasChange30d ? `${formatSigned(nw.change30dCents, nw.currency, true)} in 30 days` : 'Tracked daily'}
              </Txt>
            </>
          )}
        </View>
        {hist.length > 1 ? <Sparkline values={hist} /> : <Icon name="chevron-forward" size={20} color={c.textMuted} />}
      </Row>
    </Card>
  );
}

/** Where the current month ends: actual cumulative net so far, then the expected path. */
function ForecastCard({ f }: { f: MonthForecast }) {
  const { c } = useTheme();
  const cur = f.currency;
  const days = f.days ?? [];
  if (days.length < 2) return null;
  const todayIdx = days.reduce((last, d, i) => (d.actual ? i : last), -1);
  const actual = days.map((d, i) => (i <= todayIdx ? d.netCents : NaN));
  const expected = days.map((d, i) => (i >= todayIdx ? d.netCents : NaN));
  const lastDay = days[days.length - 1].date;
  const positive = f.projectedNetCents >= 0;
  const basis =
    f.historyMonths > 0
      ? `Based on your last ${f.historyMonths} ${f.historyMonths === 1 ? 'month' : 'months'}`
      : 'Based on this month’s pace';
  return (
    <Card>
      <Row gap={spacing.md}>
        <View style={styles.flex1}>
          <Txt variant="label">{f.hasBalance ? `Balance on ${shortDay(lastDay)}` : `Net by ${shortDay(lastDay)}`}</Txt>
          <Money
            cents={f.hasBalance ? f.projectedBalanceCents : f.projectedNetCents}
            currency={cur}
            signed={!f.hasBalance}
            variant="h2"
            color={f.hasBalance ? undefined : positive ? c.positive : c.negative}
          />
          <Txt variant="small">
            {f.daysLeft > 0 ? `${f.daysLeft} ${f.daysLeft === 1 ? 'day' : 'days'} left · ` : ''}
            {basis}
          </Txt>
        </View>
      </Row>
      <LineChart
        series={[
          { name: 'So far', color: c.primary, values: actual, area: true },
          { name: 'Expected', color: c.primary, values: expected, dashed: true },
        ]}
        labels={days.map((d) => d.date)}
        formatLabel={shortDay}
        formatValue={(v) => formatSigned(Math.round(v), cur)}
        formatAxis={(v) => compactMoney(v, cur)}
        refLine={0}
        curve="linear"
        zeroBased={false}
        animateKey={f.month}
      />
      <Row gap={spacing.sm}>
        <View style={styles.flex1}>
          <Txt variant="small">Still expected in</Txt>
          <Txt weight="700" color={c.positive}>
            +{formatMoney(f.expectedIncomeCents, cur, true)}
          </Txt>
        </View>
        <View style={styles.flex1}>
          <Txt variant="small">Still expected out</Txt>
          <Txt weight="700" color={c.negative}>
            −{formatMoney(f.expectedExpenseCents, cur, true)}
          </Txt>
        </View>
        {f.hasBalance ? (
          <View style={styles.flex1}>
            <Txt variant="small">Balance now</Txt>
            <Txt weight="700">{formatMoney(f.balanceNowCents, cur, true)}</Txt>
          </View>
        ) : null}
      </Row>
      {(f.upcoming ?? []).length > 0 ? (
        <View style={styles.upcoming}>
          <Txt variant="small" weight="700" color={c.text}>
            Coming up
          </Txt>
          {f.upcoming.slice(0, 4).map((u) => (
            <Row key={u.merchant + u.date} gap={spacing.sm}>
              <Txt variant="small" style={styles.flex1} numberOfLines={1}>
                {shortDay(u.date)} · {u.merchant}
              </Txt>
              <Txt variant="small" weight="700" color={c.text}>
                {formatMoney(u.amountCents, cur)}
              </Txt>
            </Row>
          ))}
        </View>
      ) : null}
    </Card>
  );
}

export default function FinanceScreen() {
  const router = useRouter();
  const now = new Date();
  const [ym, setYm] = useState({ year: now.getFullYear(), month: now.getMonth() + 1 });
  const dash = useDashboard(ym.year, ym.month);
  const d = dash.data;
  const isCurrent = ym.year === now.getFullYear() && ym.month === now.getMonth() + 1;
  const forecast = useForecast();
  const cur = d?.currency || 'USD';

  const shift = (delta: number) =>
    setYm((p) => {
      const dt = new Date(p.year, p.month - 1 + delta, 1);
      return { year: dt.getFullYear(), month: dt.getMonth() + 1 };
    });

  const dashboardBlocks = useDashboardBlocks(d);
  const body: MasonryBlock[] = d
    ? dashboardBlocks
    : [
        {
          key: 'state',
          weight: WEIGHT.empty,
          node: dash.error ? (
            <ErrorView error={dash.error} onRetry={() => void dash.refetch()} />
          ) : (
            <SkeletonList rows={4} />
          ),
        },
      ];
  const showForecast = isCurrent && !!forecast.data && !!d && (d.incomeCents > 0 || d.expenseCents > 0);
  const summary = (
    <>
      <HeroCard>
        <Row>
          <IconButton icon="chevron-back" color={WHITE} onPress={() => shift(-1)} label="Previous month" />
          <Txt variant="h3" color={WHITE} style={styles.monthLabel}>
            {monthLabel(ym.year, ym.month)}
          </Txt>
          <IconButton icon="chevron-forward" color={WHITE} onPress={() => shift(1)} label="Next month" />
        </Row>
        {d ? (
          <>
            <Row gap={spacing.md} style={styles.heroMain}>
              <View style={styles.flex1}>
                <Txt variant="label" color={WHITE_SOFT}>
                  Net this month
                </Txt>
                <Money
                  key={`${ym.year}-${ym.month}`}
                  cents={d.netCents}
                  currency={cur}
                  signed
                  variant="hero"
                  color={WHITE}
                  animate
                  fitChars={10}
                />
                {d.projectedMonthExpenseCents > 0 ? (
                  <Txt variant="small" color={WHITE_SOFT}>
                    Projected spend {formatMoney(d.projectedMonthExpenseCents, cur)}
                  </Txt>
                ) : null}
              </View>
              <Ring
                key={`r-${ym.year}-${ym.month}`}
                value={d.savingsRate}
                size={76}
                stroke={8}
                color={WHITE}
                track="rgba(255,255,255,0.22)"
                label={formatPercent(d.savingsRate)}
                sublabel="saved"
                labelColor={WHITE}
              />
            </Row>
            <Row gap={spacing.sm}>
              <GlassPill>
                <Txt variant="small" color={WHITE_SOFT} weight="600">
                  ↓ Income
                </Txt>
                <Money cents={d.incomeCents} currency={cur} variant="h3" color={WHITE} />
              </GlassPill>
              <GlassPill>
                <Txt variant="small" color={WHITE_SOFT} weight="600">
                  ↑ Spent
                </Txt>
                <Money cents={d.expenseCents} currency={cur} variant="h3" color={WHITE} />
              </GlassPill>
            </Row>
          </>
        ) : dash.error ? null : (
          <View style={styles.heroSkel}>
            <Skeleton width={190} height={40} style={styles.glassSkel} />
            <Row gap={spacing.sm}>
              <Skeleton width="48%" height={58} style={styles.glassSkel} />
              <Skeleton width="48%" height={58} style={styles.glassSkel} />
            </Row>
          </View>
        )}
      </HeroCard>
      <Row style={styles.shortcuts}>
        {SHORTCUTS.map((s) => (
          <Shortcut key={s.label} icon={s.icon} label={s.label} onPress={() => router.push(s.href)} />
        ))}
      </Row>
      <NetWorthCard />
    </>
  );
  const exportButton = (
    <Button
      title={`Export ${monthLabel(ym.year, ym.month)}`}
      icon="download-outline"
      variant="ghost"
      onPress={() =>
        router.push({ pathname: '/finance/export', params: { year: String(ym.year), month: String(ym.month) } })
      }
    />
  );
  const blocks: MasonryBlock[] = [
    { key: 'summary', weight: WEIGHT.summary, node: summary },
    ...(showForecast && forecast.data
      ? [
          {
            key: 'forecast',
            weight: WEIGHT.forecast,
            node: (
              <>
                <SectionHeader title="Month-end forecast" />
                <ForecastCard f={forecast.data} />
              </>
            ),
          },
        ]
      : []),
    ...body,
    { key: 'export', weight: WEIGHT.footer, node: exportButton },
  ];

  return (
    <Screen
      dashboard
      refreshing={dash.isRefetching}
      onRefresh={() => {
        void dash.refetch();
        void forecast.refetch();
      }}>
      <Masonry blocks={blocks} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  shortcuts: { justifyContent: 'space-between', alignItems: 'flex-start' },
  shortcut: { flex: 1, alignItems: 'center', gap: 6, paddingVertical: spacing.xs, minHeight: 44 },
  monthLabel: { flex: 1, textAlign: 'center' },
  heroMain: { marginTop: spacing.xs },
  heroSkel: { gap: spacing.md },
  glassSkel: { backgroundColor: 'rgba(255,255,255,0.18)' },
  donutWrap: { alignItems: 'center', paddingVertical: spacing.sm },
  insight: { alignItems: 'flex-start', paddingVertical: 2 },
  ruleRow: { gap: 8, paddingVertical: 6 },
  catRow: { gap: 8, paddingVertical: 6 },
  merchant: { paddingVertical: 8 },
  rank: { width: 30, height: 30, borderRadius: radius.sm, alignItems: 'center', justifyContent: 'center' },
  upcoming: { gap: 4 },
});
