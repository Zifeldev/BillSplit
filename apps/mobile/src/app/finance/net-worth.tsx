import { useRouter, type Href } from 'expo-router';
import React from 'react';
import { StyleSheet } from 'react-native';

import { useNetWorth } from '@/api/queries';
import type { Account, NetWorth } from '@/api/types';
import { LineChart } from '@/components/charts';
import {
  Button,
  Card,
  EmptyView,
  ErrorView,
  FadeIn,
  GlassPill,
  HeroCard,
  IconName,
  IconTile,
  ListRow,
  Money,
  Row,
  Screen,
  SectionHeader,
  SkeletonList,
  Txt,
} from '@/components/ui';
import { compactMoney, formatMoney, formatSigned, shortDay } from '@/lib/format';
import { accountMeta } from '@/lib/networth';
import { spacing, tone, useTheme } from '@/theme';

const WHITE = '#FFFFFF';
const WHITE_SOFT = 'rgba(255,255,255,0.86)';

function Hero({ nw }: { nw: NetWorth }) {
  const router = useRouter();
  const cur = nw.currency;
  return (
    <HeroCard>
      <Txt variant="label" color={WHITE_SOFT}>
        Net worth
      </Txt>
      <Money cents={nw.netCents} currency={cur} variant="hero" color={WHITE} animate />
      <Txt variant="small" color={WHITE_SOFT}>
        {nw.hasChange30d ? `${formatSigned(nw.change30dCents, cur, true)} in 30 days` : 'Tracked daily from today'}
      </Txt>
      <Row gap={spacing.sm}>
        <GlassPill>
          <Txt variant="small" color={WHITE_SOFT} weight="600">
            You own
          </Txt>
          <Txt variant="h3" color={WHITE}>
            {formatMoney(nw.assetsCents, cur, true)}
          </Txt>
        </GlassPill>
        <GlassPill>
          <Txt variant="small" color={WHITE_SOFT} weight="600">
            You owe
          </Txt>
          <Txt variant="h3" color={WHITE}>
            {formatMoney(nw.liabilitiesCents, cur, true)}
          </Txt>
        </GlassPill>
      </Row>
      <Row gap={spacing.sm}>
        <Button title="Add account" icon="add" small variant="secondary" style={styles.flex1} onPress={() => router.push('/finance/account')} />
        <Button
          title="Export"
          icon="download-outline"
          small
          variant="secondary"
          style={styles.flex1}
          onPress={() => router.push({ pathname: '/finance/export', params: { kind: 'portfolio' } })}
        />
      </Row>
    </HeroCard>
  );
}

type Part = { key: string; label: string; icon: IconName; color: string; cents: number; negative?: boolean; href?: Href };

function Breakdown({ nw }: { nw: NetWorth }) {
  const router = useRouter();
  const { c } = useTheme();
  const parts: Part[] = [
    { key: 'cash', label: 'Cash, cards & savings', icon: 'wallet', color: tone.slate, cents: nw.cashCents },
    { key: 'inv', label: 'Investments', icon: 'trending-up', color: tone.pine, cents: nw.investmentsCents, href: '/finance/investments' },
    { key: 'prop', label: 'Property', icon: 'home', color: tone.ochre, cents: nw.propertyCents },
    { key: 'owed', label: 'Friends owe you', icon: 'arrow-down-circle', color: tone.forest, cents: nw.owedToMeCents },
    { key: 'credit', label: 'Credit cards & loans', icon: 'card-outline', color: tone.plum, cents: nw.creditCents, negative: true },
    { key: 'iowe', label: 'You owe friends', icon: 'arrow-up-circle', color: tone.clay, cents: nw.iOweCents, negative: true },
  ].filter((p) => p.cents > 0) as Part[];
  if (parts.length === 0) return null;
  const total = parts.reduce((s, p) => s + p.cents, 0);
  return (
    <Card padded={false}>
      {parts.map((p, i) => (
        <ListRow
          key={p.key}
          divider={i > 0}
          style={styles.row}
          left={<IconTile icon={p.icon} color={p.color} size={36} />}
          title={p.label}
          subtitle={`${Math.round((p.cents / total) * 100)}% of the total`}
          onPress={p.href ? () => router.push(p.href as Href) : undefined}
          right={
            <Txt weight="700" color={p.negative ? c.negative : c.text} style={styles.num}>
              {p.negative ? '−' : ''}
              {formatMoney(p.cents, nw.currency, true)}
            </Txt>
          }
        />
      ))}
    </Card>
  );
}

function AccountRow({ a, home, divider }: { a: Account; home: string; divider: boolean }) {
  const router = useRouter();
  const { c } = useTheme();
  const meta = accountMeta(a.kind);
  const sub = [meta.label, a.currency !== home ? formatMoney(a.balanceCents, a.currency) : null, a.updatedAt ? `updated ${shortDay(a.updatedAt.slice(0, 10))}` : null]
    .filter(Boolean)
    .join(' · ');
  return (
    <ListRow
      divider={divider}
      style={styles.row}
      left={<IconTile icon={meta.icon} color={meta.color} size={40} />}
      title={a.name}
      subtitle={sub}
      onPress={() => router.push({ pathname: '/finance/account', params: { id: a.id } })}
      right={
        <Txt weight="700" color={a.liability ? c.negative : c.text} style={styles.num}>
          {a.liability ? '−' : ''}
          {formatMoney(a.balanceHomeCents, home)}
        </Txt>
      }
    />
  );
}

export default function NetWorthScreen() {
  const router = useRouter();
  const { c } = useTheme();
  const q = useNetWorth();
  const nw = q.data;
  const hist = nw?.history ?? [];

  return (
    <Screen refreshing={q.isRefetching} onRefresh={() => void q.refetch()}>
      {nw ? (
        <>
          <Hero nw={nw} />
          <SectionHeader title="Over time" />
          <Card>
            {hist.length > 1 ? (
              <LineChart
                series={[{ name: 'Net worth', color: c.positive, values: hist.map((h) => h.netCents), area: true }]}
                labels={hist.map((h) => h.date)}
                formatLabel={shortDay}
                formatValue={(v) => formatMoney(Math.round(v), nw.currency)}
                formatAxis={(v) => compactMoney(v, nw.currency)}
                zeroBased={false}
                summary={`${shortDay(hist[0].date)} — ${shortDay(hist[hist.length - 1].date)}`}
              />
            ) : (
              <Txt variant="muted">
                The chart fills in by itself: we record your net worth once a day. Come back tomorrow to see the first change.
              </Txt>
            )}
          </Card>

          <SectionHeader title="What it's made of" />
          <Breakdown nw={nw} />

          <SectionHeader title="Accounts" action="Add" onAction={() => router.push('/finance/account')} />
          {nw.accounts.length === 0 ? (
            <Card>
              <EmptyView
                icon="business"
                title="Add your accounts"
                message="Cards, cash, savings, credit cards and loans — enter the balances once and update them when they change. Investments and debts with friends are added automatically."
                action={<Button title="Add account" icon="add" small onPress={() => router.push('/finance/account')} />}
              />
            </Card>
          ) : (
            <FadeIn>
              <Card padded={false}>
                {nw.accounts.map((a, i) => (
                  <AccountRow key={a.id} a={a} home={nw.currency} divider={i > 0} />
                ))}
              </Card>
            </FadeIn>
          )}
          <Txt variant="small" style={styles.note}>
            Everything is converted to {nw.currency} at today&apos;s rates. Investments use live quotes and deposit interest.
          </Txt>
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
  row: { paddingHorizontal: spacing.lg, paddingVertical: spacing.md },
  num: { fontVariant: ['tabular-nums'] },
  note: { textAlign: 'center' },
});
