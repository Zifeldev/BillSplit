import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { useMarketChart, usePortfolio } from '@/api/queries';
import type { MarketRange } from '@/api/types';
import { LineChart } from '@/components/charts';
import { Button, Card, ErrorView, Money, Row, Screen, SectionHeader, Segmented, Skeleton, Txt } from '@/components/ui';
import { formatChange, formatMoney, formatPrice, shortDay, shortMonthYear } from '@/lib/format';
import { guessKind } from '@/lib/invest';
import { spacing, useTheme } from '@/theme';

const RANGES: { value: MarketRange; label: string }[] = [
  { value: '1mo', label: '1M' },
  { value: '3mo', label: '3M' },
  { value: '6mo', label: '6M' },
  { value: '1y', label: '1Y' },
  { value: '5y', label: '5Y' },
];

export default function AssetScreen() {
  const { symbol = '' } = useLocalSearchParams<{ symbol: string }>();
  const router = useRouter();
  const { c } = useTheme();
  const [range, setRange] = useState<MarketRange>('1y');
  const q = useMarketChart(symbol, range);
  const portfolio = usePortfolio();
  const ch = q.data;
  const held = portfolio.data?.items.filter((i) => i.symbol === symbol) ?? [];
  const up = (ch?.rangeChangePercent ?? 0) >= 0;
  const pts = ch?.points ?? [];

  return (
    <Screen refreshing={q.isRefetching} onRefresh={() => void q.refetch()}>
      <Stack.Screen options={{ title: ch?.name || symbol }} />
      <Card>
        <Txt variant="small">{symbol}</Txt>
        {ch ? (
          <>
            <Txt variant="hero" style={styles.num}>
              {formatPrice(ch.price, ch.currency)}
            </Txt>
            <Row gap={spacing.md}>
              <Txt weight="700" color={up ? c.positive : c.negative}>
                {formatChange(ch.rangeChangePercent)} · {RANGES.find((r) => r.value === range)?.label}
              </Txt>
              <Txt variant="small" color={ch.dayChangePercent >= 0 ? c.positive : c.negative}>
                {formatChange(ch.dayChangePercent)} today
              </Txt>
            </Row>
          </>
        ) : (
          <Skeleton width={180} height={40} />
        )}
        <Segmented options={RANGES} value={range} onChange={setRange} />
        {ch && pts.length > 1 ? (
          <LineChart
            key={range}
            series={[{ name: ch.name, color: up ? c.positive : c.negative, values: pts.map((p) => p.close), area: true }]}
            labels={pts.map((p) => p.date)}
            formatLabel={range === '5y' ? shortMonthYear : shortDay}
            formatValue={(v) => formatPrice(v, ch.currency)}
            zeroBased={false}
            height={220}
            summary={`${shortDay(pts[0].date)} — ${shortDay(pts[pts.length - 1].date)}`}
          />
        ) : q.error ? (
          <ErrorView error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <Skeleton width="100%" height={220} />
        )}
      </Card>

      {held.length > 0 && portfolio.data ? (
        <>
          <SectionHeader title="Your position" />
          {held.map((i) => (
            <Card key={i.id}>
              <Row>
                <View style={styles.flex1}>
                  <Txt variant="small">
                    {i.quantity} × {formatPrice(i.price, i.currency)}
                  </Txt>
                  <Money cents={i.valueCents} currency={i.currency} variant="h2" />
                  {i.currency !== portfolio.data.currency ? (
                    <Txt variant="small">≈ {formatMoney(i.valueHomeCents, portfolio.data.currency)}</Txt>
                  ) : null}
                </View>
                <View style={styles.right}>
                  <Txt variant="small">Invested {formatMoney(i.contributedCents, i.currency)}</Txt>
                  <Txt weight="700" color={i.gainCents >= 0 ? c.positive : c.negative}>
                    {i.gainCents >= 0 ? '+' : '−'}
                    {formatMoney(Math.abs(i.gainCents), i.currency)} ({formatChange(i.gainPercent)})
                  </Txt>
                </View>
              </Row>
              <Button title="Edit position" icon="create-outline" variant="secondary" small onPress={() => router.push({ pathname: '/finance/investment', params: { id: i.id } })} />
            </Card>
          ))}
        </>
      ) : ch ? (
        <Button
          title="Add to portfolio"
          icon="add"
          onPress={() => router.push({ pathname: '/finance/investment', params: { symbol, kind: guessKind(symbol) } })}
        />
      ) : null}
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  right: { alignItems: 'flex-end', gap: 4 },
  num: { fontVariant: ['tabular-nums'] },
});
