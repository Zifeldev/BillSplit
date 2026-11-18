import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useMarketChart, usePortfolio, useSymbolSearch } from '@/api/queries';
import { FinanceApi } from '@/api/services';
import type { Capitalization, Investment, InvestmentKind } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { CurrencyField } from '@/components/CurrencyField';
import { Banner, Button, Card, Chip, Field, ListRow, Row, Screen, Segmented, Txt } from '@/components/ui';
import { confirm } from '@/lib/dialogs';
import {
  amountPlaceholder,
  dateInputToRfc3339,
  formatDate,
  formatMoney,
  formatPrice,
  minorToInput,
  parseMinor,
  todayInput,
  tsToDateInput,
} from '@/lib/format';
import { CAPITALIZATIONS, depositSchedule, KINDS, kindMeta, SUGGESTED } from '@/lib/invest';
import { spacing, useTheme } from '@/theme';

function useDebounced<T>(v: T, ms = 350): T {
  const [d, setD] = useState(v);
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms);
    return () => clearTimeout(t);
  }, [v, ms]);
  return d;
}

const num = (s: string) => Number(s.replace(',', '.').replace(/\s/g, ''));

function TickerPicker({ kind, symbol, onPick }: { kind: InvestmentKind; symbol: string; onPick: (sym: string, name: string, type: string) => void }) {
  const { c } = useTheme();
  const [q, setQ] = useState('');
  const dq = useDebounced(q.trim());
  const search = useSymbolSearch(dq);
  const matches = (search.data?.matches ?? []).filter((m) => m.type !== 'FUTURE' || kind === 'INVESTMENT_KIND_METAL');
  return (
    <View style={styles.gap}>
      <Field label="Ticker or name" value={q} onChangeText={setQ} placeholder="Kaspi, AAPL, bitcoin, VOO…" autoCapitalize="none" autoCorrect={false} />
      {dq.length < 2 ? (
        <Row gap={6} style={styles.wrap}>
          {(SUGGESTED[kind] ?? []).map((s) => (
            <Chip key={s.symbol} label={`${s.name} · ${s.symbol}`} active={symbol === s.symbol} onPress={() => onPick(s.symbol, s.name, '')} />
          ))}
        </Row>
      ) : search.isFetching && matches.length === 0 ? (
        <Txt variant="small">Searching…</Txt>
      ) : matches.length === 0 ? (
        <Txt variant="small">{search.error ? errorMessage(search.error) : 'Nothing found'}</Txt>
      ) : (
        <View style={[styles.results, { borderColor: c.border }]}>
          {matches.slice(0, 6).map((m, i) => (
            <ListRow
              key={m.symbol}
              divider={i > 0}
              style={styles.resultRow}
              title={`${m.symbol} · ${m.name || m.symbol}`}
              subtitle={[m.exchange, m.type].filter(Boolean).join(' · ')}
              onPress={() => {
                onPick(m.symbol, m.name, m.type);
                setQ('');
              }}
            />
          ))}
        </View>
      )}
    </View>
  );
}

function Form({ inv, initialKind, initialSymbol }: { inv: Investment | null; initialKind?: InvestmentKind; initialSymbol?: string }) {
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const { user } = useAuth();
  const [kind, setKind] = useState<InvestmentKind>(inv?.kind ?? initialKind ?? 'INVESTMENT_KIND_DEPOSIT');
  const meta = kindMeta(kind);
  const [name, setName] = useState(inv?.name ?? '');
  const [symbol, setSymbol] = useState(inv?.symbol ?? initialSymbol ?? '');
  const [currency, setCurrency] = useState(inv?.currency ?? '');
  const cur = currency || user?.currency || 'USD';
  const [quantity, setQuantity] = useState(inv ? String(inv.quantity) : '');
  const [invested, setInvested] = useState(inv ? minorToInput(inv.investedCents, inv.currency) : '');
  const [rate, setRate] = useState(inv ? String(inv.ratePercent) : '');
  const [cap, setCap] = useState<Capitalization>(
    inv && inv.capitalization !== 'CAPITALIZATION_UNSPECIFIED' ? inv.capitalization : 'CAPITALIZATION_MONTHLY',
  );
  const [term, setTerm] = useState(inv?.termMonths ? String(inv.termMonths) : '12');
  const [topUp, setTopUp] = useState(inv?.monthlyContributionCents ? minorToInput(inv.monthlyContributionCents, inv.currency) : '');
  const [opened, setOpened] = useState(tsToDateInput(inv?.openedAt) || todayInput());
  const [error, setError] = useState<string | null>(null);

  const quote = useMarketChart(meta.market ? symbol : '', '1mo');
  const qcur = quote.data?.currency ?? (inv?.currency || 'USD');

  const save = useMutation({
    mutationFn: () => {
      const openedAt = dateInputToRfc3339(opened);
      if (!openedAt) throw new Error('Opening date must be YYYY-MM-DD');
      if (meta.market) {
        if (!symbol) throw new Error('Pick a ticker');
        const qty = num(quantity);
        if (!(qty > 0)) throw new Error('Enter a positive quantity');
        const cost = invested.trim() ? parseMinor(invested, qcur) : 0;
        if (cost === null || cost < 0) throw new Error('Invalid amount invested');
        return FinanceApi.setInvestment({ id: inv?.id, kind, name: name.trim(), symbol, quantity: qty, investedCents: cost, openedAt });
      }
      const amount = parseMinor(invested, cur);
      if (amount === null || amount <= 0) throw new Error('Enter a positive amount');
      const r = num(rate || '0');
      if (!Number.isFinite(r) || r < 0 || r > 1000) throw new Error('Rate must be a percentage, e.g. 16.5');
      const months = term.trim() ? Math.round(num(term)) : 0;
      if (!Number.isFinite(months) || months < 0 || months > 600) throw new Error('Term must be 0–600 months');
      const top = topUp.trim() ? parseMinor(topUp, cur) : 0;
      if (top === null || top < 0) throw new Error('Invalid monthly top-up');
      if (!name.trim()) throw new Error('Name is required');
      return FinanceApi.setInvestment({
        id: inv?.id,
        kind,
        name: name.trim(),
        currency: cur,
        investedCents: amount,
        ratePercent: r,
        capitalization: cap,
        termMonths: months,
        monthlyContributionCents: top,
        openedAt,
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.portfolio });
      void qc.invalidateQueries({ queryKey: qk.marketOverview });
      router.back();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const del = useMutation({
    mutationFn: () => FinanceApi.deleteInvestment({ id: inv?.id ?? '' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.portfolio });
      void qc.invalidateQueries({ queryKey: qk.marketOverview });
      router.back();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  // Live preview: deposit at maturity (or in 12 months for open-ended ones).
  const amountMinor = parseMinor(invested, cur) ?? 0;
  const months = Math.round(num(term || '0')) || 12;
  const preview =
    !meta.market && amountMinor > 0 && num(rate || '0') >= 0
      ? depositSchedule({ amount: amountMinor, ratePct: num(rate || '0'), months, cap, topUp: parseMinor(topUp, cur) ?? 0, taxPct: 0 })
      : null;
  const qty = num(quantity);
  const marketValue = quote.data && qty > 0 ? qty * quote.data.price : null;

  return (
    <>
      <Stack.Screen options={{ title: inv ? 'Edit investment' : 'New investment' }} />
      {error ? <Banner kind="error" message={error} /> : null}
      <Card>
        <Txt variant="label">Type</Txt>
        <Row gap={6} style={styles.wrap}>
          {KINDS.map((k) => (
            <Chip key={k.value} label={k.label} icon={k.icon} active={kind === k.value} onPress={() => setKind(k.value)} />
          ))}
        </Row>
      </Card>

      {meta.market ? (
        <Card>
          <TickerPicker
            kind={kind}
            symbol={symbol}
            onPick={(s, n, type) => {
              setSymbol(s);
              if (!name.trim() || name === quote.data?.name) setName(n);
              if (type === 'ETF' || type === 'MUTUALFUND') setKind('INVESTMENT_KIND_ETF');
              else if (type === 'CRYPTOCURRENCY') setKind('INVESTMENT_KIND_CRYPTO');
              else if (type === 'EQUITY') setKind('INVESTMENT_KIND_STOCK');
            }}
          />
          {symbol ? (
            <Row style={[styles.picked, { backgroundColor: c.cardAlt }]}>
              <View style={styles.flex1}>
                <Txt weight="700">{symbol}</Txt>
                <Txt variant="small" numberOfLines={1}>
                  {quote.data ? `${quote.data.name} · ${formatPrice(quote.data.price, quote.data.currency)}` : quote.error ? errorMessage(quote.error) : 'Loading quote…'}
                </Txt>
              </View>
            </Row>
          ) : null}
          <Field label="Name (optional)" value={name} onChangeText={setName} placeholder={quote.data?.name ?? 'Kaspi.kz'} />
          <Row gap={spacing.sm}>
            <Field style={styles.flex1} label="Quantity" value={quantity} onChangeText={setQuantity} keyboardType="decimal-pad" placeholder="10" />
            <Field
              style={styles.flex1}
              label={`Total paid, ${qcur}`}
              value={invested}
              onChangeText={setInvested}
              keyboardType="decimal-pad"
              placeholder={amountPlaceholder(qcur)}
            />
          </Row>
          <Field label="Bought on (YYYY-MM-DD)" value={opened} onChangeText={setOpened} autoCapitalize="none" placeholder="2026-01-15" />
          {marketValue !== null && quote.data ? (
            <Txt variant="small">
              Worth now: <Txt variant="small" weight="700" color={c.text}>{formatPrice(marketValue, quote.data.currency)}</Txt>
            </Txt>
          ) : null}
        </Card>
      ) : (
        <Card>
          <Field label="Name" value={name} onChangeText={setName} placeholder={kind === 'INVESTMENT_KIND_DEPOSIT' ? 'Freedom Bank deposit' : 'Bond / loan to a friend'} />
          <Row gap={spacing.sm}>
            <Field style={styles.flex1} label="Amount" value={invested} onChangeText={setInvested} keyboardType="decimal-pad" placeholder={amountPlaceholder(cur)} />
            <Field style={styles.flex1} label="Rate, % per year" value={rate} onChangeText={setRate} keyboardType="decimal-pad" placeholder="16.5" />
          </Row>
          <CurrencyField label="Currency" value={currency} onChange={setCurrency} allowEmpty emptyLabel={`Home currency (${user?.currency ?? 'USD'})`} />
          <Txt variant="label">Interest capitalization</Txt>
          <Segmented options={CAPITALIZATIONS.map((x) => ({ value: x.value, label: x.label }))} value={cap} onChange={setCap} />
          <Row gap={spacing.sm}>
            <Field style={styles.flex1} label="Term, months (0 — open)" value={term} onChangeText={setTerm} keyboardType="number-pad" placeholder="12" />
            <Field style={styles.flex1} label="Monthly top-up" value={topUp} onChangeText={setTopUp} keyboardType="decimal-pad" placeholder={amountPlaceholder(cur)} />
          </Row>
          <Field label="Opened on (YYYY-MM-DD)" value={opened} onChangeText={setOpened} autoCapitalize="none" placeholder="2026-01-15" />
          {preview ? (
            <View style={[styles.preview, { backgroundColor: c.positiveSoft }]}>
              <Txt variant="small" color={c.text}>
                {num(term || '0') > 0 ? `At maturity (${months} mo)` : 'In 12 months'}:{' '}
                <Txt variant="small" weight="800" color={c.text}>
                  {formatMoney(Math.round(preview.final), cur)}
                </Txt>
              </Txt>
              <Txt variant="small" color={c.text}>
                Interest {formatMoney(Math.round(preview.interest), cur)} · effective {preview.effective.toFixed(2)}% / yr
              </Txt>
            </View>
          ) : null}
        </Card>
      )}

      <Button title="Save" icon="checkmark" loading={save.isPending} onPress={() => { setError(null); save.mutate(); }} />
      {inv ? (
        <Button
          title="Delete"
          icon="trash-outline"
          variant="ghost"
          loading={del.isPending}
          onPress={async () => {
            if (await confirm('Delete investment?', `${inv.name} will be removed from your portfolio.`, 'Delete', true)) del.mutate();
          }}
        />
      ) : null}
      {inv?.maturityAt ? <Txt variant="small" style={styles.center}>Matures {formatDate(inv.maturityAt)}</Txt> : null}
    </>
  );
}

export default function InvestmentScreen() {
  const { id, kind, symbol } = useLocalSearchParams<{ id?: string; kind?: InvestmentKind; symbol?: string }>();
  const portfolio = usePortfolio();
  const inv = id ? portfolio.data?.items.find((i) => i.id === id) ?? null : null;
  if (id && !portfolio.data) {
    return (
      <Screen>
        <Txt variant="muted">Loading…</Txt>
      </Screen>
    );
  }
  return (
    <Screen>
      <Form key={inv?.id ?? 'new'} inv={inv} initialKind={kind} initialSymbol={symbol} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  gap: { gap: spacing.sm },
  wrap: { flexWrap: 'wrap' },
  results: { borderWidth: StyleSheet.hairlineWidth, borderRadius: 12, overflow: 'hidden' },
  resultRow: { paddingHorizontal: spacing.md, paddingVertical: spacing.sm },
  picked: { padding: spacing.md, borderRadius: 12 },
  preview: { padding: spacing.md, borderRadius: 12, gap: 2 },
  center: { textAlign: 'center' },
});
