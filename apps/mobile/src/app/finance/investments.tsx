import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { useMarketOverview, usePortfolio } from '@/api/queries';
import type { Investment, MarketQuote, Portfolio } from '@/api/types';
import { Donut, LegendRow, LineChart, Sparkline } from '@/components/charts';
import {
  Button,
  Card,
  EmptyView,
  ErrorView,
  FadeIn,
  GlassPill,
  HeroCard,
  IconTile,
  ListRow,
  Money,
  Row,
  Screen,
  SectionHeader,
  SkeletonList,
  Txt,
} from '@/components/ui';
import { compactMoney, formatChange, formatDate, formatMoney, formatPercent, formatPrice, shortDay } from '@/lib/format';
import { tapFeedback } from '@/lib/haptics';
import { CAPITALIZATIONS, kindColor, kindMeta } from '@/lib/invest';
import { spacing, useTheme } from '@/theme';

const WHITE = '#FFFFFF';
const WHITE_SOFT = 'rgba(255,255,255,0.86)';

function Hero({ p }: { p: Portfolio }) {
  const router = useRouter();
  const cur = p.currency;
  return (
    <HeroCard>
      <Txt variant="label" color={WHITE_SOFT}>
        Portfolio value
      </Txt>
      <Money cents={p.valueCents} currency={cur} variant="hero" color={WHITE} animate />
      <Row gap={spacing.sm}>
        <GlassPill>
          <Txt variant="small" color={WHITE_SOFT} weight="600">
            Gain
          </Txt>
          <Txt variant="h3" color={WHITE}>
            {p.gainCents >= 0 ? '+' : '−'}
            {formatMoney(Math.abs(p.gainCents), cur)}
          </Txt>
          <Txt variant="small" color={WHITE_SOFT}>
            {formatChange(p.gainPercent)}
          </Txt>
        </GlassPill>
        <GlassPill>
          <Txt variant="small" color={WHITE_SOFT} weight="600">
            Interest / 12 mo
          </Txt>
          <Txt variant="h3" color={WHITE}>
            {formatMoney(p.yearlyInterestCents, cur)}
          </Txt>
          <Txt variant="small" color={WHITE_SOFT}>
            from deposits
          </Txt>
        </GlassPill>
      </Row>
      <Row gap={spacing.sm}>
        <Button title="Add" icon="add" small variant="secondary" style={styles.flex1} onPress={() => router.push('/finance/investment')} />
        <Button title="Calculators" icon="calculator" small variant="secondary" style={styles.flex1} onPress={() => router.push('/finance/calculators')} />
      </Row>
    </HeroCard>
  );
}

function Allocation({ p }: { p: Portfolio }) {
  const { dark } = useTheme();
  const [active, setActive] = useState<string | null>(null);
  const slices = p.allocation
    .filter((a) => a.valueCents > 0)
    .map((a) => ({ key: a.kind, label: kindMeta(a.kind).label, value: a.valueCents, color: kindColor(a.kind, dark), share: a.share }));
  if (slices.length === 0) return null;
  const sel = slices.find((s) => s.key === active);
  return (
    <Card>
      <View style={styles.center}>
        <Donut
          slices={slices}
          active={active}
          centerTop={sel ? sel.label : 'Total'}
          centerValue={compactMoney(sel ? sel.value : p.valueCents, p.currency)}
        />
      </View>
      {slices.map((s) => (
        <LegendRow
          key={s.key}
          color={s.color}
          label={s.label}
          value={formatMoney(s.value, p.currency)}
          share={formatPercent(s.share)}
          active={active === s.key}
          onPress={() => {
            tapFeedback();
            setActive(active === s.key ? null : s.key);
          }}
        />
      ))}
    </Card>
  );
}

function holdingSubtitle(i: Investment): string {
  const meta = kindMeta(i.kind);
  if (i.symbol && meta.market) {
    if (i.quoteError) return `${i.symbol} · ${i.quoteError}`;
    return `${i.quantity} × ${formatPrice(i.price, i.currency)} · ${formatChange(i.dayChangePercent)} today`;
  }
  const cap = CAPITALIZATIONS.find((c) => c.value === i.capitalization)?.label.toLowerCase() ?? 'monthly';
  const parts = [`${i.ratePercent}%`, cap === 'none' ? 'no capitalization' : `${cap} cap. → ${i.effectiveRatePercent.toFixed(2)}% eff.`];
  if (i.maturityAt) parts.push(`until ${formatDate(i.maturityAt)}`);
  return parts.join(' · ');
}

function Holdings({ items, currency }: { items: Investment[]; currency: string }) {
  const router = useRouter();
  const { c, dark } = useTheme();
  return (
    <Card padded={false}>
      {items.map((i, idx) => {
        const meta = kindMeta(i.kind);
        const market = !!i.symbol && meta.market;
        return (
          <ListRow
            key={i.id}
            divider={idx > 0}
            style={styles.holding}
            left={<IconTile icon={meta.icon} color={kindColor(i.kind, dark)} size={40} />}
            title={i.name}
            subtitle={holdingSubtitle(i)}
            onPress={() =>
              market
                ? router.push({ pathname: '/finance/asset', params: { symbol: i.symbol, id: i.id } })
                : router.push({ pathname: '/finance/investment', params: { id: i.id } })
            }
            right={
              <View style={styles.right}>
                <Money cents={i.valueHomeCents} currency={currency} />
                <Txt variant="small" weight="700" color={i.gainCents > 0 ? c.positive : i.gainCents < 0 ? c.negative : c.textMuted}>
                  {formatChange(i.gainPercent)}
                </Txt>
              </View>
            }
          />
        );
      })}
    </Card>
  );
}

function Trends({ quotes }: { quotes: MarketQuote[] }) {
  const router = useRouter();
  const { c } = useTheme();
  return (
    <Card padded={false}>
      {quotes.map((q, idx) => (
        <ListRow
          key={q.symbol}
          divider={idx > 0}
          style={styles.holding}
          title={q.name}
          subtitle={
            <Txt variant="small" numberOfLines={1}>
              {q.symbol} · 30d {formatChange(q.monthChangePercent)}
              {q.held ? (
                <Txt variant="small" color={c.primary} weight="700">
                  {' '}· yours
                </Txt>
              ) : null}
            </Txt>
          }
          onPress={() => router.push({ pathname: '/finance/asset', params: { symbol: q.symbol } })}
          right={
            <Row gap={spacing.md}>
              <Sparkline values={q.spark} />
              <View style={styles.right}>
                <Txt weight="700" style={styles.num}>
                  {formatPrice(q.price, q.currency)}
                </Txt>
                <Txt variant="small" weight="700" color={q.dayChangePercent >= 0 ? c.positive : c.negative}>
                  {formatChange(q.dayChangePercent)}
                </Txt>
              </View>
            </Row>
          }
        />
      ))}
    </Card>
  );
}

export default function InvestmentsScreen() {
  const router = useRouter();
  const { c } = useTheme();
  const q = usePortfolio();
  const market = useMarketOverview();
  const p = q.data;
  const hist = p?.history ?? [];
  const showHistory = hist.some((h) => h.valueCents > 0);

  return (
    <Screen
      refreshing={q.isRefetching || market.isRefetching}
      onRefresh={() => {
        void q.refetch();
        void market.refetch();
      }}>
      {p ? (
        <>
          <Hero p={p} />
          {p.items.length === 0 ? (
            <Card>
              <EmptyView
                icon="trending-up"
                title="No investments yet"
                message="Add a bank deposit to track its interest, or a stock, ETF or crypto position to follow it at live prices."
                action={
                  <Row gap={spacing.sm}>
                    <Button title="Deposit" icon="business-outline" small onPress={() => router.push({ pathname: '/finance/investment', params: { kind: 'INVESTMENT_KIND_DEPOSIT' } })} />
                    <Button title="Stock / crypto" icon="trending-up" small variant="secondary" onPress={() => router.push({ pathname: '/finance/investment', params: { kind: 'INVESTMENT_KIND_STOCK' } })} />
                  </Row>
                }
              />
            </Card>
          ) : (
            <>
              {showHistory ? (
                <FadeIn>
                  <SectionHeader title="Value over the year" />
                  <Card>
                    <LineChart
                      series={[
                        { name: 'Value', color: c.primary, values: hist.map((h) => h.valueCents), area: true },
                        { name: 'Put in', color: c.textMuted, values: hist.map((h) => h.contributedCents), dashed: true },
                      ]}
                      labels={hist.map((h) => h.date)}
                      formatLabel={shortDay}
                      formatValue={(v) => formatMoney(Math.round(v), p.currency)}
                      formatAxis={(v) => compactMoney(v, p.currency)}
                    />
                    <Txt variant="small">Weekly. Market positions use today&apos;s quantity and today&apos;s exchange rate.</Txt>
                  </Card>
                </FadeIn>
              ) : null}
              <SectionHeader title="Allocation" />
              <Allocation p={p} />
              <SectionHeader title="Holdings" action="Add" onAction={() => router.push('/finance/investment')} />
              <Holdings items={p.items} currency={p.currency} />
            </>
          )}
        </>
      ) : q.error ? (
        <ErrorView error={q.error} onRetry={() => void q.refetch()} />
      ) : (
        <SkeletonList rows={4} />
      )}

      <SectionHeader title="Market trends" />
      {market.data ? (
        market.data.quotes.length ? (
          <Trends quotes={market.data.quotes} />
        ) : (
          <Card>
            <Txt variant="muted">Quotes are unavailable right now. Pull to refresh.</Txt>
          </Card>
        )
      ) : market.error ? (
        <ErrorView error={market.error} onRetry={() => void market.refetch()} />
      ) : (
        <SkeletonList rows={3} />
      )}
      <Txt variant="small" style={styles.note}>
        Quotes: Yahoo Finance, may be delayed. Not investment advice.
      </Txt>
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  center: { alignItems: 'center', paddingVertical: spacing.sm },
  holding: { paddingHorizontal: spacing.lg, paddingVertical: spacing.md },
  right: { alignItems: 'flex-end', gap: 2 },
  num: { fontVariant: ['tabular-nums'] },
  note: { textAlign: 'center', marginTop: spacing.sm },
});
